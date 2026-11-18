package fx

import "testing"

func TestApplyMinorUnits(t *testing.T) {
	cases := []struct {
		cents    int64
		rate     float64
		from, to string
		want     int64
	}{
		{1000, 1.08, "EUR", "USD", 1080},  // €10.00 -> $10.80
		{1000, 150, "USD", "JPY", 1500},   // $10.00 -> ¥1500 (JPY has no cents)
		{1500, 0.0066, "JPY", "USD", 990}, // ¥1500 -> $9.90
		{12345, 1, "USD", "USD", 12345},
	}
	for _, c := range cases {
		if got := Apply(c.cents, c.rate, c.from, c.to); got != c.want {
			t.Errorf("Apply(%d, %v, %s->%s) = %d, want %d", c.cents, c.rate, c.from, c.to, got, c.want)
		}
	}
	if FromTwoDecimals(128000, "JPY") != 1280 || FromTwoDecimals(1999, "USD") != 1999 {
		t.Error("FromTwoDecimals")
	}
}

func TestMinorUnitsThreeDecimals(t *testing.T) {
	if MinorUnits("KZT") != 2 || MinorUnits("KWD") != 3 || FromTwoDecimals(1250, "KWD") != 12500 {
		t.Error("minor units")
	}
	if got := Apply(100000, 0.0025, "KZT", "KWD"); got != 2500 { // 1000.00 ₸ -> 2.500 KWD
		t.Errorf("Apply KZT->KWD = %d", got)
	}
}
