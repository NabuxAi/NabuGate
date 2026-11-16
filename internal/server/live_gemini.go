package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nabugate/internal/provider"
)

// LiveRelay is the server-side media path for a realtime call. Zero is the
// deployment that only signals: POST /v1/live/gemini/token still mints a
// token, and the browser is told to open Google's own websocket.
type LiveRelay struct {
	Enabled     bool
	MaxSession  time.Duration
	MaxSessions int

	OpenAIKey  string
	OpenAIBase string

	GeminiKey  string
	GeminiBase string
	// GeminiWSOrigin replaces the public Gemini websocket host. Tests point
	// it at a mock. Empty uses generativelanguage.googleapis.com.
	GeminiWSOrigin string
}

func (s *Server) SetLiveRelay(r LiveRelay) {
	if r.Enabled && r.MaxSession <= 0 {
		r.MaxSession = 30 * time.Minute
	}
	s.relay = r
}

func (s *Server) relaySessionLimit() int {
	if s.relay.MaxSessions > 0 {
		return s.relay.MaxSessions
	}
	return liveMaxOpen
}

func newLiveID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "live_" + hex.EncodeToString(b[:])
}

type geminiTokenResponse struct {
	Name      string `json:"name"`
	Token     string `json:"token"`
	Expire    string `json:"expireTime"`
	ExpiresAt string `json:"expires_at"`
	URL       string `json:"url"`
	SessionID string `json:"session_id"`
}

// handleGeminiLiveToken — POST /v1/live/gemini/token.
//
// The body is `{ "setup": <BidiGenerateContent setup> }`. setup is forwarded
// as the token's bidiGenerateContentSetup, so functionDeclarations are not
// remodelled. The reply is the ephemeral token, when it expires, and the
// websocket the client should open. With the relay enabled that websocket is
// this gateway; otherwise it is Google's.
func (s *Server) handleGeminiLiveToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Model string          `json:"model"`
		Setup json.RawMessage `json:"setup"`
		Bidi  json.RawMessage `json:"bidiGenerateContentSetup"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	setup := body.Setup
	if len(strings.TrimSpace(string(setup))) == 0 {
		setup = body.Bidi
	}
	if len(strings.TrimSpace(string(setup))) == 0 {
		writeError(w, http.StatusBadRequest, "field 'setup' is required")
		return
	}
	alias := body.Model
	if alias == "" {
		alias = "nabu-live"
	}
	if !s.aliasAllowed(w, r, alias) {
		return
	}
	s.openGeminiLive(w, r, setup, true)
}

// handleLiveRelaySession — POST /v1/live/ws/session.
//
// Opens a relay ticket for OpenAI Realtime's websocket or, with a setup
// object, for Gemini Live. The relay has to be enabled; this route does not
// exist as a way around that.
func (s *Server) handleLiveRelaySession(w http.ResponseWriter, r *http.Request) {
	if !s.relay.Enabled {
		writeError(w, http.StatusNotFound, "live relay is disabled")
		return
	}
	var body struct {
		Provider string          `json:"provider"`
		Model    string          `json:"model"`
		Setup    json.RawMessage `json:"setup"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Provider == "gemini" || (body.Provider == "" && len(body.Setup) > 0) {
		if len(strings.TrimSpace(string(body.Setup))) == 0 {
			writeError(w, http.StatusBadRequest, "field 'setup' is required")
			return
		}
		alias := body.Model
		if alias == "" {
			alias = "nabu-live"
		}
		if !s.aliasAllowed(w, r, alias) {
			return
		}
		s.openGeminiLive(w, r, body.Setup, true)
		return
	}
	if body.Provider != "" && body.Provider != "openai" {
		writeError(w, http.StatusBadRequest, "field 'provider' must be openai or gemini")
		return
	}
	alias := body.Model
	if alias == "" {
		alias = "nabu-live"
	}
	if !s.aliasAllowed(w, r, alias) {
		return
	}
	model := alias
	if prov, upstream, ok := s.router.LivePrimary(alias); ok {
		if prov != "openai" {
			writeError(w, http.StatusBadRequest, "live relay dials OpenAI only for an openai live alias")
			return
		}
		model = upstream
	}
	if strings.TrimSpace(s.relay.OpenAIKey) == "" || strings.TrimSpace(s.relay.OpenAIBase) == "" {
		writeError(w, http.StatusServiceUnavailable, "live relay has no OpenAI key configured")
		return
	}
	s.beginLive(w, r, "openai", model, openAIRealtimeURL(s.relay.OpenAIBase, model), map[string]string{
		"Authorization": "Bearer " + s.relay.OpenAIKey,
	})
}

