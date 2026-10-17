package server

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

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
