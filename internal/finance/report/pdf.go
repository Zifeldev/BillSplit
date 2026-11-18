package report

import (
	"bytes"
	_ "embed"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
)

// DejaVu Sans covers Latin, Cyrillic and currency signs (₸ ₽ €); see fonts/LICENSE.
var (
	//go:embed fonts/DejaVuSans.ttf
	fontRegular []byte
	//go:embed fonts/DejaVuSans-Bold.ttf
	fontBold []byte
)

type rgb struct{ r, g, b int }

// Palette matches the app (light theme).
var (
	cInk      = rgb{17, 24, 39}
	cMuted    = rgb{107, 114, 128}
	cLine     = rgb{229, 231, 235}
	cSoft     = rgb{246, 247, 249}
	cPrimary  = rgb{91, 95, 232}
	cPositive = rgb{20, 150, 106}
	cNegative = rgb{217, 61, 87}
	cBand     = rgb{15, 118, 110}
)

const (
	pageW    = 210.0
	margin   = 15.0
	contentW = pageW - 2*margin
)

type doc struct {
	*fpdf.Fpdf
}

func newDoc(title, subtitle string, generated time.Time) *doc {
	f := fpdf.New("P", "mm", "A4", "")
	f.AddUTF8FontFromBytes("dv", "", fontRegular)
	f.AddUTF8FontFromBytes("dv", "B", fontBold)
	f.SetMargins(margin, margin, margin)
	f.SetAutoPageBreak(true, 18)
	f.SetTitle(title, true)
	f.SetCreator("Billsplit", true)
	f.AliasNbPages("{nb}")
	d := &doc{f}
	f.SetFooterFunc(func() {
		f.SetY(-12)
		d.font(7.5, false, cMuted)
		f.CellFormat(contentW/2, 4, "Billsplit · "+generated.Format("2 Jan 2006 15:04 MST"), "", 0, "L", false, 0, "")
		f.CellFormat(contentW/2, 4, fmt.Sprintf("Page %d of {nb}", f.PageNo()), "", 0, "R", false, 0, "")
	})
	f.AddPage()

	// Header band.
	d.fill(cBand)
	f.Rect(0, 0, pageW, 30, "F")
	f.SetXY(margin, 8)
	d.font(8, true, rgb{204, 251, 241})
	f.CellFormat(contentW, 4, "BILLSPLIT", "", 1, "L", false, 0, "")
	d.font(17, true, rgb{255, 255, 255})
	f.CellFormat(contentW, 9, title, "", 1, "L", false, 0, "")
	d.font(9, false, rgb{204, 251, 241})
	f.CellFormat(contentW, 5, subtitle, "", 1, "L", false, 0, "")
	f.SetY(38)
	return d
}

func (d *doc) font(size float64, bold bool, c rgb) {
	style := ""
	if bold {
		style = "B"
	}
	d.SetFont("dv", style, size)
	d.SetTextColor(c.r, c.g, c.b)
}

func (d *doc) fill(c rgb) { d.SetFillColor(c.r, c.g, c.b) }
func (d *doc) draw(c rgb) { d.SetDrawColor(c.r, c.g, c.b) }

