package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// ExpoPushURL is the Expo push API endpoint.
	ExpoPushURL = "https://exp.host/--/api/v2/push/send"
	// expoBatchSize is the max number of messages Expo accepts per request.
	expoBatchSize = 100
)

// PushMessage is one Expo push message (one device).
type PushMessage struct {
	To    string            `json:"to"`
	Title string            `json:"title"`
	Body  string            `json:"body"`
	Data  map[string]string `json:"data,omitempty"`
	Sound string            `json:"sound,omitempty"`
}

// PushResult is the outcome for one message, in the same order as the input.
type PushResult struct {
	Token string
	OK    bool
	// Error is the Expo error code (details.error) or a transport error description.
	Error string
	// Unregistered is true when Expo reported DeviceNotRegistered: the token must be dropped.
	Unregistered bool
}

// Pusher sends push messages. Implemented by ExpoClient; faked in tests.
type Pusher interface {
	Send(ctx context.Context, msgs []PushMessage) []PushResult
}

// ExpoClient talks to the Expo push service.
type ExpoClient struct {
	url         string
	accessToken string
	http        *http.Client
}

// NewExpoClient builds a client. accessToken is optional (Expo "enhanced push security");
// url defaults to ExpoPushURL.
func NewExpoClient(url, accessToken string) *ExpoClient {
	if url == "" {
		url = ExpoPushURL
	}
	return &ExpoClient{url: url, accessToken: accessToken, http: &http.Client{Timeout: 15 * time.Second}}
}

type expoTicket struct {
	Status  string `json:"status"` // ok | error
	ID      string `json:"id"`
	Message string `json:"message"`
	Details struct {
		Error string `json:"error"`
	} `json:"details"`
}

type expoResponse struct {
	Data   []expoTicket `json:"data"`
	Errors []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

// Send pushes msgs in batches of up to 100. It never returns an error: every
// message gets a PushResult (a failed request fails all messages of its batch).
func (c *ExpoClient) Send(ctx context.Context, msgs []PushMessage) []PushResult {
	out := make([]PushResult, 0, len(msgs))
	for start := 0; start < len(msgs); start += expoBatchSize {
		end := min(start+expoBatchSize, len(msgs))
		out = append(out, c.sendBatch(ctx, msgs[start:end])...)
	}
	return out
}

func (c *ExpoClient) sendBatch(ctx context.Context, batch []PushMessage) []PushResult {
	res := make([]PushResult, len(batch))
	for i, m := range batch {
		res[i].Token = m.To
	}
	failAll := func(reason string) []PushResult {
		for i := range res {
			res[i].Error = reason
		}
		return res
	}

	body, err := json.Marshal(batch)
	if err != nil {
		return failAll("marshal: " + err.Error())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return failAll("request: " + err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return failAll("transport: " + err.Error())
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return failAll("read: " + err.Error())
	}

	var er expoResponse
	_ = json.Unmarshal(raw, &er)
	if resp.StatusCode != http.StatusOK {
		reason := fmt.Sprintf("http %d", resp.StatusCode)
		if len(er.Errors) > 0 {
			reason += ": " + er.Errors[0].Code
		}
		return failAll(reason)
	}
	if len(er.Data) != len(batch) {
		return failAll(fmt.Sprintf("unexpected ticket count %d for %d messages", len(er.Data), len(batch)))
	}
	for i, t := range er.Data {
		if t.Status == "ok" {
			res[i].OK = true
			continue
		}
		res[i].Error = t.Details.Error
		if res[i].Error == "" {
			res[i].Error = "unknown"
		}
		res[i].Unregistered = t.Details.Error == "DeviceNotRegistered"
	}
	return res
}
