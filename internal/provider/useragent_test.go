package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestOutboundUserAgentIsProduct guards the WAF fix: outbound provider requests
// must NOT go out with Go's default "Go-http-client/1.1" User-Agent, which some
// upstreams (e.g. Parspack AI Studio) treat as a bot and answer with a 403 block
// page. Both shared clients inject the product User-Agent instead.
func TestOutboundUserAgentIsProduct(t *testing.T) {
	var gotShared, gotStream string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stream" {
			gotStream = r.Header.Get("User-Agent")
		} else {
			gotShared = r.Header.Get("User-Agent")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	// Non-streaming path (sharedHTTPClient via postJSON).
	if _, _, err := postJSON(context.Background(), srv.URL, nil, []byte(`{}`), "test"); err != nil {
		t.Fatalf("postJSON: %v", err)
	}
	if gotShared != userAgent {
		t.Errorf("shared client User-Agent = %q, want %q", gotShared, userAgent)
	}

	// Streaming path (streamHTTPClient via doStreamRequest).
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := doStreamRequest(context.Background(), req, "test")
	if err != nil {
		t.Fatalf("doStreamRequest: %v", err)
	}
	resp.Body.Close()
	if gotStream != userAgent {
		t.Errorf("stream client User-Agent = %q, want %q", gotStream, userAgent)
	}
}

// TestUserAgentTransportPreservesExplicit verifies an adapter-supplied
// User-Agent is not overwritten by the injector.
func TestUserAgentTransportPreservesExplicit(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	headers := map[string]string{"User-Agent": "custom-agent/9"}
	if _, _, err := postJSON(context.Background(), srv.URL, headers, []byte(`{}`), "test"); err != nil {
		t.Fatalf("postJSON: %v", err)
	}
	if got != "custom-agent/9" {
		t.Errorf("User-Agent = %q, want the explicit %q (must not be overwritten)", got, "custom-agent/9")
	}
}
