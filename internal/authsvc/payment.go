package authsvc

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"

	"github.com/Zifeldev/Billsplit/internal/domain"
)

// PaymentHandles are used by clients to build "pay back" deep links. Empty = not set.
type PaymentHandles struct {
	Venmo, PayPal, Zelle string
}

// PaymentUpdate carries the handles to change; nil keeps the stored value, "" clears it.
type PaymentUpdate struct {
	Venmo, PayPal, Zelle *string
}

var handleRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,30}$`)

// NormalizeHandle validates a Venmo / PayPal.me username (a leading @ is dropped).
func NormalizeHandle(field, v string) (string, error) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "@")
	if v == "" {
		return "", nil
	}
	if !handleRe.MatchString(v) {
		return "", fmt.Errorf("%w: %s must be 1..30 letters, digits, _ or -", domain.ErrInvalidArgument, field)
	}
	return v, nil
}

// NormalizeZelle accepts an email (lower-cased) or a US phone number
// (normalized to 10 digits, an optional +1 / 1 country code is dropped).
func NormalizeZelle(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil
	}
	if strings.Contains(v, "@") {
		a, err := mail.ParseAddress(v)
		if err != nil || a.Address != v || len(v) > 254 {
			return "", fmt.Errorf("%w: zelle must be an email or a US phone number", domain.ErrInvalidArgument)
		}
		return strings.ToLower(v), nil
	}
	var digits strings.Builder
	for _, r := range v {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case strings.ContainsRune(" +-().", r):
		default:
			return "", fmt.Errorf("%w: zelle must be an email or a US phone number", domain.ErrInvalidArgument)
		}
	}
	d := digits.String()
	if len(d) == 11 && d[0] == '1' {
		d = d[1:]
	}
	if len(d) != 10 || d[0] < '2' || d[3] < '2' { // NANP: area code and exchange can't start with 0/1
		return "", fmt.Errorf("%w: zelle must be an email or a US phone number", domain.ErrInvalidArgument)
	}
	return d, nil
}

// apply validates the provided handles and writes them into p.
func (u PaymentUpdate) apply(p *PaymentHandles) error {
	var err error
	if u.Venmo != nil {
		if p.Venmo, err = NormalizeHandle("venmo", *u.Venmo); err != nil {
			return err
		}
	}
	if u.PayPal != nil {
		if p.PayPal, err = NormalizeHandle("paypal", *u.PayPal); err != nil {
			return err
		}
	}
	if u.Zelle != nil {
		if p.Zelle, err = NormalizeZelle(*u.Zelle); err != nil {
			return err
		}
	}
	return nil
}
