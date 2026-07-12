package provider

import (
	"net/http"
	"testing"
	"time"
)

// TestStreamClientHasNoWholeRequestTimeout guards against reintroducing an
// http.Client.Timeout on the streaming client: because that timeout also covers
// reading the response body, it would sever a long-running SSE stream
// mid-generation. Streaming must rely on the request context instead, with only
// a response-header timeout to bound a dead upstream.
func TestStreamClientHasNoWholeRequestTimeout(t *testing.T) {
	if streamHTTPClient.Timeout != 0 {
		t.Errorf("streamHTTPClient.Timeout = %v, want 0 (would cut long streams)", streamHTTPClient.Timeout)
	}
	uat, ok := streamHTTPClient.Transport.(*userAgentTransport)
	if !ok {
		t.Fatalf("streamHTTPClient.Transport is %T, want *userAgentTransport", streamHTTPClient.Transport)
	}
	tr, ok := uat.base.(*http.Transport)
	if !ok {
		t.Fatalf("streamHTTPClient transport base is %T, want *http.Transport", uat.base)
	}
	if tr.ResponseHeaderTimeout <= 0 || tr.ResponseHeaderTimeout > 5*time.Minute {
		t.Errorf("ResponseHeaderTimeout = %v, want a sane positive cap", tr.ResponseHeaderTimeout)
	}
}
