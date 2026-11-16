package server

import (
	"net/http"
	"sync"
	"time"

	"nabugate/internal/wsutil"
)

// A relay ticket is the right to open one websocket. It is single use and
// lives about a minute, which is long enough for the client that just asked
// for it to connect. The upstream URL is chosen here, never by the client.
type relayTicket struct {
	project   string
	sessionID string
	upstream  string
	header    http.Header
	expires   time.Time
	created   time.Time
}

type relayTickets struct {
	mu sync.Mutex
	m  map[string]*relayTicket
}

func newRelayTickets() *relayTickets {
	return &relayTickets{m: map[string]*relayTicket{}}
}

func (t *relayTickets) put(id string, ticket *relayTicket) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for k, v := range t.m {
		if time.Now().After(v.expires) {
			delete(t.m, k)
		}
	}
	t.m[id] = ticket
}

func (t *relayTickets) take(id string) (relayTicket, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	v, ok := t.m[id]
	if !ok {
		return relayTicket{}, false
	}
	delete(t.m, id)
	if time.Now().After(v.expires) {
		return relayTicket{}, false
	}
	return *v, true
}

// issueRelay stores a ticket and returns the path the client opens.
// Empty means the ticket store is missing, which is a programming error.
func (s *Server) issueRelay(project, sessionID, upstream string, headers map[string]string) string {
	if s.tickets == nil || upstream == "" {
		return ""
	}
	id := newLiveID()
	h := make(http.Header, len(headers))
	for k, v := range headers {
		h.Set(k, v)
	}
	s.tickets.put(id, &relayTicket{
		project:   project,
		sessionID: sessionID,
		upstream:  upstream,
		header:    h,
		expires:   time.Now().Add(time.Minute),
		created:   time.Now(),
	})
	return "/v1/live/ws?ticket=" + id
}

// handleLiveRelay — GET /v1/live/ws?ticket=…
//
// The browser (or the product's server) speaks this socket. The gateway
// dials the vendor and copies frames both ways. Frames are not parsed: a
// Gemini toolCall or toolResponse, and an OpenAI Realtime event, go through
// with the same bytes.
func (s *Server) handleLiveRelay(w http.ResponseWriter, r *http.Request) {
	if !s.relay.Enabled {
		writeError(w, http.StatusNotFound, "live relay is disabled")
		return
	}
	ticket, ok := s.tickets.take(r.URL.Query().Get("ticket"))
	if !ok {
		writeError(w, http.StatusUnauthorized, "unknown or expired live relay ticket")
		return
	}
	upstream, err := wsutil.Dial(r.Context(), ticket.upstream, ticket.header)
	if err != nil {
		s.log.Warn("live relay dial", "error", err.Error())
		writeError(w, http.StatusBadGateway, "live relay could not reach the vendor")
		return
	}
	client, err := wsutil.Accept(w, r)
	if err != nil {
		upstream.Close()
		s.log.Warn("live relay upgrade", "error", err.Error())
		return
	}
	s.runRelay(r, ticket, client, upstream)
}

func (s *Server) runRelay(r *http.Request, ticket relayTicket, client, upstream *wsutil.Conn) {
	limit := s.relay.MaxSession
	if limit <= 0 {
		limit = 30 * time.Minute
	}
	remain := limit - time.Since(ticket.created)
	if remain < 0 {
		remain = 0
	}
	timer := time.NewTimer(remain)
	defer timer.Stop()

	done := make(chan struct{})
	var once sync.Once
	stop := func() {
		once.Do(func() {
			_ = client.Write(wsutil.OpClose, []byte{0x03, 0xe8})
			client.Close()
			upstream.Close()
			close(done)
		})
	}
	go func() {
		copyFrames(upstream, client)
		stop()
	}()
	go func() {
		copyFrames(client, upstream)
		stop()
	}()
	select {
	case <-done:
	case <-timer.C:
		stop()
		<-done
	}

	seconds := int64(time.Since(ticket.created).Seconds())
	if capS := int64(limit.Seconds()); seconds > capS {
		seconds = capS
	}
	session, delta, ok, err := s.live.take(ticket.sessionID, ticket.project, seconds, true)
	if err != nil {
		s.log.Warn("persist live sessions", "error", err)
	}
	if ok {
		s.billLive(r, ticket.project, session, delta, false)
	}
}

func copyFrames(dst, src *wsutil.Conn) {
	for {
		op, payload, err := src.Read()
		if err != nil {
			return
		}
		if op == wsutil.OpClose {
			_ = dst.Write(wsutil.OpClose, payload)
			return
		}
		if err := dst.Write(op, payload); err != nil {
			return
		}
	}
}
