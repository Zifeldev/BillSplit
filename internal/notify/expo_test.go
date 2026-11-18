package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type expoStub struct {
	mu       sync.Mutex
	batches  []int
	auth     []string
	dead     map[string]bool
	status   int
	messages []PushMessage
}

func (s *expoStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var msgs []PushMessage
	if err := json.NewDecoder(r.Body).Decode(&msgs); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.batches = append(s.batches, len(msgs))
	s.auth = append(s.auth, r.Header.Get("Authorization"))
	s.messages = append(s.messages, msgs...)
	s.mu.Unlock()
	if s.status != 0 {
		w.WriteHeader(s.status)
		_, _ = w.Write([]byte(`{"errors":[{"code":"INTERNAL_SERVER_ERROR","message":"boom"}]}`))
		return
	}
	tickets := make([]map[string]any, len(msgs))
	for i, m := range msgs {
		if s.dead[m.To] {
			tickets[i] = map[string]any{"status": "error", "message": "not registered",
				"details": map[string]any{"error": "DeviceNotRegistered"}}
		} else {
			tickets[i] = map[string]any{"status": "ok", "id": fmt.Sprint("ticket-", i)}
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"data": tickets})
}

func msgs(n int) []PushMessage {
	out := make([]PushMessage, n)
	for i := range out {
		out[i] = PushMessage{To: fmt.Sprintf("ExponentPushToken[t%d]", i), Title: "hi", Body: "b",
			Data: map[string]string{"k": "v"}, Sound: "default"}
	}
	return out
}

func TestExpoBatching(t *testing.T) {
	stub := &expoStub{}
	srv := httptest.NewServer(stub)
	defer srv.Close()

	res := NewExpoClient(srv.URL, "").Send(context.Background(), msgs(250))
	if len(res) != 250 {
		t.Fatalf("results = %d, want 250", len(res))
	}
	for i, r := range res {
		if !r.OK || r.Token != fmt.Sprintf("ExponentPushToken[t%d]", i) {
			t.Fatalf("result %d = %+v", i, r)
		}
	}
	if fmt.Sprint(stub.batches) != "[100 100 50]" {
		t.Fatalf("batches = %v", stub.batches)
	}
	if stub.auth[0] != "" {
		t.Fatalf("authorization header sent without token: %q", stub.auth[0])
	}
	m := stub.messages[0]
	if m.Sound != "default" || m.Data["k"] != "v" {
		t.Fatalf("message payload = %+v", m)
	}
}

func TestExpoAccessToken(t *testing.T) {
	stub := &expoStub{}
	srv := httptest.NewServer(stub)
	defer srv.Close()

	NewExpoClient(srv.URL, "secret").Send(context.Background(), msgs(1))
	if stub.auth[0] != "Bearer secret" {
		t.Fatalf("authorization = %q", stub.auth[0])
	}
}

func TestExpoDeviceNotRegistered(t *testing.T) {
	stub := &expoStub{dead: map[string]bool{"ExponentPushToken[t1]": true, "ExponentPushToken[t120]": true}}
	srv := httptest.NewServer(stub)
	defer srv.Close()

	res := NewExpoClient(srv.URL, "").Send(context.Background(), msgs(150))
	var dead []string
	for _, r := range res {
		if r.Unregistered {
			if r.OK || r.Error != "DeviceNotRegistered" {
				t.Fatalf("bad dead result %+v", r)
			}
			dead = append(dead, r.Token)
		} else if !r.OK {
			t.Fatalf("unexpected failure %+v", r)
		}
	}
	if fmt.Sprint(dead) != "[ExponentPushToken[t1] ExponentPushToken[t120]]" {
		t.Fatalf("dead = %v", dead)
	}
}

func TestExpoHTTPErrorFailsBatch(t *testing.T) {
	stub := &expoStub{status: http.StatusInternalServerError}
	srv := httptest.NewServer(stub)
	defer srv.Close()

	res := NewExpoClient(srv.URL, "").Send(context.Background(), msgs(3))
	for _, r := range res {
		if r.OK || r.Unregistered || r.Error == "" {
			t.Fatalf("want failure, got %+v", r)
		}
	}
}

func TestValidToken(t *testing.T) {
	for tok, want := range map[string]bool{
		"ExponentPushToken[xxxxxxxxxxxxxxxxxxxxxx]": true,
		"ExpoPushToken[abc-DEF_123]":                true,
		"ExponentPushToken[]":                       false,
		"fcm-token":                                 false,
		"ExponentPushToken[a b]":                    false,
		"xExponentPushToken[abc]":                   false,
	} {
		if got := ValidToken(tok); got != want {
			t.Errorf("ValidToken(%q) = %v, want %v", tok, got, want)
		}
	}
}
