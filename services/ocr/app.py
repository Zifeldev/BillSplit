"""Receipt / document OCR service: RapidOCR (PaddleOCR PP-OCR models on ONNX
Runtime) + pypdfium2 for PDFs.

POST /ocr        body = image (jpeg/png/webp) or PDF  ->  {"text", "lines", "pages", "skew_deg", "source"}
POST /pdf/text   body = PDF                           ->  {"pages": [{"lines": [...]}], "text"}

Receipts are tables: OCR returns separate boxes for "BANANAS" and "1.96", so
boxes are grouped into visual rows and joined left-to-right. Phone photos are
usually a bit rotated; a 1° tilt over a 1000px-wide receipt shifts the right
column by a whole text line and glues neighbouring items together. So rows are
grouped in a de-rotated coordinate system (skew estimated from the text boxes).

PDFs: digital PDFs (e-receipts, bank statements) have a text layer — it's
returned as is (exact, fast). Scanned PDFs have none — pages are rendered and OCR'd.
"""
import logging
import math
import statistics

import numpy as np
import pypdfium2 as pdfium
from fastapi import FastAPI, HTTPException, Request
from rapidocr import LangRec, RapidOCR

MAX_BYTES = 16 * 1024 * 1024
MAX_PDF_PAGES = 30
RENDER_SCALE = 2.5  # ~180 dpi: enough for receipt fonts without huge images

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("ocr")

engine = RapidOCR(params={"Rec.lang_type": LangRec.EN})
app = FastAPI(title="billsplit-ocr")


# ---------------------------------------------------------------- image OCR

def estimate_skew(items):
    """Width-weighted median angle of wide text boxes (radians)."""
    wide = [i for i in items if i["w"] > 3 * i["h"]] or items
    wide.sort(key=lambda i: i["ang"])
    half, acc = sum(i["w"] for i in wide) / 2, 0.0
    for i in wide:
        acc += i["w"]
        if acc >= half:
            return i["ang"]
    return 0.0


def group_rows(boxes, txts, scores, min_score=0.5):
    """Group text boxes into visual lines, compensating for image rotation."""
    items = []
    for box, txt, score in zip(boxes, txts, scores):
        if score < min_score or not txt.strip():
            continue
        p = np.asarray(box, dtype=float)  # tl, tr, br, bl
        items.append({
            "p": p,
            "t": txt.strip(),
            "ang": math.atan2(p[1][1] - p[0][1], p[1][0] - p[0][0]),
            "w": float(np.hypot(*(p[1] - p[0]))),
            "h": float(np.hypot(*(p[3] - p[0]))),
        })
    if not items:
        return [], 0.0

    skew = estimate_skew(items)
    c, s = math.cos(-skew), math.sin(-skew)
    for i in items:  # rotate into the text's own axis
        xs = i["p"][:, 0] * c - i["p"][:, 1] * s
        ys = i["p"][:, 0] * s + i["p"][:, 1] * c
        i["x"], i["yc"] = float(xs.min()), float(ys.mean())

    tol = statistics.median(i["h"] for i in items) * 0.45
    items.sort(key=lambda i: i["yc"])
    rows, cur = [], [items[0]]
    for it in items[1:]:
        if abs(it["yc"] - statistics.median(j["yc"] for j in cur)) <= tol:
            cur.append(it)
        else:
            rows.append(cur)
            cur = [it]
    rows.append(cur)
    lines = [" ".join(j["t"] for j in sorted(r, key=lambda j: j["x"])) for r in rows]
    return lines, math.degrees(skew)


def ocr_image(data):
    res = engine(data)
    if res is None or res.txts is None:
        return [], 0.0
    return group_rows(res.boxes, res.txts, res.scores)


# ---------------------------------------------------------------- PDF

def pdf_pages(data, keep_trailing_spaces=False):
    """Yields (lines, source) per page: text layer if present, else OCR of the rendered page.

    keep_trailing_spaces: in bank statements a wrapped table cell ends its line
    with a space while the last line of a cell doesn't — the statement parser
    relies on that, so /pdf/text keeps lines exactly as in the text layer."""
    try:
        pdf = pdfium.PdfDocument(data)
    except pdfium.PdfiumError as e:
        raise HTTPException(422, f"cannot open PDF: {e}")
    try:
        if len(pdf) > MAX_PDF_PAGES:
            raise HTTPException(413, f"PDF has {len(pdf)} pages, max {MAX_PDF_PAGES}")
        for i in range(len(pdf)):
            page = pdf[i]
            text = page.get_textpage().get_text_range()
            lines = [ln if keep_trailing_spaces else ln.strip()
                     for ln in text.replace("\r", "").split("\n") if ln.strip()]
            if len(lines) >= 3:
                yield lines, "text-layer"
                continue
            img = page.render(scale=RENDER_SCALE).to_numpy()  # scanned page
            ocr_lines, _ = ocr_image(np.ascontiguousarray(img[:, :, :3]))
            yield ocr_lines, "ocr"
    finally:
        pdf.close()


def is_pdf(data):
    return data[:5] == b"%PDF-"


async def read_body(request):
    data = await request.body()
    if not data:
        raise HTTPException(400, "empty body")
    if len(data) > MAX_BYTES:
        raise HTTPException(413, "file too large")
    return data


# ---------------------------------------------------------------- HTTP

@app.get("/healthz")
def healthz():
    return {"ok": True}


@app.post("/ocr")
async def ocr(request: Request):
    data = await read_body(request)
    if is_pdf(data):
        pages = [{"lines": lines, "source": src} for lines, src in pdf_pages(data)]
        lines = [ln for p in pages for ln in p["lines"]]
        source = "pdf-" + ("text-layer" if all(p["source"] == "text-layer" for p in pages) else "ocr")
        log.info("pdf: %d pages, %d lines (%s)", len(pages), len(lines), source)
        return {"text": "\n".join(lines), "lines": lines, "pages": pages, "skew_deg": 0, "source": source}
    try:
        lines, skew = ocr_image(data)
    except Exception as e:  # corrupted image etc.
        log.warning("ocr failed: %s", e)
        raise HTTPException(422, f"cannot process image: {e}")
    log.info("image: %d lines, skew %.2f°", len(lines), skew)
    return {"text": "\n".join(lines), "lines": lines, "pages": [{"lines": lines, "source": "ocr"}],
            "skew_deg": round(skew, 2), "source": "image"}


@app.post("/pdf/text")
async def pdf_text(request: Request):
    data = await read_body(request)
    if not is_pdf(data):
        raise HTTPException(415, "not a PDF")
    pages = [{"lines": lines, "source": src} for lines, src in pdf_pages(data, keep_trailing_spaces=True)]
    log.info("pdf/text: %d pages", len(pages))
    return {"pages": pages, "text": "\n".join(ln for p in pages for ln in p["lines"])}
