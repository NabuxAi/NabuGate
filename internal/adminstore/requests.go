package adminstore

import (
	"strings"
	"sync"
	"time"
)

// RequestLog is the recent-calls view the console shows.
//
// Deliberately in memory and deliberately bounded. The persisted counters
// answer "how much has this project spent", which is the question that has to
// survive a restart; this answers "what did my key just do", which is the
// question somebody asks with the failing request still on their screen. Making
// it durable would mean a disk write on every call through the gateway and a
// state file that grows without limit, to keep answering a question nobody asks
// about last month.
//
// So: a fixed ring, oldest entry overwritten, empty after a restart. The
// console says as much rather than presenting an empty log as "no traffic".
type RequestLog struct {
	mu      sync.RWMutex
	entries []RequestEntry
	next    int
	filled  bool
}

// RequestEntry is one call through the gateway.
type RequestEntry struct {
	At       time.Time `json:"at"`
	Project  string    `json:"project"`
	Provider string    `json:"provider"`
	Model    string    `json:"model"`
	Tokens   int64     `json:"tokens"`
	CostUSD  float64   `json:"cost_usd"`

	// Denied marks a request refused before a provider was reached — by the
	// key's allow-list, its origin list or its rate limit. These are the rows
	// worth having a log for: an aggregate counter says a key was refused
	// eleven times without saying which alias it was reaching for.
	Denied bool   `json:"denied"`
	Reason string `json:"reason,omitempty"`
}

// NewRequestLog returns a log holding at most size entries.
func NewRequestLog(size int) *RequestLog {
	if size <= 0 {
		size = 500
	}
	return &RequestLog{entries: make([]RequestEntry, size)}
}

// Add records one call, overwriting the oldest entry once the ring is full.
func (l *RequestLog) Add(e RequestEntry) {
	if l == nil {
		return
	}
	if e.At.IsZero() {
		e.At = time.Now()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries[l.next] = e
	l.next = (l.next + 1) % len(l.entries)
	if l.next == 0 {
		l.filled = true
	}
}

// Recent returns up to limit entries, newest first. An empty owner returns
// nothing rather than everything: an unauthenticated caller looks exactly like
// an empty string, and the failure that hands them the whole deployment's
// traffic should not be a missing argument.
//
// projects is the set of project names the caller owns; nil means every
// project, which only an administrator is ever given.
func (l *RequestLog) Recent(projects map[string]bool, limit int) []RequestEntry {
	if l == nil {
		return nil
	}
	if limit <= 0 || limit > len(l.entries) {
		limit = len(l.entries)
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	out := make([]RequestEntry, 0, limit)
	// Walk backwards from the most recent write, so the ring is read newest
	// first without sorting.
	count := len(l.entries)
	if !l.filled {
		count = l.next
	}
	for i := 0; i < count && len(out) < limit; i++ {
		idx := (l.next - 1 - i + len(l.entries)) % len(l.entries)
		e := l.entries[idx]
		if e.At.IsZero() {
			continue
		}
		if projects != nil && !projects[strings.ToLower(e.Project)] {
			continue
		}
		out = append(out, e)
	}
	return out
}
