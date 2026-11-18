package ratelimit

import "testing"

func TestLimiter(t *testing.T) {
	l := New(100, 3, 1, 1)
	if !l.Allow("ip", true) {
		t.Fatal("first strict request must pass")
	}
	if l.Allow("ip", true) {
		t.Fatal("second strict request must be limited")
	}
	if !l.Allow("ip", false) {
		t.Fatal("regular request must still pass")
	}
	if !l.Allow("other-ip", true) {
		t.Fatal("limits are per key")
	}
}
