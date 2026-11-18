FROM python:3.12-slim
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
RUN apt-get update && apt-get install -y --no-install-recommends libgl1 libglib2.0-0 && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY app.py .
# Download OCR models at build time, not on the first request.
RUN python -c "from rapidocr import RapidOCR, LangRec; RapidOCR(params={'Rec.lang_type': LangRec.EN})"
RUN useradd -u 10001 app && chown -R app /app /usr/local/lib/python3.12/site-packages/rapidocr
USER app
EXPOSE 8000
# Exec form: uvicorn is PID 1 and gets SIGTERM directly; it stops accepting and
# waits up to 20s for in-flight OCR requests.
CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8000", "--workers", "1", "--timeout-graceful-shutdown", "20"]
