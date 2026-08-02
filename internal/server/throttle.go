package server

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Brute-force protection for the console sign-in.
//
// Everything the gateway guards sits behind this one form: minting project
// tokens, editing agents, adding admins, resetting usage. Unlike a project key,
// which is high-entropy and generated for the operator, the console password is
// chosen by a human and only has to clear a ten-character minimum — so guessing
// is a real attack rather than a theoretical one, and until now the endpoint
// accepted attempts at whatever rate an attacker could send them.
//
// Deliberately the same shape as NabuAuth's throttle, which solves the same
// problem for the same operators. Divergence between the two would only make
// both harder to reason about.
//
// State is in memory: a restart forgets the counters. That is an accepted
// trade-off — the gateway is a single process and persisting failed logins
// would mean writing to the admin store on every guess, which is its own denial
// of service.

const (
	maxLoginAttempts = 8
	loginLockFor     = 10 * time.Minute
)

type loginAttempt struct {
	count int
	until time.Time
}

type throttle struct {
	mu       sync.Mutex
	attempts map[string]*loginAttempt
}

func newThrottle() *throttle { return &throttle{attempts: map[string]*loginAttempt{}} }

// blocked reports whether the key is currently locked out, and clears the entry
// once its window has passed.
func (t *throttle) blocked(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	a := t.attempts[key]
	if a == nil {
		return false
	}
	if time.Now().After(a.until) {
		delete(t.attempts, key)
		return false
	}
	return a.count >= maxLoginAttempts
}

// fail records a failed attempt and extends the window.
func (t *throttle) fail(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	a := t.attempts[key]
	if a == nil || time.Now().After(a.until) {
		a = &loginAttempt{}
		t.attempts[key] = a
	}
	a.count++
	a.until = time.Now().Add(loginLockFor)
}

// reset clears the counter after a successful sign-in.
func (t *throttle) reset(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.attempts, key)
}

// loginKey identifies an attempt by username and source address together.
//
// Keying on the username alone would hand anyone a way to lock a real admin out
// by guessing at their name; keying on the address alone lets one host walk
// through every account. Requiring both to repeat is what makes the lock bite
// the attacker rather than the victim.
func loginKey(username string, r *http.Request) string {
	return strings.ToLower(strings.TrimSpace(username)) + "|" + clientIP(r)
}

// clientIP is the caller's address, preferring the first hop in X-Forwarded-For
// because the gateway runs behind a reverse proxy.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
