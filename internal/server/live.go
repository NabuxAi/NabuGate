package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"nabugate/internal/adminstore"
	"nabugate/internal/provider"
	"nabugate/internal/router"
	"nabugate/internal/usage"
)

// Realtime voice (GPT-Live) is the one capability whose traffic does not pass
// through the gateway: after the SDP handshake the browser and the vendor
// talk directly over WebRTC, and only the vendor knows how long the call ran.
// So the session is billed in two steps — created here, then charged as the
// caller reports the duration. The caller is the key holder's own server,
// which is as trusted as any request on that key; a browser is never handed
// the key.

// liveSessionTTL bounds how long an unreported session stays in the registry.
// A vendor session cannot outlive this; one that was never closed properly is
// simply dropped, unbilled beyond what was reported.
const liveSessionTTL = 6 * time.Hour

// The minutes of a live call are reported by the caller, because only the
// vendor sees the audio. These bound what a caller who reports wrongly — or
// not at all — can do with that trust:
const (
	// livePrepaidSeconds is billed when the session is created, so a call
	// that never reports is not a free one.
	livePrepaidSeconds = 60
	// liveStaleAfter is how long an open session may go without a report
	// before it is treated as abandoned and billed, once, until it went quiet
	// plus this window — the next time its key touches live. The session is
	// kept, so a call that was only quiet and reports later is billed the rest
	// and nothing twice: reports are cumulative snapshots.
	liveStaleAfter = 10 * time.Minute
	// liveClockSlack is how far a report may run ahead of the wall clock
	// since creation. Beyond it the report is a bug, and billing a customer
	// for a call longer than the time that has passed is not on.
	liveClockSlack = 5 * time.Minute
	// liveMaxOpen is how many sessions one key may hold open at once.
	liveMaxOpen = 10
)

type liveSession struct {
	project  string
	provider string
	model    string
	alias    string
	created  time.Time
	// byCaller is a session opened on the caller's own vendor key. The vendor
	// bills them for every minute of it, so the gateway must not — on the
	// usage reports as much as on the request that created it.
	byCaller bool
	// billedSeconds is the cumulative duration already charged. Usage reports
	// are snapshots, not increments, so a repeated or out-of-order report can
	// never double-bill.
	billedSeconds int64
	// lastSeen is the last time the session was created or reported on.
	lastSeen time.Time
	// reaped marks a session already billed as abandoned, so it is billed
	// that way once and no longer counts as open.
	reaped bool
}

// liveSessionRecord is a session as it is kept on disk.
type liveSessionRecord struct {
	ID            string    `json:"id"`
	Project       string    `json:"project"`
	Provider      string    `json:"provider"`
	Model         string    `json:"model"`
	Alias         string    `json:"alias"`
	Created       time.Time `json:"created"`
	ByCaller      bool      `json:"by_caller,omitempty"`
	BilledSeconds int64     `json:"billed_seconds"`
	LastSeen      time.Time `json:"last_seen,omitempty"`
	Reaped        bool      `json:"reaped,omitempty"`
}

type liveSessions struct {
	mu       sync.Mutex
	sessions map[string]*liveSession
	// path is where the registry is kept between restarts. Empty keeps it in
	// memory alone — tests, and a deployment with no state volume.
	path string
}

func newLiveSessions() *liveSessions {
	return &liveSessions{sessions: map[string]*liveSession{}}
}

func (l *liveSessions) put(id string, s *liveSession) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune()
	l.sessions[id] = s
	return l.saveLocked()
}

// take returns the session for a project's usage report and how many new
// seconds it accounts for, advancing the billed mark. final removes it. The
// session comes back as a copy, read under the lock.
func (l *liveSessions) take(id, project string, seconds int64, final bool) (liveSession, int64, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, ok := l.sessions[id]
	if !ok || s.project != project {
		return liveSession{}, 0, false, nil
	}
	if ceiling := int64(time.Since(s.created).Seconds() + liveClockSlack.Seconds()); seconds > ceiling {
		seconds = ceiling
	}
	delta := seconds - s.billedSeconds
	if delta < 0 {
		delta = 0
	}
	s.billedSeconds += delta
	s.lastSeen = time.Now()
	snapshot := *s
	if final {
		delete(l.sessions, id)
	}
	var err error
	if delta > 0 || final {
		err = l.saveLocked()
	}
	return snapshot, delta, true, err
}

