package statement

import "testing"

func TestParseCSVChase(t *testing.T) {
	data := []byte(`Details,Posting Date,Description,Amount,Type,Balance,Check or Slip #
DEBIT,05/14/2024,"WAL-MART #1234 SPRINGFIELD IL",-20.74,DEBIT_CARD,1000.00,,
CREDIT,05/15/2024,"ACME CORP PAYROLL",2500.00,ACH_CREDIT,3500.00,,
`)
	rows, err := Parse(data, FormatAuto)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d", len(rows))
	}
	if rows[0].AmountCents != -2074 || rows[0].Date.Format("2006-01-02") != "2024-05-14" {
		t.Errorf("row0 = %+v", rows[0])
	}
	if rows[0].Merchant != "WAL-MART SPRINGFIELD IL" {
		t.Errorf("merchant = %q", rows[0].Merchant)
	}
	if rows[1].AmountCents != 250000 {
		t.Errorf("row1 = %+v", rows[1])
	}
}

func TestParseCSVDebitCredit(t *testing.T) {
	data := []byte("Date,Description,Debit,Credit\n2024-05-01,Netflix,15.49,\n2024-05-02,Refund,,\"$1,000.10\"\n")
	rows, err := ParseCSV(data)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].AmountCents != -1549 || rows[1].AmountCents != 100010 {
		t.Errorf("rows = %+v", rows)
	}
}

func TestParseCSVNoHeader(t *testing.T) {
	data := []byte("\"05/14/2024\",\"-12.50\",\"*\",\"\",\"STARBUCKS STORE 123\"\n")
	rows, err := ParseCSV(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].AmountCents != -1250 || rows[0].Description != "STARBUCKS STORE 123" {
		t.Errorf("rows = %+v", rows)
	}
}

func TestParseOFX(t *testing.T) {
	data := []byte(`OFXHEADER:100
<OFX><BANKMSGSRSV1><STMTTRNRS><STMTRS><BANKTRANLIST>
<STMTTRN><TRNTYPE>DEBIT<DTPOSTED>20240514120000[-5:EST]<TRNAMT>-20.74<FITID>abc123<NAME>WALMART SUPERCENTER</STMTTRN>
<STMTTRN><TRNTYPE>CREDIT<DTPOSTED>20240515<TRNAMT>2500.00<FITID>abc124<NAME>PAYROLL</STMTTRN>
</BANKTRANLIST></STMTRS></STMTTRNRS></BANKMSGSRSV1></OFX>`)
	rows, err := Parse(data, FormatAuto)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].AmountCents != -2074 || rows[0].DedupKey != "fitid:abc123" {
		t.Errorf("rows = %+v", rows)
	}
}

func TestParseAmount(t *testing.T) {
	cases := map[string]int64{"(12.34)": -1234, "-5": -500, "$1,234.5": 123450, "0.07": 7}
	for in, want := range cases {
		if got, err := parseAmount(in); err != nil || got != want {
			t.Errorf("parseAmount(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
}