// openGeminiLive mints the ephemeral token and either returns Google's
// websocket or a relay ticket, depending on the flag.
func (s *Server) openGeminiLive(w http.ResponseWriter, r *http.Request, setup json.RawMessage, allowDirect bool) {
	project := s.project(r)
	s.reapLive(r, project)
	if s.live.open(project) >= s.relaySessionLimit() {
		writeError(w, http.StatusTooManyRequests, "too many live sessions open for this key")
		return
	}
	if strings.TrimSpace(s.relay.GeminiKey) == "" {
		writeError(w, http.StatusServiceUnavailable, "gemini live: GEMINI_API_KEY is not configured")
		return
	}
	now := time.Now()
	ttl := s.relay.MaxSession
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	tok, err := provider.MintGeminiLiveToken(r.Context(), s.relay.GeminiBase, s.relay.GeminiKey, setup, now.Add(ttl), now.Add(2*time.Minute))
	if err != nil {
		writeError(w, liveErrStatus(err), err.Error())
		return
	}
	model := setupModel(setup)
	sessionID := newLiveID()
	sess := &liveSession{
		project:       project,
		provider:      "gemini",
		model:         model,
		alias:         "nabu-live",
		created:       now,
		byCaller:      servedByCaller(r.Context()),
		billedSeconds: livePrepaidSeconds,
		lastSeen:      now,
		rate:          gatewayRateFrom(r.Context()),
	}
	if err := s.live.put(sessionID, sess); err != nil {
		s.log.Warn("persist live sessions", "error", err)
	}
	s.billLive(r, project, *sess, livePrepaidSeconds, true)

	socket := provider.GeminiLiveSocketURL(tok.Name)
	if s.relay.Enabled {
		origin := s.relay.GeminiWSOrigin
		if origin == "" {
			origin = "https://generativelanguage.googleapis.com"
		}
		socket = s.issueRelay(project, sessionID, provider.GeminiLiveSocketURLOn(origin, tok.Name), nil)
		if socket == "" {
			writeError(w, http.StatusInternalServerError, "could not issue a live relay ticket")
			return
		}
		socket = requestWSBase(r) + socket
	} else if !allowDirect {
		writeError(w, http.StatusNotFound, "live relay is disabled")
		return
	}
	exp := tok.Expire.UTC().Format(time.RFC3339)
	writeJSON(w, http.StatusOK, geminiTokenResponse{
		Name:      tok.Name,
		Token:     tok.Name,
		Expire:    exp,
		ExpiresAt: exp,
		URL:       socket,
		SessionID: sessionID,
	})
}

func setupModel(setup json.RawMessage) string {
	var body struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(setup, &body)
	model := strings.TrimPrefix(strings.TrimSpace(body.Model), "models/")
	if model == "" {
		return "gemini-2.5-flash-native-audio-preview-12-2025"
	}
	return model
}

func openAIRealtimeURL(base, model string) string {
	base = strings.TrimRight(base, "/")
	switch {
	case strings.HasPrefix(base, "https://"):
		base = "wss://" + strings.TrimPrefix(base, "https://")
	case strings.HasPrefix(base, "http://"):
		base = "ws://" + strings.TrimPrefix(base, "http://")
	}
	return base + "/realtime?model=" + url.QueryEscape(model)
}

// beginLive registers a billed session and returns a relay websocket URL.
func (s *Server) beginLive(w http.ResponseWriter, r *http.Request, prov, model, upstream string, headers map[string]string) {
	project := s.project(r)
	s.reapLive(r, project)
	if s.live.open(project) >= s.relaySessionLimit() {
		writeError(w, http.StatusTooManyRequests, "too many live sessions open for this key")
		return
	}
	now := time.Now()
	sessionID := newLiveID()
	sess := &liveSession{
		project:       project,
		provider:      prov,
		model:         model,
		alias:         "nabu-live",
		created:       now,
		byCaller:      servedByCaller(r.Context()),
		billedSeconds: livePrepaidSeconds,
		lastSeen:      now,
		rate:          gatewayRateFrom(r.Context()),
	}
	if err := s.live.put(sessionID, sess); err != nil {
		s.log.Warn("persist live sessions", "error", err)
	}
	s.billLive(r, project, *sess, livePrepaidSeconds, true)
	path := s.issueRelay(project, sessionID, upstream, headers)
	if path == "" {
		writeError(w, http.StatusInternalServerError, "could not issue a live relay ticket")
		return
	}
	writeJSON(w, http.StatusOK, geminiTokenResponse{
		Token:     sessionID,
		ExpiresAt: now.Add(time.Minute).UTC().Format(time.RFC3339),
		URL:       requestWSBase(r) + path,
		SessionID: sessionID,
	})
}

func requestWSBase(r *http.Request) string {
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	scheme := "wss"
	if proto == "http" {
		scheme = "ws"
	}
	return scheme + "://" + host
}