// reaped is an abandoned session and the seconds still owed on it.
type reaped struct {
	id      string
	session liveSession
	owed    int64
}

// reap bills a project's sessions that have gone quiet past liveStaleAfter
// and returns what each owes: from creation until it went quiet plus the
// stale window, less what was already billed. Each session is reaped once.
func (l *liveSessions) reap(project string, now time.Time) ([]reaped, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []reaped
	for id, s := range l.sessions {
		if s.project != project || s.reaped || now.Sub(s.lastSeen) <= liveStaleAfter {
			continue
		}
		until := int64(s.lastSeen.Add(liveStaleAfter).Sub(s.created).Seconds())
		owed := until - s.billedSeconds
		if owed < 0 {
			owed = 0
		}
		s.billedSeconds += owed
		s.reaped = true
		out = append(out, reaped{id: id, session: *s, owed: owed})
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, l.saveLocked()
}

// open counts a project's sessions that are still live.
func (l *liveSessions) open(project string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, s := range l.sessions {
		if s.project == project && !s.reaped {
			n++
		}
	}
	return n
}

func (l *liveSessions) prune() {
	cutoff := time.Now().Add(-liveSessionTTL)
	for id, s := range l.sessions {
		if s.created.Before(cutoff) {
			delete(l.sessions, id)
		}
	}
}

// persistTo keeps the registry at path from now on: what is already there is
// loaded (minus anything past its TTL), and every change is written back —
// to a temporary file, then renamed over the old one, so a crash mid-write
// leaves the previous registry rather than half of a new one.
func (l *liveSessions) persistTo(path string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return err
	default:
		var records []liveSessionRecord
		if err := json.Unmarshal(raw, &records); err != nil {
			return fmt.Errorf("live sessions %s: %w", path, err)
		}
		for _, rec := range records {
			if rec.ID == "" {
				continue
			}
			l.sessions[rec.ID] = &liveSession{
				project:       rec.Project,
				provider:      rec.Provider,
				model:         rec.Model,
				alias:         rec.Alias,
				created:       rec.Created,
				byCaller:      rec.ByCaller,
				billedSeconds: rec.BilledSeconds,
				lastSeen:      rec.LastSeen,
				reaped:        rec.Reaped,
			}
			if l.sessions[rec.ID].lastSeen.IsZero() {
				l.sessions[rec.ID].lastSeen = rec.Created
			}
		}
	}
	l.path = path
	l.prune()
	return l.saveLocked()
}

func (l *liveSessions) saveLocked() error {
	if l.path == "" {
		return nil
	}
	records := make([]liveSessionRecord, 0, len(l.sessions))
	for id, s := range l.sessions {
		records = append(records, liveSessionRecord{
			ID:            id,
			Project:       s.project,
			Provider:      s.provider,
			Model:         s.model,
			Alias:         s.alias,
			Created:       s.created,
			ByCaller:      s.byCaller,
			BilledSeconds: s.billedSeconds,
			LastSeen:      s.lastSeen,
			Reaped:        s.reaped,
		})
	}
	raw, err := json.Marshal(records)
	if err != nil {
		return err
	}
	tmp := l.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, l.path)
}

