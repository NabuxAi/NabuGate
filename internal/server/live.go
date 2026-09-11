package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"nabugate/internal/adminstore"
	"nabugate/internal/provider"
)

// Realtime voice (GPT-Live) is the one capability whose traffic does not pass
// through the gateway: after the SDP handshake the browser and the vendor
// talk directly over WebRTC, and only the vendor knows how long the call ran.
// So the session is billed in two steps — created here, then charged as the
// caller reports the duration the vendor's usage events told it. The caller
// is the key holder's own server, which is as trusted as any request on that
// key; a browser is never handed the key.

// liveSessionTTL bounds how long an unreported session stays in the registry.
// A vendor session cannot outlive this; one that was never closed properly is
// simply dropped, unbilled beyond what was reported.
const liveSessionTTL = 6 * time.Hour

type liveSession struct {
	project  string
	provider string
	model    string
	alias    string
	created  time.Time
	// billedSeconds is the cumulative duration already charged. Usage reports
	// are snapshots, not increments, so a repeated or out-of-order report can
	// never double-bill.
	billedSeconds int64
}

type liveSessions struct {
	mu       sync.Mutex
	sessions map[string]*liveSession
}

func newLiveSessions() *liveSessions {
	return &liveSessions{sessions: map[string]*liveSession{}}
}

func (l *liveSessions) put(id string, s *liveSession) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune()
	l.sessions[id] = s
}

// take returns the session for a project's usage report and how many new
// seconds it accounts for, advancing the billed mark. final removes it.
func (l *liveSessions) take(id, project string, seconds int64, final bool) (*liveSession, int64, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, ok := l.sessions[id]
	if !ok || s.project != project {
		return nil, 0, false
	}
	delta := seconds - s.billedSeconds
	if delta < 0 {
		delta = 0
	}
	s.billedSeconds += delta
	if final {
		delete(l.sessions, id)
	}
	return s, delta, true
}

func (l *liveSessions) prune() {
	cutoff := time.Now().Add(-liveSessionTTL)
	for id, s := range l.sessions {
		if s.created.Before(cutoff) {
			delete(l.sessions, id)
		}
	}
}

// liveRequestAlias reads the alias from the caller's body: top-level "model"
// mirrors every other endpoint; "session.model" is where the vendor's own
// shape carries it, accepted so an SDK-shaped body works unchanged.
func liveRequestAlias(body []byte) string {
	var top struct {
		Model   string `json:"model"`
		Session struct {
			Model string `json:"model"`
		} `json:"session"`
	}
	_ = json.Unmarshal(body, &top)
	if top.Model != "" {
		return top.Model
	}
	return top.Session.Model
}

// handleLiveSession — POST /v1/live/sessions. The body is the vendor's session
// creation body (session + transport with the browser's SDP offer) with the
// model given as a gateway alias. The vendor's answer is returned verbatim
// under the gateway's provider headers.
func (s *Server) handleLiveSession(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read body")
		return
	}
	if !json.Valid(body) {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	alias := liveRequestAlias(body)
	if alias == "" {
		writeError(w, http.StatusBadRequest, "field 'model' (live alias) is required")
		return
	}
	if !s.aliasAllowed(w, r, alias) {
		return
	}

	result, err := s.router.LiveSession(r.Context(), alias, body)
	if err != nil {
		writeError(w, aliasErrStatus(err, "unknown live alias"), err.Error())
		return
	}

	project := s.project(r)
	s.live.put(result.ID, &liveSession{
		project:  project,
		provider: result.Provider,
		model:    result.Model,
		alias:    alias,
		created:  time.Now(),
	})
	// The creation itself is a request on the books with no cost yet; the
	// minutes follow through usage reports.
	s.record(r, result.Provider, result.Model, provider.Usage{})

	w.Header().Set("X-Nabu-Provider", result.Provider)
	w.Header().Set("X-Nabu-Model", result.Model)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(result.Body)
}

type liveUsageBody struct {
	// Seconds is the session's cumulative duration so far, as the vendor's
	// session.usage.updated / session.closed events report it.
	Seconds int64 `json:"seconds"`
	// Final marks the session closed: it is billed and forgotten.
	Final bool `json:"final"`
}

type liveUsageResponse struct {
	SessionID     string  `json:"session_id"`
	Seconds       int64   `json:"seconds"`
	BilledSeconds int64   `json:"billed_seconds"`
	CostUSD       float64 `json:"cost_usd"`
	Final         bool    `json:"final"`
}

// handleLiveUsage — POST /v1/live/sessions/{id}/usage. Charges the seconds
// not yet billed on this session to the key's owner at the model's per-minute
// price (times the plan rate, like every other metered call).
func (s *Server) handleLiveUsage(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var body liveUsageBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Seconds < 0 {
		writeError(w, http.StatusBadRequest, "field 'seconds' must not be negative")
		return
	}
	project := s.project(r)
	session, delta, ok := s.live.take(id, project, body.Seconds, body.Final)
	if !ok {
		// Unknown to this key: either never created here, already finalised,
		// or someone else's. All three read the same to the caller.
		writeError(w, http.StatusNotFound, "unknown live session")
		return
	}

	cost := s.usage.SecondsCost(session.provider, session.model, delta)
	if servedByCaller(r.Context()) {
		cost = 0
	} else {
		cost = gatewayRateFrom(r.Context()).apply(cost)
	}
	if delta > 0 {
		s.usage.RecordAt(project, session.provider, session.model, provider.Usage{}, cost)
		if s.admin != nil {
			s.admin.RecordUsage(project, session.provider, session.model, 0, 0, cost)
		}
		s.requests.Add(adminstore.RequestEntry{
			Project:  project,
			Provider: session.provider,
			Model:    session.model,
			CostUSD:  cost,
		})
		s.log.Info("billed", "project", project, "provider", session.provider, "model", session.model,
			"live_seconds", delta, "cost_usd", cost)
	}

	writeJSON(w, http.StatusOK, liveUsageResponse{
		SessionID:     id,
		Seconds:       body.Seconds,
		BilledSeconds: session.billedSeconds,
		CostUSD:       cost,
		Final:         body.Final,
	})
}
