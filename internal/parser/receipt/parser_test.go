package receipt

import (
	"testing"
)

const walmartReceipt = `
Walmart
Save money. Live better.
( 555 ) 555 - 5555
MANAGER JOHN DOE
1234 MAIN ST
SPRINGFIELD IL 62701
ST# 01234 OP# 000012 TE# 12 TR# 01234
BANANAS 000000004011 F 1.96 N
2 AT 1 FOR 0.98
GV MILK 2% 007874235187 F 3.48 N
TIDE PODS 003700087754 12.97 X
ROLLBACK 1.00-
AVOCADO 000000004046 F 2.50 N
SUBTOTAL 19.91
TAX 1 7.000 % 0.83
TOTAL 20.74
VISA TEND 20.74
ACCOUNT # **** **** **** 1234
CHANGE DUE 0.00
# ITEMS SOLD 4
05/14/24 14:32:10
`

func TestParseTextWalmart(t *testing.T) {
	p, err := ParseText(walmartReceipt)
	if err != nil {
		t.Fatal(err)
	}
	if p.Store != "Walmart" {
		t.Errorf("store = %q", p.Store)
	}
	type item struct {
		name, code string
		qty        int32
		cents      int64
	}
	want := []item{
		{"BANANAS", "000000004011", 2, 196},
		{"GV MILK 2%", "007874235187", 1, 348},
		{"TIDE PODS", "003700087754", 1, 1197}, // rollback applied
		{"AVOCADO", "000000004046", 1, 250},
	}
	if len(p.Items) != len(want) {
		t.Fatalf("items = %+v", p.Items)
	}
	for i, w := range want {
		got := item{p.Items[i].RawName, p.Items[i].Code, p.Items[i].Quantity, p.Items[i].PriceCents}
		if got != w || p.Items[i].Name != w.name {
			t.Errorf("item %d = %+v, want %+v", i, p.Items[i], w)
		}
	}
	if p.SubtotalCents != 1991 || p.TaxCents != 83 || p.TotalCents != 2074 {
		t.Errorf("totals = %d/%d/%d", p.SubtotalCents, p.TaxCents, p.TotalCents)
	}
	if len(p.Warnings) != 0 {
		t.Errorf("warnings: %v", p.Warnings)
	}
	if p.PurchasedAt == nil || p.PurchasedAt.Format("2006-01-02") != "2024-05-14" {
		t.Errorf("date = %v", p.PurchasedAt)
	}
}

func TestParseTextNoItems(t *testing.T) {
	if _, err := ParseText("hello\nworld"); err != ErrNoItems {
		t.Fatalf("err = %v", err)
	}
}

func TestToCents(t *testing.T) {
	cases := map[string]int64{"12.34": 1234, "$1,234.56": 123456, "-0.99": -99, "3,50": 350}
	for in, want := range cases {
		got, err := toCents(in)
		if err != nil || got != want {
			t.Errorf("toCents(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
}

// Real-world shape: skewed phone photo after row de-rotation, one price lost a digit,
// a voided entry without price. Codes are kept for product lookup.
const walmartSkewed = `Walmart
WM Supercenter
ITEMS SOLD 4
EQ MTT 240 681131105260 H 6.98 X
BRM OATS 039978004780 4.8
GV 41-60 CK 078742133710 F 6.97 N
BRAT BUN 299689000000 F 2.46 N
** VOIDED ENTRY **
BRAT BUN 299689000000 F
SUBTOTAL 21.29
TAX1 8.7000 % 1.85
TOTAL 23.14
09/27/26 18:49:15`

func TestParseTextRepairsDroppedDigit(t *testing.T) {
	p, err := ParseText(walmartSkewed)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 4 {
		t.Fatalf("items = %+v", p.Items)
	}
	if p.Items[1].PriceCents != 488 || p.Items[1].Code != "039978004780" || p.Items[1].RawName != "BRM OATS" {
		t.Errorf("oats = %+v, want repaired 4.88 with code", p.Items[1])
	}
	if len(p.Warnings) != 0 {
		t.Errorf("warnings: %v", p.Warnings)
	}
	if p.TotalCents != 2314 || p.TaxCents != 185 {
		t.Errorf("totals = %d/%d", p.TotalCents, p.TaxCents)
	}
}

func TestParseTextTargetCodeFirstAndGluedCode(t *testing.T) {
	p, err := ParseText(`TARGET
212000351 GOOD&GATHER MILK NF $3.49
490970134 UP&UP PAPER TOWEL T $8.99
TIDE PODS 00370008775411.97 X
SUBTOTAL $24.45
T = AZ TAX 8.7000 on $8.99 $0.78
TOTAL $25.23`)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 3 || p.Items[0].RawName != "GOOD&GATHER MILK" || p.Items[0].Code != "212000351" {
		t.Fatalf("items = %+v", p.Items)
	}
	if p.Items[2].RawName != "TIDE PODS" || p.Items[2].Code != "003700087754" || p.Items[2].PriceCents != 1197 {
		t.Errorf("glued = %+v", p.Items[2])
	}
	if p.TaxCents != 78 || len(p.Warnings) != 0 {
		t.Errorf("tax = %d, warnings = %v", p.TaxCents, p.Warnings)
	}
}

// Walmart prints a tax flag "O" after some prices; OCR reads it as "0" and
// sometimes glues it ("0.990"), or drops a digit ("0.9" + "0" -> "0.90").
func TestParseTextZeroFlagAndSiblings(t *testing.T) {
	p, err := ParseText(`Walmart
GV WHOLE HG 078742352000 F 1.59 0
ELBOW PASTA 076808516130 F 0.99 0
ELBOW PASTA 076808516130 F 0.990
ELBOW PASTA 076808516130 F 0.90
ORG RSPBERRY 750301807430 F 3.98 0
SUBTOTAL 8.54
TOTAL 8.54`)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 5 {
		t.Fatalf("items = %+v", p.Items)
	}
	for _, i := range []int{1, 2, 3} {
		if p.Items[i].PriceCents != 99 {
			t.Errorf("pasta %d = %d, want 99", i, p.Items[i].PriceCents)
		}
	}
	if len(p.Warnings) != 0 {
		t.Errorf("warnings: %v", p.Warnings)
	}
}
