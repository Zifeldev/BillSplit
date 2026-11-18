package authsvc

import (
	"errors"
	"testing"

	"github.com/Zifeldev/Billsplit/internal/domain"
)

func TestNormalizeHandle(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"alice", "alice", true},
		{"@Alice_Smith-1", "Alice_Smith-1", true},
		{"  @bob ", "bob", true},
		{"", "", true},
		{"@", "", true}, // just "@" clears too
		{"has space", "", false},
		{"dot.name", "", false},
		{"0123456789012345678901234567890", "", false}, // 31 chars
		{"@@double", "", false},
	}
	for _, c := range cases {
		got, err := NormalizeHandle("venmo", c.in)
		if c.ok != (err == nil) || got != c.want {
			t.Errorf("NormalizeHandle(%q) = %q, %v; want %q ok=%v", c.in, got, err, c.want, c.ok)
		}
		if err != nil && !errors.Is(err, domain.ErrInvalidArgument) {
			t.Errorf("want ErrInvalidArgument, got %v", err)
		}
	}
}

func TestNormalizeZelle(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"", "", true},
		{"Alice@Example.com", "alice@example.com", true},
		{"(415) 555-0123", "4155550123", true},
		{"+1 415.555.0123", "4155550123", true},
		{"14155550123", "4155550123", true},
		{"415-555-012", "", false},  // 9 digits
		{"015-555-0123", "", false}, // bad area code
		{"415-155-0123", "", false}, // bad exchange
		{"+44 20 7946 0958", "", false},
		{"Alice <a@b.com>", "", false},
		{"not an email@", "", false},
		{"call me", "", false},
	}
	for _, c := range cases {
		got, err := NormalizeZelle(c.in)
		if c.ok != (err == nil) || got != c.want {
			t.Errorf("NormalizeZelle(%q) = %q, %v; want %q ok=%v", c.in, got, err, c.want, c.ok)
		}
	}
}

func TestPaymentUpdateApply(t *testing.T) {
	p := PaymentHandles{Venmo: "old", PayPal: "keep", Zelle: "4155550123"}
	venmo, zelle := "@new", ""
	if err := (PaymentUpdate{Venmo: &venmo, Zelle: &zelle}).apply(&p); err != nil {
		t.Fatal(err)
	}
	want := PaymentHandles{Venmo: "new", PayPal: "keep", Zelle: ""}
	if p != want {
		t.Fatalf("got %+v, want %+v", p, want)
	}
	bad := "bad handle"
	if err := (PaymentUpdate{PayPal: &bad}).apply(&p); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("want ErrInvalidArgument, got %v", err)
	}
}
