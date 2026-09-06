package server

import (
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// --- Cross-origin requests ---
//
// The gateway sent no CORS headers at all, which made it unreachable from a
// browser: a page calling it got no response it was allowed to read, and a
// custom header — X-Nabu-Key-* among them — could not even be sent, because a
// custom header triggers a preflight and there was nothing to answer it.
//
// Two properties this has to hold, and they pull in opposite directions:
//
//   - It must open nothing by default. An empty allow-list behaves exactly as
//     before, so an existing deployment gains no exposure by upgrading.
//   - The allow-list is checked against the actual Origin, and the response
//     echoes that one origin rather than "*", because "*" and credentials are
//     mutually exclusive and, worse, would let any page on the internet spend
//     a key that a browser holds.
//
// The per-key AllowedOrigins check in the auth path is unaffected and still
// runs: this decides whether the browser is allowed to *make* the call, that
// decides whether the key is allowed to be used *from there*. A deployment can
// allow an origin here and still refuse it for a given key.

// allowedRequestHeaders is the list a preflight is answered with.
// Access-Control-Allow-Headers cannot wildcard a prefix, so every
// X-Nabu-Key-<provider> a caller might send has to be named. Deriving it from
// the configured providers rather than hard-coding it means adding a provider
// never silently leaves its key header un-sendable from a browser.
func (s *Server) allowedRequestHeaders() string {
	names := []string{"Authorization", "Content-Type", callerKeyPrefix + "Mode"}
	for name := range s.providers {
		names = append(names, callerKeyPrefix+name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// corsExposed are the response headers a browser may read. Without this the
// balance headers exist on the wire and are invisible to the page that needs
// them.
const corsExposed = "X-Nabu-Balance-USD, X-Nabu-Balance-Warning, Retry-After"

// cors wraps a handler with cross-origin support for the configured origins.
// With no origins configured it is a no-op, and the handler behaves exactly as
// it did before this existed.
func (s *Server) cors(next http.Handler) http.Handler {
	if len(s.corsOrigins) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" && s.originPermitted(origin) {
			// Echo the one origin, never "*": a wildcard cannot carry
			// credentials, and would invite any page to spend a key the
			// browser holds.
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Expose-Headers", corsExposed)
			// The response differs by Origin, so a shared cache must not serve
			// one origin's response to another.
			w.Header().Add("Vary", "Origin")
		}

		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			// A preflight carries no Authorization header, so it cannot be
			// authenticated and must be answered before auth runs. It reveals
			// nothing beyond what this gateway accepts from this origin.
			if origin == "" || !s.originPermitted(origin) {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", s.allowedRequestHeaders())
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// originPermitted matches an Origin header against the configured list. The
// patterns are the same shape as a key's allowed_origins — a bare host, or
// "*.example.com" for a subdomain tree — so an operator learns one syntax.
func (s *Server) originPermitted(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return false
	}
	for _, pattern := range s.corsOrigins {
		if matchOrigin(strings.ToLower(strings.TrimSpace(pattern)), host) {
			return true
		}
	}
	return false
}

// SetCORSOrigins configures which browser origins may call the gateway. Empty
// disables cross-origin support entirely, which is the default.
func (s *Server) SetCORSOrigins(origins []string) { s.corsOrigins = cleanList(origins) }
