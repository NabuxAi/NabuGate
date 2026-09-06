package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nabugate/internal/config"
)

func corsServer(origins ...string) (*Server, http.Handler) {
	s := &Server{providers: map[string]config.ProviderMeta{
		"gemini": {Name: "gemini"}, "openai": {Name: "openai"},
	}}
	s.SetCORSOrigins(origins)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("reached"))
	})
	return s, s.cors(inner)
}

// The default must be exactly what the gateway did before this existed:
// nothing. An upgrade cannot open a deployment that named no origins.
func TestCORSDisabledByDefault(t *testing.T) {
	_, h := corsServer()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	r.Header.Set("Origin", "https://anywhere.example")
	h.ServeHTTP(w, r)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("allow-origin = %q with no origins configured", got)
	}
	if w.Body.String() != "reached" {
		t.Error("the request did not reach the handler")
	}
}

func TestCORSEchoesTheAllowedOrigin(t *testing.T) {
	_, h := corsServer("app.nabuxai.com", "*.mrchatgpt.org")

	for _, tc := range []struct {
		origin string
		want   bool
	}{
		{"https://app.nabuxai.com", true},
		{"https://mrchatgpt.org", true},
		{"https://panel.mrchatgpt.org", true},
		{"https://evil.example", false},
		// A host that merely ends in the same letters is not in the tree.
		{"https://notmrchatgpt.org", false},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		r.Header.Set("Origin", tc.origin)
		h.ServeHTTP(w, r)

		got := w.Header().Get("Access-Control-Allow-Origin")
		if tc.want && got != tc.origin {
			t.Errorf("%s: allow-origin = %q, want the origin echoed back", tc.origin, got)
		}
		if !tc.want && got != "" {
			t.Errorf("%s: allowed, and should not be", tc.origin)
		}
		// A wildcard cannot carry credentials, and would let any page on the
		// internet spend a key the browser holds.
		if got == "*" {
			t.Errorf("%s: answered with a wildcard", tc.origin)
		}
		// The response differs by Origin, so a shared cache must not reuse it.
		if tc.want && !strings.Contains(w.Header().Get("Vary"), "Origin") {
			t.Errorf("%s: no Vary: Origin", tc.origin)
		}
		// A refused origin still gets its request served — CORS decides what
		// the browser may read, not whether the server answers.
		if w.Body.String() != "reached" {
			t.Errorf("%s: request did not reach the handler", tc.origin)
		}
	}
}

// A preflight carries no Authorization header, so it must be answered before
// auth runs — and it must name every X-Nabu-Key-<provider> header, because
// Allow-Headers cannot wildcard a prefix.
func TestCORSPreflight(t *testing.T) {
	_, h := corsServer("app.nabuxai.com")

	w := httptest.NewRecorder()
	r := httptest.NewRequest("OPTIONS", "/v1/audio/transcriptions", nil)
	r.Header.Set("Origin", "https://app.nabuxai.com")
	r.Header.Set("Access-Control-Request-Method", "POST")
	r.Header.Set("Access-Control-Request-Headers", "authorization, x-nabu-key-gemini")
	h.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	if w.Body.String() == "reached" {
		t.Error("the preflight reached the handler instead of being answered")
	}
	allow := w.Header().Get("Access-Control-Allow-Headers")
	for _, want := range []string{"Authorization", "X-Nabu-Key-Mode", "X-Nabu-Key-gemini", "X-Nabu-Key-openai"} {
		if !strings.Contains(allow, want) {
			t.Errorf("allow-headers missing %q: %s", want, allow)
		}
	}
	if !strings.Contains(w.Header().Get("Access-Control-Allow-Methods"), "POST") {
		t.Errorf("allow-methods = %q", w.Header().Get("Access-Control-Allow-Methods"))
	}
}

func TestCORSPreflightFromAnUnknownOriginIsRefused(t *testing.T) {
	_, h := corsServer("app.nabuxai.com")
	w := httptest.NewRecorder()
	r := httptest.NewRequest("OPTIONS", "/v1/chat/completions", nil)
	r.Header.Set("Origin", "https://evil.example")
	r.Header.Set("Access-Control-Request-Method", "POST")
	h.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("an unknown origin was told it is allowed")
	}
}

// The balance headers exist on the wire and are useless to a page that cannot
// read them.
func TestCORSExposesTheBalanceHeaders(t *testing.T) {
	_, h := corsServer("app.nabuxai.com")
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	r.Header.Set("Origin", "https://app.nabuxai.com")
	h.ServeHTTP(w, r)

	if !strings.Contains(w.Header().Get("Access-Control-Expose-Headers"), "X-Nabu-Balance-USD") {
		t.Errorf("expose-headers = %q", w.Header().Get("Access-Control-Expose-Headers"))
	}
}
