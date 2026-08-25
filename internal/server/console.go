package server

import (
	"context"
	"encoding/json"
	"fmt"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nabugate/internal/adminstore"
	"nabugate/internal/provider"
)

const consoleCookie = "nabugate_console"

// mountConsoleAPI adds the console's own endpoints under /api/.
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

	mux.HandleFunc("GET /api/status", s.consoleStatus)
	mux.HandleFunc("POST /api/setup", s.consoleSetup)
	mux.HandleFunc("POST /api/login", s.consoleLogin)
	mux.HandleFunc("POST /api/signup", s.consoleSignup)

	mux.HandleFunc("POST /api/logout", s.consoleLogout)

	// Single sign-on with a Nabu account, restricted to an explicit admin
	// allow-list. Browser redirects rather than JSON: the browser is handed to
	// NabuAuth and comes back with a code.
	mux.HandleFunc("GET /api/nabu/status", s.consoleNabuStatus)
	mux.HandleFunc("GET /api/nabu", s.consoleNabuStart)
	mux.HandleFunc("GET /api/nabu/callback", s.consoleNabuCallback)
	mux.HandleFunc("GET /admin/api/nabu/callback", s.consoleNabuCallback)

		mux.Handle("GET /api/tokens", s.consoleAuth(s.listTokens))
	mux.Handle("GET /api/me", s.consoleAuth(s.getMe))
	mux.Handle("POST /api/me/recharge", s.consoleAuth(s.rechargeMe))
	mux.Handle("POST /api/tokens", s.consoleAuth(s.createToken))
	mux.Handle("DELETE /api/tokens/{name}", s.consoleAuth(s.deleteToken))
	mux.Handle("PATCH /api/tokens/{name}", s.consoleAuth(s.patchToken))

	mux.Handle("GET /api/overview", s.consoleAuth(s.consoleOverview))
	mux.Handle("GET /api/usage", s.consoleAuth(s.consoleUsage))
	mux.Handle("POST /api/usage/reset", s.consoleAuth(s.resetUsage))

	mux.Handle("GET /api/admins", s.consoleAuth(requireAdmin(s.listAdmins)))
	mux.Handle("GET /api/users", s.consoleAuth(requireAdmin(s.listUsers)))
	mux.Handle("POST /api/users/recharge", s.consoleAuth(requireAdmin(s.adminRechargeUser)))
	mux.Handle("POST /api/admins", s.consoleAuth(requireAdmin(s.createAdmin)))

	mux.Handle("GET /api/agents", s.consoleAuth(requireAdmin(s.listAgents)))
	mux.Handle("POST /api/agents", s.consoleAuth(requireAdmin(s.saveAgent)))
	mux.Handle("PATCH /api/agents/{name}", s.consoleAuth(requireAdmin(s.saveAgent)))
	mux.Handle("DELETE /api/agents/{name}", s.consoleAuth(requireAdmin(s.deleteAgent)))
	mux.Handle("POST /api/agents/{name}/test", s.consoleAuth(requireAdmin(s.testAgent)))
	mux.Handle("GET /api/flows", s.consoleAuth(requireAdmin(s.listFlows)))
	mux.Handle("POST /api/flows", s.consoleAuth(requireAdmin(s.saveFlow)))
	mux.Handle("PATCH /api/flows/{name}", s.consoleAuth(requireAdmin(s.saveFlow)))
	mux.Handle("DELETE /api/flows/{name}", s.consoleAuth(requireAdmin(s.deleteFlow)))
	mux.Handle("POST /api/flows/{name}/test", s.consoleAuth(requireAdmin(s.testFlow)))
}

// consoleAuth gates a console endpoint on a live login session.
func (s *Server) consoleAuth(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(consoleCookie)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "sign in to the console first")
			return
		}
		info, valid := s.admin.ValidSession(c.Value)
		if !valid {
			writeError(w, http.StatusUnauthorized, "sign in to the console first")
			return
		}
		ctx := context.WithValue(r.Context(), consoleEmailCtxKey{}, info.Email)
		ctx = context.WithValue(ctx, consoleAdminCtxKey{}, info.IsAdmin)
		next(w, r.WithContext(ctx))
	})
}

