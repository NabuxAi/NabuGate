package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRewriteLiveModelReplacesTheAliasWithTheUpstreamModel(t *testing.T) {
	out, err := rewriteLiveModel(json.RawMessage(
		`{"model":"nabu-live","session":{"model":"nabu-live","instructions":"be brief"},"transport":{"type":"webrtc","sdp":"v=0 offer"}}`,
	), "gpt-live-1")
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(out, &top); err != nil {
		t.Fatal(err)
	}
	if _, leaked := top["model"]; leaked {
		t.Fatalf("the gateway alias reached the vendor: %s", out)
	}
	var session map[string]any
	if err := json.Unmarshal(top["session"], &session); err != nil {
		t.Fatal(err)
	}
	if session["model"] != "gpt-live-1" || session["instructions"] != "be brief" {
		t.Fatalf("session = %v", session)
	}
	if !strings.Contains(string(top["transport"]), `"sdp":"v=0 offer"`) {
		t.Fatalf("offer not forwarded untouched: %s", top["transport"])
	}
}

func TestRewriteLiveModelOnEmptyAndMalformedBodies(t *testing.T) {
	out, err := rewriteLiveModel(nil, "gpt-live-1")
	if err != nil || string(out) != `{"session":{"model":"gpt-live-1"}}` {
		t.Fatalf("empty body: %s, %v", out, err)
	}
	if _, err := rewriteLiveModel(json.RawMessage(`[1,2]`), "m"); err == nil {
		t.Fatal("a non-object body was accepted")
	}
	if _, err := rewriteLiveModel(json.RawMessage(`{"session":"not an object"}`), "m"); err == nil {
		t.Fatal("a non-object session was accepted")
	}
}

func TestCreateLiveSessionReturnsTheVendorsAnswer(t *testing.T) {
	var auth, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, path = r.Header.Get("Authorization"), r.URL.Path
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"session":{"id":"live_9"},"transport":{"type":"webrtc","sdp":"v=0 answer"}}`))
	}))
	defer srv.Close()

	resp, err := NewOpenAIAdapter("openai", srv.URL, "k", nil).CreateLiveSession(context.Background(),
		LiveSessionRequest{Model: "gpt-live-1", Body: json.RawMessage(`{"transport":{"sdp":"o"}}`)})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != "live_9" || !strings.Contains(string(resp.Body), "v=0 answer") {
		t.Fatalf("answer not passed through: %+v", resp)
	}
	if auth != "Bearer k" || (path != "/realtime/calls" && path != "/realtime/sessions") {
		t.Fatalf("auth %q path %q", auth, path)
	}
}

func TestCreateLiveSessionOpenAIGaWithLocationHeader(t *testing.T) {
	var auth, path, ctype string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, path = r.Header.Get("Authorization"), r.URL.Path
		ctype = r.Header.Get("Content-Type")
		w.Header().Set("Location", "/v1/realtime/calls/call_ga_123")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("v=0\r\no=- 999 2 IN IP4 127.0.0.1\r\ns=-\r\n"))
	}))
	defer srv.Close()

	resp, err := NewOpenAIAdapter("openai", srv.URL, "k", nil).CreateLiveSession(context.Background(),
		LiveSessionRequest{Model: "gpt-4o-realtime-preview", Body: json.RawMessage(`{"transport":{"type":"webrtc","sdp":"v=0\r\no=offer"}}`)})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != "call_ga_123" {
		t.Fatalf("expected ID call_ga_123, got %q", resp.ID)
	}
	if auth != "Bearer k" || path != "/realtime/calls" {
		t.Fatalf("auth %q path %q", auth, path)
	}
	if !strings.HasPrefix(ctype, "multipart/form-data;") {
		t.Fatalf("expected multipart content-type, got %q", ctype)
	}
	var parsed struct {
		ID      string `json:"id"`
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
		Transport struct {
			SDP string `json:"sdp"`
		} `json:"transport"`
	}
	if err := json.Unmarshal(resp.Body, &parsed); err != nil {
		t.Fatalf("invalid json body: %v, raw: %s", err, resp.Body)
	}
	if parsed.Session.ID != "call_ga_123" || !strings.Contains(parsed.Transport.SDP, "IN IP4 127.0.0.1") {
		t.Fatalf("unexpected normalized body: %+v", parsed)
	}
}

func TestCreateLiveSessionIsAttemptedExactlyOnce(t *testing.T) {
	// Every other endpoint retries a 5xx. This one must not: the vendor may
	// already have opened the session whose answer was lost, and a replayed
	// offer opens a second one nothing ever closes.
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"message":"upstream hiccup"}}`))
	}))
	defer srv.Close()

	_, err := NewOpenAIAdapter("openai", srv.URL, "k", nil).CreateLiveSession(context.Background(),
		LiveSessionRequest{Model: "gpt-live-1", Body: json.RawMessage(`{}`)})
	var refusal *LiveUpstreamError
	if !errors.As(err, &refusal) || refusal.Status != http.StatusBadGateway {
		t.Fatalf("err = %v", err)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("session creation attempted %d times", n)
	}
}

