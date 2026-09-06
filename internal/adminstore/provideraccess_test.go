package adminstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func storeWithSecret(t *testing.T, secret string) *Store {
	t.Helper()
	t.Setenv("NABUGATE_SECRET_KEY", secret)
	s, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return s
}

func TestProviderKeyRoundTrip(t *testing.T) {
	s := storeWithSecret(t, "a master secret")
	if !s.SecretConfigured() {
		t.Fatal("secret not configured")
	}
	if err := s.SetProviderKey("Me@Example.com", "Gemini", "AIza-super-secret-value"); err != nil {
		t.Fatalf("set: %v", err)
	}

	// The console sees which key is saved, never the key.
	prefixes := s.ProviderKeyPrefixes("me@example.com")
	if got := prefixes["gemini"]; got == "" || strings.Contains(got, "super") {
		t.Errorf("prefix = %q; it must identify the key without revealing it", got)
	}

	if got := s.ProviderKeys("me@example.com")["gemini"]; got != "AIza-super-secret-value" {
		t.Errorf("decrypted = %q", got)
	}

	// Nothing on disk holds the plaintext.
	raw := mustReadState(t, s)
	if strings.Contains(raw, "AIza-super-secret-value") {
		t.Fatal("the key was written to disk in the clear")
	}

	if err := s.DeleteProviderKey("me@example.com", "gemini"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(s.ProviderKeys("me@example.com")) != 0 {
		t.Error("key survived deletion")
	}
}

// No secret means refusing, not degrading. A silent plaintext write is the
// breach this exists to prevent.
func TestProviderKeyRefusedWithoutSecret(t *testing.T) {
	s := storeWithSecret(t, "")
	if s.SecretConfigured() {
		t.Fatal("reported a secret it does not have")
	}
	err := s.SetProviderKey("me@example.com", "gemini", "sk-1")
	if err != ErrNoSecret {
		t.Fatalf("err = %v, want ErrNoSecret", err)
	}
	raw := mustReadState(t, s)
	if strings.Contains(raw, "sk-1") {
		t.Fatal("the key was stored anyway")
	}
}

// A rotated secret must cost one stored credential, not the gateway.
func TestProviderKeySurvivesRotationAsSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	t.Setenv("NABUGATE_SECRET_KEY", "first secret")
	s1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s1.SetProviderKey("me@example.com", "gemini", "sk-old"); err != nil {
		t.Fatal(err)
	}
	if err := s1.Persist(); err != nil {
		t.Fatal(err)
	}

	t.Setenv("NABUGATE_SECRET_KEY", "a different secret")
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Still listed — the user can see it is there and replace it — but not
	// usable, and reading it does not error.
	if _, ok := s2.ProviderKeyPrefixes("me@example.com")["gemini"]; !ok {
		t.Error("the stored key vanished from the listing")
	}
	if got := s2.ProviderKeys("me@example.com"); len(got) != 0 {
		t.Errorf("keys = %v, want the undecryptable one skipped", got)
	}
}

func TestProviderRequestAutoAndManual(t *testing.T) {
	s := storeWithSecret(t, "x")

	// Auto: granted on the spot, with credit if the deployment gives any.
	req, err := s.RequestProvider("me@example.com", "groq", "", true, 2.5)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if req.Status != StatusApproved || !req.Auto {
		t.Errorf("auto request = %+v", req)
	}
	if u := s.GetUser("me@example.com"); u == nil || u.Balance != 2.5 {
		t.Errorf("credit not applied: %+v", u)
	}

	// Manual: waits.
	req2, err := s.RequestProvider("me@example.com", "openai", "برای پروژهٔ رونویسی", false, 0)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if req2.Status != StatusPending {
		t.Errorf("status = %q", req2.Status)
	}
	if req2.DecidedAt != nil {
		t.Errorf("a pending request has a decision date: %v", req2.DecidedAt)
	}
	// Asking again must not reset the queue position or duplicate the row.
	again, _ := s.RequestProvider("me@example.com", "openai", "دوباره", false, 0)
	if again.CreatedAt != req2.CreatedAt {
		t.Error("asking twice restarted the request")
	}
	if got := len(s.ProviderRequestsFor("me@example.com")); got != 2 {
		t.Errorf("requests = %d, want 2", got)
	}

	approved := s.ApprovedProviders("me@example.com")
	if !approved["groq"] || approved["openai"] {
		t.Errorf("approved = %v", approved)
	}

	// Approval and credit are one decision.
	decided, err := s.DecideProviderRequest(req2.ID, true, 5, "تأیید شد", "admin@example.com")
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if decided.Status != StatusApproved || decided.DecidedBy != "admin@example.com" {
		t.Errorf("decided = %+v", decided)
	}
	if u := s.GetUser("me@example.com"); u.Balance != 7.5 {
		t.Errorf("balance = %v, want 2.5 + 5", u.Balance)
	}
	if !s.ApprovedProviders("me@example.com")["openai"] {
		t.Error("approval did not take")
	}
}

// A denial is not a wall: asking again reopens it rather than needing an admin
// to find the old row.
func TestDeniedRequestCanBeReopened(t *testing.T) {
	s := storeWithSecret(t, "x")
	req, _ := s.RequestProvider("me@example.com", "openai", "", false, 0)
	if _, err := s.DecideProviderRequest(req.ID, false, 0, "الان نه", "admin"); err != nil {
		t.Fatal(err)
	}
	if s.ApprovedProviders("me@example.com")["openai"] {
		t.Fatal("denied request counted as approved")
	}
	again, err := s.RequestProvider("me@example.com", "openai", "دلیل تازه", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if again.Status != StatusPending || again.Decision != "" {
		t.Errorf("reopened request = %+v", again)
	}
	// A pending request has no decision date. Serialising the zero time made
	// every client render a decision in the year one.
	if again.DecidedAt != nil {
		t.Errorf("reopened request carries a decision date: %v", again.DecidedAt)
	}
}

// mustReadState reads the persisted file, so a test can assert what did and did
// not reach the disk.
func mustReadState(t *testing.T, s *Store) string {
	t.Helper()
	if err := s.Persist(); err != nil {
		t.Fatalf("persist: %v", err)
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	return string(raw)
}