// fit shortens text with an ellipsis to fit width (mm) in the current font.
func (d *doc) fit(s string, w float64) string {
	s = strings.Join(strings.Fields(s), " ")
	if d.GetStringWidth(s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 1 && d.GetStringWidth(string(r)+"…") > w {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

func (d *doc) section(title string) {
	d.ensure(16)
	d.Ln(3)
	d.font(11.5, true, cInk)
	d.CellFormat(contentW, 7, title, "", 1, "L", false, 0, "")
	d.Ln(1)
}

// ensure starts a new page when less than h mm are left.
func (d *doc) ensure(h float64) {
	_, pageH := d.GetPageSize()
	if d.GetY()+h > pageH-20 {
		d.AddPage()
	}
}

type kpi struct {
	label, value string
	color        rgb
}

func (d *doc) kpis(items []kpi) {
	gap := 4.0
	w := (contentW - gap*float64(len(items)-1)) / float64(len(items))
	y := d.GetY()
	for i, k := range items {
		x := margin + float64(i)*(w+gap)
		d.fill(cSoft)
		d.RoundedRect(x, y, w, 19, 2.5, "1234", "F")
		d.SetXY(x+4, y+3.5)
		d.font(7.5, false, cMuted)
		d.CellFormat(w-8, 4, strings.ToUpper(k.label), "", 2, "L", false, 0, "")
		d.SetX(x + 4)
		size := 13.0
		d.font(size, true, k.color)
		for size > 8 && d.GetStringWidth(k.value) > w-8 {
			size -= 0.5
			d.font(size, true, k.color)
		}
		d.CellFormat(w-8, 8, k.value, "", 0, "L", false, 0, "")
	}
	d.SetXY(margin, y+23)
}

type col struct {
	title string
	w     float64 // share of contentW
	align string
}

type cell struct {
	text  string
	color rgb
	bold  bool
}

func plain(s string) cell { return cell{text: s, color: cInk} }

// table draws a striped table, repeating the header on page breaks.
func (d *doc) table(cols []col, rows [][]cell) {
	const rh = 6.2
	header := func() {
		d.font(7.5, true, cMuted)
		for _, c := range cols {
			d.CellFormat(c.w*contentW, rh, strings.ToUpper(c.title), "B", 0, c.align, false, 0, "")
		}
		d.Ln(-1)
	}
	d.draw(cLine)
	d.SetLineWidth(0.2)
	d.ensure(rh * 3)
	header()
	for i, row := range rows {
		_, pageH := d.GetPageSize()
		if d.GetY()+rh > pageH-20 {
			d.AddPage()
			header()
		}
		fill := i%2 == 1
		if fill {
			d.fill(cSoft)
		}
		for j, c := range cols {
			v := row[j]
			d.font(8.5, v.bold, v.color)
			w := c.w * contentW
			d.CellFormat(w, rh, d.fit(v.text, w-2), "", 0, c.align, fill, 0, "")
		}
		d.Ln(-1)
	}
}

// lineChart plots values with a soft area fill, y gridlines and date labels.
func (d *doc) lineChart(vals []float64, labels []string, color rgb, h float64, fmtY func(float64) string) {
	if len(vals) < 2 {
		return
	}
	d.ensure(h + 10)
	x0, y0 := margin+16, d.GetY()+2
	w := contentW - 16
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, v := range vals {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	if lo > 0 {
		lo = 0
	}
	if hi <= lo {
		hi = lo + 1
	}
	px := func(i int) float64 { return x0 + w*float64(i)/float64(len(vals)-1) }
	py := func(v float64) float64 { return y0 + h - (v-lo)/(hi-lo)*h }

	// Grid + y labels.
	d.SetLineWidth(0.15)
	d.draw(cLine)
	for k := 0; k <= 4; k++ {
		v := lo + (hi-lo)*float64(k)/4
		y := py(v)
		d.Line(x0, y, x0+w, y)
		d.font(6.5, false, cMuted)
		d.SetXY(margin, y-2)
		d.CellFormat(15, 4, fmtY(v), "", 0, "R", false, 0, "")
	}
	// Area.
	pts := []fpdf.PointType{{X: px(0), Y: py(math.Max(lo, 0))}}
	for i, v := range vals {
		pts = append(pts, fpdf.PointType{X: px(i), Y: py(v)})
	}
	pts = append(pts, fpdf.PointType{X: px(len(vals) - 1), Y: py(math.Max(lo, 0))})
	d.SetAlpha(0.12, "Normal")
	d.fill(color)
	d.Polygon(pts, "F")
	d.SetAlpha(1, "Normal")
	// Line.
	d.SetLineWidth(0.6)
	d.draw(color)
	d.SetLineCapStyle("round")
	d.SetLineJoinStyle("round")
	for i := 1; i < len(vals); i++ {
		d.Line(px(i-1), py(vals[i-1]), px(i), py(vals[i]))
	}
	// X labels: first, middle, last.
	d.font(6.5, false, cMuted)
	for _, i := range []int{0, len(vals) / 2, len(vals) - 1} {
		lw := 24.0
		x := px(i) - lw/2
		align := "C"
		if i == 0 {
			x, align = px(0), "L"
		} else if i == len(vals)-1 {
			x, align = px(i)-lw, "R"
		}
		d.SetXY(x, y0+h+1)
		d.CellFormat(lw, 4, labels[i], "", 0, align, false, 0, "")
	}
	d.SetXY(margin, y0+h+7)
}

// bar is a small inline share bar drawn at the current row.
func (d *doc) bar(x, y, w, share float64, color rgb) {
	d.fill(cLine)
	d.RoundedRect(x, y, w, 1.8, 0.9, "1234", "F")
	if share > 0 {
		d.fill(color)
		d.RoundedRect(x, y, math.Max(1.8, w*math.Min(1, share)), 1.8, 0.9, "1234", "F")
	}
}

func (d *doc) bytes() ([]byte, error) {
	var buf bytes.Buffer
	if err := d.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func signColor(v int64) rgb {
	switch {
	case v > 0:
		return cPositive
	case v < 0:
		return cNegative
	}
	return cInk
}

func compact(v float64, cur string) string {
	sym := symbols[cur]
	if sym == "" {
		sym = cur + " "
	}
	a := math.Abs(v)
	sign := ""
	if v < 0 {
		sign = "−"
	}
	switch {
	case a >= 1e9:
		return fmt.Sprintf("%s%s%.1fB", sign, sym, a/1e9)
	case a >= 1e6:
		return fmt.Sprintf("%s%s%.1fM", sign, sym, a/1e6)
	case a >= 1e3:
		return fmt.Sprintf("%s%s%.0fk", sign, sym, a/1e3)
	}
	return fmt.Sprintf("%s%s%.0f", sign, sym, a)
}

// MonthPDF renders the monthly report.
func MonthPDF(m Month) ([]byte, error) {
	title := "Monthly report · " + m.Month.Format("January 2006")
	d := newDoc(title, fmt.Sprintf("%d transactions · amounts in %s", len(m.Transactions), m.Currency), m.GeneratedAt)
	cur := m.Currency
	items := []kpi{
		{"Income", Money(m.IncomeCents, cur), cPositive},
		{"Expenses", Money(m.ExpenseCents, cur), cNegative},
		{"Net", Money(m.NetCents, cur), signColor(m.NetCents)},
		{"Savings rate", fmt.Sprintf("%.0f%%", m.SavingsRate*100), cInk},
	}
	if m.HasForecast {
		items[3] = kpi{"Month-end forecast", Money(m.ProjectedNetCents, cur), signColor(m.ProjectedNetCents)}
	}
	d.kpis(items)

	if len(m.DailySpend) > 1 {
		d.section("Daily spending")
		vals := make([]float64, len(m.DailySpend))
		labels := make([]string, len(m.DailySpend))
		div := math.Pow10(minor(cur))
		for i, v := range m.DailySpend {
			vals[i] = float64(v) / div
			labels[i] = m.Month.AddDate(0, 0, i).Format("2 Jan")
		}
		d.lineChart(vals, labels, cPrimary, 38, func(v float64) string { return compact(v, cur) })
	}

	if len(m.Categories) > 0 {
		d.section("Spending by category")
		cols := []col{{"Category", 0.36, "L"}, {"", 0.24, "L"}, {"Share", 0.1, "R"}, {"Spent", 0.15, "R"}, {"Budget", 0.15, "R"}}
		var rows [][]cell
		for _, c := range m.Categories {
			share := 0.0
			if m.ExpenseCents > 0 {
				share = float64(c.SpentCents) / float64(m.ExpenseCents)
			}
			budget := cell{text: "—", color: cMuted}
			if c.BudgetCents > 0 {
				budget = plain(Money(c.BudgetCents, cur))
				if c.SpentCents > c.BudgetCents {
					budget.color = cNegative
				}
			}
			rows = append(rows, []cell{plain(c.Name), {}, plain(fmt.Sprintf("%.0f%%", share*100)),
				{text: Money(c.SpentCents, cur), color: cInk, bold: true}, budget})
		}
		startY := d.GetY() + 6.2
		startPage := d.PageNo()
		d.table(cols, rows)
		if d.PageNo() == startPage { // inline share bars (skip if the table broke across pages)
			for i, c := range m.Categories {
				share := 0.0
				if m.ExpenseCents > 0 {
					share = float64(c.SpentCents) / float64(m.ExpenseCents)
				}
				d.bar(margin+0.36*contentW, startY+float64(i)*6.2+2.2, 0.22*contentW, share, cPrimary)
			}
		}
	}

	if len(m.Merchants) > 0 {
		d.section("Top merchants")
		var rows [][]cell
		for _, x := range m.Merchants {
			rows = append(rows, []cell{plain(x.Name), plain(fmt.Sprint(x.Count)), {text: Money(x.SpentCents, cur), color: cInk, bold: true}})
		}
		d.table([]col{{"Merchant", 0.6, "L"}, {"Payments", 0.2, "R"}, {"Spent", 0.2, "R"}}, rows)
	}

	d.section("Transactions")
	if len(m.Transactions) == 0 {
		d.font(9, false, cMuted)
		d.CellFormat(contentW, 6, "No transactions this month.", "", 1, "L", false, 0, "")
	} else {
		var rows [][]cell
		for _, t := range m.Transactions {
			desc := t.Description
			if t.Merchant != "" && !strings.Contains(strings.ToLower(desc), strings.ToLower(t.Merchant)) {
				desc = t.Merchant + " · " + desc
			}
			amt := cell{text: Money(t.AmountCents, t.Currency), color: signColor(t.AmountCents), bold: true}
			if t.AmountCents > 0 {
				amt.text = "+" + amt.text
			}
			rows = append(rows, []cell{{text: t.Date.Format("02.01"), color: cMuted}, plain(desc), {text: t.Category, color: cMuted}, amt})
		}
		d.table([]col{{"Date", 0.09, "L"}, {"Description", 0.53, "L"}, {"Category", 0.18, "L"}, {"Amount", 0.2, "R"}}, rows)
	}
	return d.bytes()
}

// PortfolioPDF renders net worth, investments and accounts.
func PortfolioPDF(p Portfolio) ([]byte, error) {
	cur := p.Currency
	d := newDoc("Net worth & portfolio", "As of "+p.GeneratedAt.Format("2 January 2006")+" · amounts in "+cur, p.GeneratedAt)
	d.kpis([]kpi{
		{"Net worth", Money(p.NetCents, cur), signColor(p.NetCents)},
		{"Assets", Money(p.AssetsCents, cur), cInk},
		{"Liabilities", Money(p.LiabilitiesCents, cur), cNegative},
		{"Investments", Money(p.InvestmentsCents, cur), cInk},
	})

	if len(p.NetHistory) > 1 {
		d.section("Net worth over time")
		vals := make([]float64, len(p.NetHistory))
		labels := make([]string, len(p.NetHistory))
		div := math.Pow10(minor(cur))
		for i, x := range p.NetHistory {
			vals[i] = float64(x.Cents) / div
			labels[i] = x.Date.Format("2 Jan 2006")
		}
		d.lineChart(vals, labels, cPositive, 38, func(v float64) string { return compact(v, cur) })
	}

	d.section("Composition")
	comp := [][]cell{}
	add := func(name string, v int64, liability bool) {
		if v == 0 {
			return
		}
		c := cell{text: Money(v, cur), color: cInk, bold: true}
		if liability {
			c.text, c.color = "−"+Money(v, cur), cNegative
		}
		comp = append(comp, []cell{plain(name), c})
	}
	add("Cash, cards & savings", p.CashCents, false)
	add("Investments", p.InvestmentsCents, false)
	add("Property", p.PropertyCents, false)
	add("Friends owe you", p.OwedToMeCents, false)
	add("Credit cards & loans", p.CreditCents, true)
	add("You owe friends", p.IOweCents, true)
	comp = append(comp, []cell{{text: "Net worth", color: cInk, bold: true}, {text: Money(p.NetCents, cur), color: signColor(p.NetCents), bold: true}})
	d.table([]col{{"", 0.7, "L"}, {"Amount", 0.3, "R"}}, comp)

	if len(p.Holdings) > 0 {
		d.section("Investments")
		var rows [][]cell
		for _, h := range p.Holdings {
			detail := ""
			if h.Symbol != "" {
				detail = fmt.Sprintf("%s · %s × %s", h.Symbol, trimFloat(h.Quantity), trimFloat(h.Price))
			} else {
				detail = fmt.Sprintf("%s%% (eff. %.2f%%)", trimFloat(h.RatePercent), h.EffectiveRatePercent)
				if h.MaturityAt != nil {
					detail += " · until " + h.MaturityAt.Format("02.01.2006")
				}
			}
			gain := cell{text: fmt.Sprintf("%+.2f%%", h.GainPercent), color: signColor(int64(math.Round(h.GainPercent * 100)))}
			rows = append(rows, []cell{plain(h.Name), {text: h.Kind, color: cMuted}, {text: detail, color: cMuted},
				plain(Money(h.InvestedCents, h.Currency)), {text: Money(h.ValueHomeCents, cur), color: cInk, bold: true}, gain})
		}
		d.table([]col{{"Name", 0.22, "L"}, {"Kind", 0.1, "L"}, {"Details", 0.27, "L"}, {"Invested", 0.15, "R"},
			{"Value", 0.16, "R"}, {"Gain", 0.1, "R"}}, rows)
		d.font(7.5, false, cMuted)
		d.Ln(1)
		d.MultiCell(contentW, 3.6, fmt.Sprintf("Total gain %s (%+.2f%%). Market prices from Yahoo Finance, may be delayed.",
			Money(p.GainCents, cur), p.GainPercent), "", "L", false)
	}

	if len(p.Accounts) > 0 {
		d.section("Accounts")
		var rows [][]cell
		for _, a := range p.Accounts {
			v := cell{text: Money(a.BalanceHomeCents, cur), color: cInk, bold: true}
			if a.Liability {
				v.text, v.color = "−"+v.text, cNegative
			}
			orig := ""
			if a.Currency != cur {
				orig = Money(a.BalanceCents, a.Currency)
			}
			rows = append(rows, []cell{plain(a.Name), {text: a.Kind, color: cMuted}, {text: orig, color: cMuted}, v})
		}
		d.table([]col{{"Account", 0.4, "L"}, {"Kind", 0.2, "L"}, {"Original", 0.2, "R"}, {"Balance", 0.2, "R"}}, rows)
	}
	return d.bytes()
}

func trimFloat(v float64) string {
	s := fmt.Sprintf("%.6f", v)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return s
}