func TestCreateLiveSessionSurfacesTheVendorsMessageWithoutSecrets(t *testing.T) {
	cases := []struct {
		name, body, want, mustNot string
		status                    int
	}{
		{"structured", `{"error":{"message":"Invalid SDP offer: missing fingerprint","type":"invalid_request_error"}}`, "Invalid SDP offer: missing fingerprint", "", http.StatusBadRequest},
		{"echoed key", `{"error":{"message":"Incorrect API key provided: sk-proj-abcdefghijklmnop."}}`, "[redacted]", "abcdefghijklmnop", http.StatusUnauthorized},
		{"html page", "<html>\n<body>bad gateway</body>\n</html>", "<html> <body>bad gateway</body> </html>", "\n", http.StatusBadGateway},
		{"empty", ``, "(empty response)", "", http.StatusServiceUnavailable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.body))
			}))
			defer srv.Close()

			_, err := NewOpenAIAdapter("openai", srv.URL, "k", nil).CreateLiveSession(context.Background(),
				LiveSessionRequest{Model: "gpt-live-1", Body: json.RawMessage(`{}`)})
			var refusal *LiveUpstreamError
			if !errors.As(err, &refusal) {
				t.Fatalf("err = %v", err)
			}
			if refusal.Status != c.status || !strings.Contains(refusal.Message, c.want) {
				t.Fatalf("refusal = %+v", refusal)
			}
			if c.mustNot != "" && strings.Contains(refusal.Message, c.mustNot) {
				t.Fatalf("message leaked %q: %q", c.mustNot, refusal.Message)
			}
		})
	}
}

func TestCreateLiveSessionReportsTheGaRefusalNotTheRetiredEndpoints(t *testing.T) {
	// OpenAI answers 404 for a model it does not serve, and the retired
	// /realtime/sessions answers 404 "Invalid URL" for everything. Falling
	// through to the second and reporting its message hid the real reason
	// for a fortnight: the alias was pointed at a model that no longer exists.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		if r.URL.Path == "/realtime/calls" {
			_, _ = w.Write([]byte(`{"error":{"message":"The model 'gpt-4o-realtime-preview' does not exist","code":"model_not_found"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid URL (POST /v1/realtime/sessions)"}}`))
	}))
	defer srv.Close()

	_, err := NewOpenAIAdapter("openai", srv.URL, "k", nil).CreateLiveSession(context.Background(),
		LiveSessionRequest{Model: "gpt-4o-realtime-preview", Body: json.RawMessage(`{"transport":{"type":"webrtc","sdp":"v=0 offer"}}`)})
	var refusal *LiveUpstreamError
	if !errors.As(err, &refusal) || refusal.Status != http.StatusNotFound {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(refusal.Message, "does not exist") || strings.Contains(refusal.Message, "Invalid URL") {
		t.Fatalf("the retired endpoint's message was reported instead of the vendor's: %q", refusal.Message)
	}
}