// consoleStatus tells the SPA whether to show a login form or first-run setup,
// without revealing anything to an unauthenticated visitor beyond that.
func (s *Server) consoleStatus(w http.ResponseWriter, r *http.Request) {
	authed := false
	var info adminstore.SessionInfo
	if c, err := r.Cookie(consoleCookie); err == nil {
		info, authed = s.admin.ValidSession(c.Value)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"needs_setup":   s.admin.NeedsSetup(),
		"authenticated": authed,
		"is_admin":      info.IsAdmin,
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
	// Guessing this password is what gets an attacker the whole gateway, so the
	// endpoint is rate-limited per username and source address. See throttle.go.
	key := loginKey(user, r)
	if s.logins.blocked(key) {
		writeError(w, http.StatusTooManyRequests, "too many failed sign-in attempts; try again later")
		return
	}

	token, expiry, err := s.admin.Authenticate(user, pass)
	if err != nil {
		// Fallback to user login
		token, expiry, err = s.admin.AuthenticateUser(user, pass)
		if err != nil {
			s.logins.fail(key)
			s.log.Warn("console sign-in failed", "username", user, "ip", clientIP(r))
			writeError(w, http.StatusUnauthorized, "invalid username or password")
			return
		}
	}
	s.logins.reset(key)

	http.SetCookie(w, &http.Cookie{
		Name:  consoleCookie,
		Value: token,
		Path:  "/",
		// HttpOnly so no script on the page can read it, and SameSite=Strict
		// because the console has no cross-site use at all.
		HttpOnly: true,
		Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
		SameSite: http.SameSiteLaxMode,
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
		Name: consoleCookie, Value: "", Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
	w.WriteHeader(http.StatusNoContent)
}

// ─────────────────────────── admin accounts ─────────────────────────────────

// listAdmins returns the console account usernames (never hashes).
func (s *Server) listAdmins(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"admins": s.admin.Usernames()})
}

// createAdmin adds another console account. Unlike consoleSetup (first-run and
// unauthenticated), this requires an existing signed-in admin (consoleAuth).
func (s *Server) createAdmin(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&c); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.admin.CreateAdmin(c.Username, c.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"username": strings.ToLower(strings.TrimSpace(c.Username)),
	})
}

// ─────────────────────────── sub-agents ─────────────────────────────────────

// listAgents surfaces every sub-agent — baked (config/YAML, read-only) and
// console-managed (editable) — with the fields the editor needs.
func (s *Server) listAgents(w http.ResponseWriter, _ *http.Request) {
	managed := map[string]bool{}
	if s.admin != nil {
		for _, rec := range s.admin.Agents() {
			managed[rec.Name] = true
		}
	}
	type agentInfo struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Model       string   `json:"model"`
		System      string   `json:"system"`
		Temperature *float64 `json:"temperature,omitempty"`
		MaxTokens   *int     `json:"max_tokens,omitempty"`
		Editable    bool     `json:"editable"`
	}
	out := make([]agentInfo, 0)
	if s.agents != nil {
		for _, name := range s.agents.Names() {
			if a, ok := s.agents.Lookup(name); ok {
				out = append(out, agentInfo{
					Name: a.Name, Description: a.Description, Model: a.Model,
					System: a.System, Temperature: a.Temperature, MaxTokens: a.MaxTokens,
					Editable: managed[a.Name],
				})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": out})
}

type agentRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Model       string   `json:"model"`
	System      string   `json:"system"`
	Temperature *float64 `json:"temperature"`
	MaxTokens   *int     `json:"max_tokens"`
}

// saveAgent creates (POST) or updates (PATCH) a console-managed sub-agent and
// registers it live, so it is callable immediately. Baked agents are read-only;
// creating one under a baked name shadows it with the managed definition.
func (s *Server) saveAgent(w http.ResponseWriter, r *http.Request) {
	var req agentRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<18)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if name := r.PathValue("name"); name != "" {
		req.Name = name // PATCH takes the name from the path
	}
	rec := adminstore.AgentRecord{
		Name: req.Name, Description: req.Description, Model: req.Model,
		System: req.System, Temperature: req.Temperature, MaxTokens: req.MaxTokens,
	}
	if err := s.admin.SaveAgent(rec); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.loadManagedAgents()
	writeJSON(w, http.StatusOK, map[string]any{"agent": rec})
}

