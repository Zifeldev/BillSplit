package ocrworker

import "testing"

func TestClaudeReceiptToParsed(t *testing.T) {
	r := claudeReceipt{Store: "Walmart", SubtotalCents: 598, TaxCents: 42, PurchaseDate: "2024-05-14"}
	r.Items = append(r.Items,
		claudeItem{Name: "Bananas", RawName: "BANANAS", Code: "000000004011", Quantity: 2, PriceCents: 250},
		claudeItem{Name: "Milk", Quantity: 0, PriceCents: 348},
	)
	p, err := r.toParsed()
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 2 || p.Items[1].Quantity != 1 || p.TotalCents != 640 || len(p.Warnings) != 0 {
		t.Errorf("parsed = %+v", p)
	}
	if p.Items[0].RawName != "BANANAS" || p.Items[1].RawName != "Milk" {
		t.Errorf("raw names = %q %q", p.Items[0].RawName, p.Items[1].RawName)
	}
	if p.PurchasedAt == nil || p.PurchasedAt.Format("2006-01-02") != "2024-05-14" {
		t.Errorf("date = %v", p.PurchasedAt)
	}
	if _, err := (claudeReceipt{}).toParsed(); err == nil {
		t.Error("empty receipt must fail")
	}
}
