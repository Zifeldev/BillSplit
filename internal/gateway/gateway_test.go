package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Zifeldev/Billsplit/internal/platform/auth"
	"github.com/Zifeldev/Billsplit/internal/platform/ratelimit"
	pb "github.com/Zifeldev/Billsplit/pkg/pb/billsplit/v1"
)

func TestClientIP(t *testing.T) {
	cases := []struct {
		name, remote, xff, want string
	}{
		{"no xff", "203.0.113.7:5000", "", "203.0.113.7"},
		{"public peer can't spoof", "203.0.113.7:5000", "1.2.3.4", "203.0.113.7"},
		{"private proxy", "10.0.0.5:5000", "198.51.100.9, 10.0.0.1", "198.51.100.9"},
		{"loopback proxy", "127.0.0.1:5000", " 198.51.100.9 ", "198.51.100.9"},
		{"ipv6 loopback proxy", "[::1]:5000", "2001:db8::1", "2001:db8::1"},
		{"docker bridge", "172.18.0.3:5000", "198.51.100.9", "198.51.100.9"},
		{"garbage xff", "10.0.0.5:5000", "not-an-ip", "10.0.0.5"},
		{"no port", "10.0.0.5", "198.51.100.9", "198.51.100.9"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", nil)
			r.RemoteAddr = c.remote
			if c.xff != "" {
				r.Header.Set("X-Forwarded-For", c.xff)
			}
			if got := clientIP(r); got != c.want {
				t.Fatalf("clientIP = %q, want %q", got, c.want)
			}
		})
	}
}

func TestAuthenticate(t *testing.T) {
	jwt := auth.NewJWTManager("test-secret", time.Minute, time.Hour)
	tp, err := jwt.Issue("user-1")
	if err != nil {
		t.Fatal(err)
	}
	var seen string
	h := authenticate(jwt, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = auth.UserID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	cases := []struct {
		name, path, token string
		status            int
		uid               string
	}{
		{"public anonymous", pb.GroupService_GetInvite_FullMethodName, "", 200, ""},
		{"public with token keeps user", pb.GroupService_GetInvite_FullMethodName, tp.Access, 200, "user-1"},
		{"public with bad token is anonymous", pb.GroupService_GetInvite_FullMethodName, "garbage", 200, ""},
		{"currencies public", pb.FinanceService_ListCurrencies_FullMethodName, "", 200, ""},
		{"login public", pb.AuthService_Login_FullMethodName, "", 200, ""},
		{"private needs token", pb.GroupService_GetGroup_FullMethodName, "", 401, ""},
		{"private bad token", pb.GroupService_GetGroup_FullMethodName, "garbage", 401, ""},
		{"refresh token is not an access token", pb.GroupService_GetGroup_FullMethodName, tp.Refresh, 401, ""},
		{"private ok", pb.GroupService_GetGroup_FullMethodName, tp.Access, 200, "user-1"},
		{"logout needs auth", pb.AuthService_Logout_FullMethodName, "", 401, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seen = ""
			r := httptest.NewRequest(http.MethodPost, c.path, nil)
			if c.token != "" {
				r.Header.Set("Authorization", "Bearer "+c.token)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != c.status || seen != c.uid {
				t.Fatalf("status=%d uid=%q, want %d %q", w.Code, seen, c.status, c.uid)
			}
		})
	}
}

// rejectStrict lets regular requests through and rejects every strict one.
type rejectStrict struct{}

func (rejectStrict) Allow(_ string, strict bool) bool       { return !strict }
func (rejectStrict) Cleanup(context.Context, time.Duration) {}

var (
	_ Limiter = (*ratelimit.Limiter)(nil)
	_ Limiter = (*ratelimit.Redis)(nil)
)

func TestStrictLimitOnlyForCredentials(t *testing.T) {
	h := rateLimit(rejectStrict{}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	for path, wantStrict := range map[string]bool{
		pb.AuthService_Login_FullMethodName:             true,
		pb.AuthService_Register_FullMethodName:          true,
		pb.AuthService_Refresh_FullMethodName:           true,
		pb.AuthService_Logout_FullMethodName:            false,
		pb.GroupService_GetInvite_FullMethodName:        false,
		pb.FinanceService_ListCurrencies_FullMethodName: false,
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))
		if limited := w.Code == http.StatusTooManyRequests; limited != wantStrict {
			t.Errorf("%s: strict=%v, want %v", path, limited, wantStrict)
		}
	}
}
