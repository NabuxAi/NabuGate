package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func loginRequest(remoteAddr, xff string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/admin/api/login", nil)
	r.RemoteAddr = remoteAddr
	if xff != "" {
		r.Header.Set("X-Forwarded-For", xff)
	}
	return r
}

func TestThrottleLocksOutAfterTheAttemptLimit(t *testing.T) {
	th := newThrottle()
	key := loginKey("admin", loginRequest("10.0.0.1:5555", ""))

	for i := 0; i < maxLoginAttempts-1; i++ {
		th.fail(key)
		if th.blocked(key) {
			t.Fatalf("locked out after %d attempts, limit is %d", i+1, maxLoginAttempts)
		}
	}

	th.fail(key)
	if !th.blocked(key) {
		t.Fatalf("still accepting attempts after %d failures", maxLoginAttempts)
	}
}

func TestThrottleForgetsAfterTheWindow(t *testing.T) {
	th := newThrottle()
	key := loginKey("admin", loginRequest("10.0.0.1:5555", ""))

	for i := 0; i < maxLoginAttempts; i++ {
		th.fail(key)
	}
	if !th.blocked(key) {
		t.Fatal("expected a lockout")
	}

	// Expire the window rather than sleeping for it.
	th.mu.Lock()
	th.attempts[key].until = time.Now().Add(-time.Second)
	th.mu.Unlock()

	if th.blocked(key) {
		t.Error("the lockout outlived its window")
	}
}

func TestSuccessfulSignInClearsTheCounter(t *testing.T) {
	th := newThrottle()
	key := loginKey("admin", loginRequest("10.0.0.1:5555", ""))

	for i := 0; i < maxLoginAttempts-1; i++ {
		th.fail(key)
	}
	th.reset(key)

	for i := 0; i < maxLoginAttempts-1; i++ {
		th.fail(key)
		if th.blocked(key) {
			t.Fatal("the counter was not cleared by a successful sign-in")
		}
	}
}

// Keying on the username alone would let anyone lock a real admin out by
// guessing at their name from anywhere.
func TestOneAddressCannotLockOutAnAdminElsewhere(t *testing.T) {
	th := newThrottle()
	attacker := loginKey("admin", loginRequest("10.0.0.1:5555", ""))
	victim := loginKey("admin", loginRequest("10.0.0.2:5555", ""))

	for i := 0; i < maxLoginAttempts*2; i++ {
		th.fail(attacker)
	}

	if !th.blocked(attacker) {
		t.Error("the attacker was not locked out")
	}
	if th.blocked(victim) {
		t.Error("the real admin was locked out from their own address")
	}
}

// Keying on the address alone would let one host walk through every account.
func TestOneAddressCannotWalkThroughAccountsFreely(t *testing.T) {
	th := newThrottle()
	r := loginRequest("10.0.0.1:5555", "")

	first := loginKey("alice", r)
	for i := 0; i < maxLoginAttempts; i++ {
		th.fail(first)
	}

	if !th.blocked(first) {
		t.Fatal("expected a lockout on the first account")
	}
	// A different username from the same host is a different key by design —
	// what matters is that each one still costs the attacker a full lockout.
	second := loginKey("bob", r)
	for i := 0; i < maxLoginAttempts; i++ {
		th.fail(second)
	}
	if !th.blocked(second) {
		t.Error("the second account was never locked out")
	}
}

func TestClientIPPrefersTheFirstForwardedHop(t *testing.T) {
	cases := []struct {
		name, remoteAddr, xff, want string
	}{
		{"no proxy", "203.0.113.9:41234", "", "203.0.113.9"},
		{"single hop", "10.0.0.1:5555", "198.51.100.7", "198.51.100.7"},
		{"chain", "10.0.0.1:5555", "198.51.100.7, 10.0.0.2", "198.51.100.7"},
		{"padded chain", "10.0.0.1:5555", "  198.51.100.7 , 10.0.0.2", "198.51.100.7"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := clientIP(loginRequest(c.remoteAddr, c.xff)); got != c.want {
				t.Errorf("clientIP = %q, want %q", got, c.want)
			}
		})
	}
}
