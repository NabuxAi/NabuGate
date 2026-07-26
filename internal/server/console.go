package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nabugate/internal/adminstore"
)

const consoleCookie = "nabugate_console"

// mountConsoleAPI adds the console's own endpoints under /admin/api/.
//
// These are separate from /v1/*: the gateway API is authenticated by a project
// key, while the console is authenticated by a human logging in. Sharing one
// credential for both is what made the console world-readable before — the SPA
// shell was served to anyone, and the only thing standing between a visitor and
// the gateway was them not knowing the admin key.
func (s *Server) mountConsoleAPI(mux *http.ServeMux) {
	if s.admin == nil {
		return
	}

	mux.HandleFunc("GET /admin/api/status", s.consoleStatus)
	mux.HandleFunc("POST /admin/api/setup", s.consoleSetup)
	mux.HandleFunc("POST /admin/api/login", s.consoleLogin)
	mux.HandleFunc("POST /admin/api/logout", s.consoleLogout)

	mux.Handle("GET /admin/api/tokens", s.consoleAuth(s.listTokens))
	mux.Handle("POST /admin/api/tokens", s.consoleAuth(s.createToken))
	mux.Handle("DELETE /admin/api/tokens/{name}", s.consoleAuth(s.deleteToken))
	mux.Handle("PATCH /admin/api/tokens/{name}", s.consoleAuth(s.patchToken))

	mux.Handle("GET /admin/api/usage", s.consoleAuth(s.consoleUsage))
	mux.Handle("POST /admin/api/usage/reset", s.consoleAuth(s.resetUsage))
}

// consoleAuth gates a console endpoint on a live login session.
func (s *Server) consoleAuth(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(consoleCookie)
		if err != nil || !s.admin.ValidSession(c.Value) {
			writeError(w, http.StatusUnauthorized, "sign in to the console first")
			return
		}
		next(w, r)
	})
}

// consoleStatus tells the SPA whether to show a login form or first-run setup,
// without revealing anything to an unauthenticated visitor beyond that.
func (s *Server) consoleStatus(w http.ResponseWriter, r *http.Request) {
	authed := false
	if c, err := r.Cookie(consoleCookie); err == nil {
		authed = s.admin.ValidSession(c.Value)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"needs_setup":   s.admin.NeedsSetup(),
		"authenticated": authed,
	})
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// consoleSetup creates the first admin. Allowed without authentication exactly
// once — there is nobody to authorise it — and refused forever after, so it
// cannot be used to add a second account from outside.
func (s *Server) consoleSetup(w http.ResponseWriter, r *http.Request) {
	if !s.admin.NeedsSetup() {
		writeError(w, http.StatusForbidden, "an admin already exists; sign in instead")
		return
	}
	var c credentials
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&c); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.admin.CreateAdmin(c.Username, c.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.startConsoleSession(w, r, c.Username, c.Password)
}

func (s *Server) consoleLogin(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&c); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	s.startConsoleSession(w, r, c.Username, c.Password)
}

func (s *Server) startConsoleSession(w http.ResponseWriter, r *http.Request, user, pass string) {
	token, expiry, err := s.admin.Authenticate(user, pass)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:  consoleCookie,
		Value: token,
		Path:  "/admin",
		// HttpOnly so no script on the page can read it, and SameSite=Strict
		// because the console has no cross-site use at all.
		HttpOnly: true,
		Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
		SameSite: http.SameSiteStrictMode,
		Expires:  expiry,
		MaxAge:   int(time.Until(expiry).Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true})
}

func (s *Server) consoleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(consoleCookie); err == nil {
		_ = s.admin.EndSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: consoleCookie, Value: "", Path: "/admin",
		HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1,
	})
	w.WriteHeader(http.StatusNoContent)
}

// ─────────────────────────── tokens ─────────────────────────────────────────

func (s *Server) listTokens(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"tokens": s.admin.Tokens()})
}

type tokenRequest struct {
	Name           string   `json:"name"`
	Allow          []string `json:"allow"`
	RateLimit      int      `json:"rate_limit"`
	AllowedOrigins []string `json:"allowed_origins"`
	Disabled       *bool    `json:"disabled"`
}

func (s *Server) createToken(w http.ResponseWriter, r *http.Request) {
	var req tokenRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	t, secret, err := s.admin.NewToken(req.Name, cleanList(req.Allow), req.RateLimit, cleanList(req.AllowedOrigins))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, adminstore.ErrNameTaken) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}

	// The secret appears in this response and nowhere else, ever: only its hash
	// is stored, so it cannot be shown again and must be copied now.
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":  t,
		"secret": secret,
		"note":   "Copy this now — it is stored only as a hash and cannot be shown again.",
	})
}

func (s *Server) deleteToken(w http.ResponseWriter, r *http.Request) {
	if err := s.admin.DeleteToken(r.PathValue("name")); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) patchToken(w http.ResponseWriter, r *http.Request) {
	var req tokenRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	name := r.PathValue("name")

	if req.Disabled != nil {
		if err := s.admin.SetTokenDisabled(name, *req.Disabled); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
	}
	if req.AllowedOrigins != nil {
		if err := s.admin.SetOrigins(name, cleanList(req.AllowedOrigins)); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ─────────────────────────── usage ──────────────────────────────────────────

// consoleUsage reports the persisted per-project counters.
//
// These are the real numbers, and they survive a restart — the in-memory
// tracker behind /v1/usage resets to zero on every redeploy, which made the
// console look like nothing had ever run.
func (s *Server) consoleUsage(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"by_project": s.admin.Usage()})
}

func (s *Server) resetUsage(w http.ResponseWriter, r *http.Request) {
	if err := s.admin.ResetUsage(r.URL.Query().Get("project")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─────────────────────────── origin filtering ───────────────────────────────

// originAllowed reports whether a request may use a token, given the token's
// origin allow-list.
//
// Empty list means anywhere. Otherwise the request's Origin is matched, falling
// back to Referer: a browser sets both itself and a page cannot forge them for
// another site, which is what makes this worth checking for a key that ships
// inside a web app. It is not a defence against a non-browser client, which can
// send any header it likes — that is what the key itself is for.
func originAllowed(allowed []string, r *http.Request) bool {
	if len(allowed) == 0 {
		return true
	}

	host := requestOriginHost(r)
	if host == "" {
		// No Origin and no Referer: not a browser. A token that named specific
		// origins was meant for one, so refuse rather than assume.
		return false
	}

	for _, pattern := range allowed {
		if matchOrigin(strings.ToLower(strings.TrimSpace(pattern)), host) {
			return true
		}
	}
	return false
}

func requestOriginHost(r *http.Request) string {
	for _, raw := range []string{r.Header.Get("Origin"), r.Header.Get("Referer")} {
		if raw == "" || raw == "null" {
			continue
		}
		if u, err := url.Parse(raw); err == nil && u.Host != "" {
			return strings.ToLower(u.Hostname())
		}
	}
	return ""
}

// matchOrigin supports an exact host or a leading "*." wildcard, which covers
// "every subdomain of ours" without permitting a lookalike like
// "evil-example.com" that a bare substring match would let through.
func matchOrigin(pattern, host string) bool {
	if pattern == "*" {
		return true
	}
	if suffix, ok := strings.CutPrefix(pattern, "*."); ok {
		return host == suffix || strings.HasSuffix(host, "."+suffix)
	}
	return host == pattern
}

func cleanList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}