// deleteAgent removes a console-managed sub-agent. Baked agents cannot be
// deleted here — edit their YAML in the repo.
func (s *Server) deleteAgent(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	managed := false
	for _, rec := range s.admin.Agents() {
		if rec.Name == name {
			managed = true
			break
		}
	}
	if !managed {
		writeError(w, http.StatusBadRequest, "this agent is defined in config; edit its YAML in the repo")
		return
	}
	if err := s.admin.DeleteAgent(name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.agents != nil {
		s.agents.Remove(name)
	}
	w.WriteHeader(http.StatusNoContent)
}

// testAgent runs one message through the agent and returns its reply, so an
// admin can preview an agent from the console without a gateway token.
func (s *Server) testAgent(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ag, ok := s.agents.Lookup(name)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown agent")
		return
	}
	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<18)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(body.Message) == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}
	req := provider.ChatRequest{Messages: []provider.Message{{Role: "user", Content: body.Message}}}
	applyAgentToChat(ag, &req)
	result, err := s.router.Chat(r.Context(), ag.Model, req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"content":  result.Response.Content,
		"provider": result.Provider,
		"model":    result.Model,
	})
}

// ─────────────────────────── tokens ─────────────────────────────────────────

func (s *Server) listTokens(w http.ResponseWriter, r *http.Request) {
	isAdmin, _ := r.Context().Value(consoleAdminCtxKey{}).(bool)
	email, _ := r.Context().Value(consoleEmailCtxKey{}).(string)
	
	allTokens := s.admin.Tokens()
	if isAdmin {
		writeJSON(w, http.StatusOK, map[string]any{"tokens": allTokens})
		return
	}
	
	var userTokens []adminstore.Token
	for _, t := range allTokens {
		if strings.EqualFold(t.Owner, email) {
			userTokens = append(userTokens, t)
		}
	}
	if userTokens == nil {
		userTokens = []adminstore.Token{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": userTokens})
}

type tokenRequest struct {
	Name           string   `json:"name"`
	Allow          []string `json:"allow"`
	RateLimit      int      `json:"rate_limit"`
	AllowedOrigins []string `json:"allowed_origins"`
	Providers      []string `json:"providers"`
	Disabled       *bool    `json:"disabled"`
}

func (s *Server) createToken(w http.ResponseWriter, r *http.Request) {
	var req tokenRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	owner := ""
	if sessionEmail, ok := r.Context().Value(consoleEmailCtxKey{}).(string); ok {
		owner = sessionEmail
	}

	t, secret, err := s.admin.NewToken(req.Name, cleanList(req.Allow), req.RateLimit, cleanList(req.AllowedOrigins), owner, cleanList(req.Providers))
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
	name := r.PathValue("name")
	if !s.canEditToken(r, name) {
		writeError(w, http.StatusForbidden, "not authorized to edit this token")
		return
	}
	if err := s.admin.DeleteToken(name); err != nil {
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
	if !s.canEditToken(r, name) {
		writeError(w, http.StatusForbidden, "not authorized to edit this token")
		return
	}

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
	if req.Providers != nil {
		if err := s.admin.SetProviders(name, cleanList(req.Providers)); err != nil {
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
	byProj, byModel, byProv := s.admin.Usage()
	writeJSON(w, http.StatusOK, map[string]any{
		"by_project": byProj,
		"by_model": byModel,
		"by_provider": byProv,
	})
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

// consoleOverview reports what the gateway is actually running: which providers
// came up, which aliases and agents exist, and the config-declared keys.
//
// The console used to render all of this from a mock file, so it described a
// gateway that did not exist — providers that were never keyed, aliases that had
// been renamed. Everything here is read from the live router.

func (s *Server) getMe(w http.ResponseWriter, r *http.Request) {
	email, _ := r.Context().Value(consoleEmailCtxKey{}).(string)
	user := s.admin.GetUser(email)
	if user == nil {
		user = &adminstore.User{Email: email, Balance: 0}
	}
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) rechargeMe(w http.ResponseWriter, r *http.Request) {
	email, _ := r.Context().Value(consoleEmailCtxKey{}).(string)
	var req struct {
		Amount float64 `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid amount")
		return
	}
	// generate a fake transaction ID for simulation
	txID := fmt.Sprintf("tr_%d", time.Now().UnixNano())
	newBalance := s.admin.AddPayment(email, req.Amount, "success", txID)
	writeJSON(w, http.StatusOK, map[string]any{"balance": newBalance})
}

func (s *Server) consoleOverview(w http.ResponseWriter, r *http.Request) {
	type providerInfo struct {
		Name        string `json:"name"`
		Passthrough bool   `json:"passthrough"`
	}

	providers := make([]providerInfo, 0)
	for _, name := range s.router.ProviderNames() {
		providers = append(providers, providerInfo{Name: name, Passthrough: s.router.IsPassthrough(name)})
	}

	isAdmin, _ := r.Context().Value(consoleAdminCtxKey{}).(bool)
	email, _ := r.Context().Value(consoleEmailCtxKey{}).(string)
	
	allowedProviders := make(map[string]bool)
	if !isAdmin {
		for _, t := range s.admin.Tokens() {
			if strings.EqualFold(t.Owner, email) {
				for _, p := range t.Providers {
					allowedProviders[p] = true
				}
			}
		}
	}

	aliases := make([]map[string]any, 0)
	for _, a := range s.router.AliasInfos() {
		if !isAdmin && len(allowedProviders) > 0 && a.Owner != "agent" && a.Owner != "flow" {
			if !allowedProviders[a.Owner] {
				continue
			}
		}
		aliases = append(aliases, map[string]any{"id": a.ID, "owner": a.Owner})
	}

	// Config-declared keys are listed by project name only. Their secrets live
	// in the deployment's environment and the gateway never sees them in a form
	// worth showing — and a console that displayed keys would be a console worth
	// stealing.
	byProj, _, _ := s.admin.Usage()
	writeJSON(w, http.StatusOK, map[string]any{
		"providers":   providers,
		"aliases":     aliases,
		"agents":      s.agents.Names(),
		"config_keys": s.policy.Projects(),
		"usage":       byProj,
	})
}

type consoleEmailCtxKey struct{}

type consoleAdminCtxKey struct{}

func (s *Server) canEditToken(r *http.Request, name string) bool {
	isAdmin, _ := r.Context().Value(consoleAdminCtxKey{}).(bool)
	if isAdmin {
		return true
	}
	email, _ := r.Context().Value(consoleEmailCtxKey{}).(string)
	if email == "" {
		return false
	}
	for _, t := range s.admin.Tokens() {
		if strings.EqualFold(t.Name, name) {
			return strings.EqualFold(t.Owner, email)
		}
	}
	return false
}

func requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		isAdmin, _ := r.Context().Value(consoleAdminCtxKey{}).(bool)
		if !isAdmin {
			writeError(w, http.StatusForbidden, "only admins can do this")
			return
		}
		next(w, r)
	}
}
func (s *Server) consoleSignup(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&c); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.admin.SignupUser(c.Username, c.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.startConsoleSession(w, r, c.Username, c.Password)
}

func (s *Server) listUsers(w http.ResponseWriter, _ *http.Request) {
	if s.admin == nil {
		writeJSON(w, http.StatusOK, map[string]any{"users": []any{}})
		return
	}
	users := s.admin.ListUsers()
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (s *Server) adminRechargeUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email  string  `json:"email"`
		Amount float64 `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if s.admin == nil {
		writeError(w, http.StatusInternalServerError, "no admin store")
		return
	}
	s.admin.AddPayment(body.Email, body.Amount, "admin-recharge", "manual-"+fmt.Sprint(time.Now().UnixNano()))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
