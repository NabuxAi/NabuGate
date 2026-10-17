package server

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"nabugate/internal/adminstore"
	"nabugate/internal/policy"
)

func authProbe(t *testing.T) (*Server, *adminstore.Store, http.HandlerFunc) {
	t.Helper()
	store, err := adminstore.Open(filepath.Join(t.TempDir(), "admin.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{admin: store, policy: policy.New(nil, nil), log: slog.New(slog.DiscardHandler),
		requests: adminstore.NewRequestLog(10)}
	h := s.auth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	return s, store, h
}

func hit(h http.HandlerFunc, key string) int {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	if key != "" {
		r.Header.Set("Authorization", "Bearer "+key)
	}
	w := httptest.NewRecorder()
	h(w, r)
	return w.Code
}

// A deployment whose config keys are all unset used to serve everyone
// unauthenticated, even with customers' console keys minted.
func TestConsoleTokensTurnAuthOnWithoutConfigKeys(t *testing.T) {
	_, store, h := authProbe(t)
	if code := hit(h, ""); code != http.StatusNoContent {
		t.Fatalf("a keyless checkout should stay open for development: %d", code)
	}
	if err := store.SignupUser("buyer@example.com", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	store.AddPayment("buyer@example.com", 5, "success", "manual-1")
	_, secret, err := store.NewToken("buyer-key", []string{"*"}, 0, nil, "buyer@example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	if code := hit(h, ""); code != http.StatusUnauthorized {
		t.Fatalf("with console keys minted, a keyless call got %d", code)
	}
	if code := hit(h, secret); code != http.StatusNoContent {
		t.Fatalf("the customer's own key was refused: %d", code)
	}
}

// A console token's own rate limit is applied, not only config keys'.
func TestConsoleTokenRateLimitIsEnforced(t *testing.T) {
	_, store, h := authProbe(t)
	store.SignupUser("buyer@example.com", "a-long-password")
	store.AddPayment("buyer@example.com", 5, "success", "manual-1")
	_, secret, _ := store.NewToken("buyer-key", []string{"*"}, 2, nil, "buyer@example.com", nil)
	codes := []int{hit(h, secret), hit(h, secret), hit(h, secret)}
	if codes[0] != http.StatusNoContent || codes[2] != http.StatusTooManyRequests {
		t.Fatalf("codes %v, want the third refused at 2/min", codes)
	}
}

// A small balance cannot fan out into many concurrent calls admitted against
// money the others are about to spend.
func TestConcurrentRequestsAreHeldAgainstTheBalance(t *testing.T) {
	s, store, _ := authProbe(t)
	store.SignupUser("buyer@example.com", "a-long-password")
	store.AddPayment("buyer@example.com", 0.01, "success", "manual-1")
	_, secret, _ := store.NewToken("buyer-key", []string{"*"}, 0, nil, "buyer@example.com", nil)

	entered, release := make(chan struct{}), make(chan struct{})
	h := s.auth(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		w.WriteHeader(http.StatusNoContent)
	})
	go hit(h, secret)
	<-entered
	// One call is running against the last cent; a second must wait.
	if code := hit(s.auth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }), secret); code != http.StatusPaymentRequired {
		t.Fatalf("second concurrent call got %d", code)
	}
	close(release)
	// Once it finished, the next one is admitted again.
	deadline := time.Now().Add(time.Second)
	for {
		code := hit(s.auth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }), secret)
		if code == http.StatusNoContent {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("hold was never released: %d", code)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