// SetLiveStateFile keeps the live-session registry at path between restarts.
// A session is signalled here and billed afterwards, from usage the product
// reports while the call runs; held in memory alone, a redeploy mid-call
// turned every later report into "unknown live session", and those minutes
// were never billed.
func (s *Server) SetLiveStateFile(path string) error {
	return s.live.persistTo(path)
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

// liveErrStatus answers a failed session request in the class of what went
// wrong. A refused alias is 503. A vendor refusal keeps its class where the
// caller can act on it — a rejected body or offer is 400, the vendor's rate
// limit 429 — and is 502 where only the gateway's operator can: a rejected
// key, a vendor 5xx.
func liveErrStatus(err error) int {
	var refused *router.LiveMisconfiguredError
	if errors.As(err, &refused) {
		return http.StatusServiceUnavailable
	}
	var vendor *provider.LiveUpstreamError
	if errors.As(err, &vendor) {
		switch vendor.Status {
		case http.StatusTooManyRequests:
			return http.StatusTooManyRequests
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			return http.StatusBadRequest
		}
		return http.StatusBadGateway
	}
	return aliasErrStatus(err, "unknown live alias")
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
	project := s.project(r)
	s.reapLive(r, project)
	if s.live.open(project) >= liveMaxOpen {
		writeError(w, http.StatusTooManyRequests, fmt.Sprintf(
			"this key already has %d live sessions open; finalise one (POST /v1/live/sessions/{id}/usage with \"final\": true) before opening another", liveMaxOpen))
		return
	}

	result, err := s.router.LiveSession(r.Context(), alias, body)
	if err != nil {
		writeError(w, liveErrStatus(err), err.Error())
		return
	}

	now := time.Now()
	session := &liveSession{
		project:       project,
		provider:      result.Provider,
		model:         result.Model,
		alias:         alias,
		created:       now,
		byCaller:      servedByCaller(r.Context()),
		billedSeconds: livePrepaidSeconds,
		lastSeen:      now,
	}
	if err := s.live.put(result.ID, session); err != nil {
		// The call is already open at the vendor; failing the caller now would
		// strand it. Said out loud so an unwritable volume gets noticed.
		s.log.Warn("persist live sessions", "error", err)
	}
	// The creation is the call's one request on the books, and pays its
	// first minute; the rest follows through usage reports.
	s.billLive(r, project, *session, livePrepaidSeconds, true)

	w.Header().Set("X-Nabu-Provider", result.Provider)
	w.Header().Set("X-Nabu-Model", result.Model)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(result.Body)
}

type liveUsageBody struct {
	// Seconds is the session's cumulative duration so far — from the vendor's
	// usage events, or from the product's own clock.
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
	s.reapLive(r, project)
	session, delta, ok, saveErr := s.live.take(id, project, body.Seconds, body.Final)
	if saveErr != nil {
		s.log.Warn("persist live sessions", "error", saveErr)
	}
	if !ok {
		// Unknown to this key: either never created here, already finalised,
		// or someone else's. All three read the same to the caller.
		writeError(w, http.StatusNotFound, "unknown live session")
		return
	}

	// The call was counted as one request when it was signalled; a report of
	// its clock only adds minutes and money. Counting each one made a
	// two-minute call nine requests in the console.
	cost := s.billLive(r, project, session, delta, false)

	writeJSON(w, http.StatusOK, liveUsageResponse{
		SessionID:     id,
		Seconds:       body.Seconds,
		BilledSeconds: session.billedSeconds,
		CostUSD:       cost,
		Final:         body.Final,
	})
}

// billLive charges seconds of a live call to the key's owner at the model's
// per-minute price (times the plan rate, like every other metered call).
// request counts the call itself, once, when it is signalled.
func (s *Server) billLive(r *http.Request, project string, session liveSession, seconds int64, request bool) float64 {
	if seconds <= 0 && !request {
		return 0
	}
	list, priced := s.usage.PriceOf(session.provider, session.model, usage.Metered{Seconds: float64(seconds)})
	cost, providerCost := s.charge(r.Context(), session.byCaller, list)
	e := adminstore.UsageEntry{
		CostUSD:         cost,
		ProviderCostUSD: providerCost,
		Unpriced:        !priced && !session.byCaller,
		AudioSeconds:    float64(seconds),
	}
	if request {
		e.Requests = 1
	}
	s.meter(r, project, session.provider, session.model, e)
	return cost
}

// reapLive bills and forgets this key's abandoned sessions. It runs on the
// key's own live requests, so the bill lands with the key that ran the call
// and no background job has to impersonate one.
func (s *Server) reapLive(r *http.Request, project string) {
	gone, err := s.live.reap(project, time.Now())
	if err != nil {
		s.log.Warn("persist live sessions", "error", err)
	}
	for _, g := range gone {
		cost := s.billLive(r, project, g.session, g.owed, false)
		s.log.Warn("live session went quiet without a final report; billed until it went quiet plus the grace window",
			"session", g.id, "project", project, "seconds", g.owed, "cost_usd", cost)
	}
}
