package adminstore

import (
	"path/filepath"
	"testing"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "console.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}

func TestFirstAdminThenLogin(t *testing.T) {
	s := newStore(t)

	if !s.NeedsSetup() {
		t.Fatal("a fresh store should report that it needs setup")
	}
	if err := s.CreateAdmin("Hussein", "a-long-enough-passphrase"); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	if s.NeedsSetup() {
		t.Error("setup should be complete once an admin exists")
	}

	// Usernames are case-insensitive, or the account someone created is not the
	// one they can sign in to.
	token, _, err := s.Authenticate("hussein", "a-long-enough-passphrase")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if _, ok := s.ValidSession(token); !ok {
		t.Error("the returned session was not valid")
	}

	if err := s.EndSession(token); err != nil {
		t.Fatalf("EndSession: %v", err)
	}
	if _, ok := s.ValidSession(token); ok {
		t.Error("the session survived logout")
	}
}

func TestWrongPasswordAndUnknownUserAreIndistinguishable(t *testing.T) {
	s := newStore(t)
	if err := s.CreateAdmin("hussein", "a-long-enough-passphrase"); err != nil {
		t.Fatal(err)
	}

	_, _, wrongPass := s.Authenticate("hussein", "not-the-password")
	_, _, noSuchUser := s.Authenticate("nobody", "not-the-password")

	// Same error text either way: distinguishing them turns the login form into
	// an account enumerator.
	if wrongPass == nil || noSuchUser == nil {
		t.Fatal("both cases must fail")
	}
	if wrongPass.Error() != noSuchUser.Error() {
		t.Errorf("errors differ: %q vs %q", wrongPass, noSuchUser)
	}
}

func TestShortPasswordRefused(t *testing.T) {
	s := newStore(t)
	if err := s.CreateAdmin("hussein", "short"); err == nil {
		t.Error("a short password was accepted")
	}
}

func TestTokenSecretIsShownOnceAndStoredHashed(t *testing.T) {
	s := newStore(t)

	tok, secret, err := s.NewToken("nabuwrite", []string{"write-*"}, 120, nil, "admin@test.com", nil)
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	if secret == "" {
		t.Fatal("no secret returned")
	}
	if tok.Hash == secret {
		t.Fatal("the secret was stored verbatim")
	}

	got, ok := s.Lookup(secret)
	if !ok || got.Name != "nabuwrite" {
		t.Fatalf("Lookup(secret) = %+v, %v; want the nabuwrite token", got, ok)
	}
	if _, ok := s.Lookup("ng-not-a-real-token"); ok {
		t.Error("an unknown secret resolved to a token")
	}

	// Listing must not carry hashes to a browser: they are offline-crackable
	// and nothing outside the store needs them.
	for _, l := range s.Tokens() {
		if l.Hash != "" {
			t.Error("Tokens() exposed a hash")
		}
	}
}

func TestTokenRequiresAnAllowList(t *testing.T) {
	s := newStore(t)
	// A key that reaches everything is an admin key, and minting one from a
	// console form is how that happens by accident.
	if _, _, err := s.NewToken("careless", nil, 0, nil, "admin@test.com", nil); err == nil {
		t.Error("a token with no allow-list was created")
	}
}

func TestDisabledTokenStopsResolving(t *testing.T) {
	s := newStore(t)
	_, secret, err := s.NewToken("nabuwrite", []string{"write-*"}, 0, nil, "admin@test.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetTokenDisabled("nabuwrite", true); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Lookup(secret); ok {
		t.Error("a disabled token still authenticated")
	}
}

func TestUsageSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "console.json")

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.RecordUsage("nabuwrite", "openai", "gpt-4", 100, 40, 0.002)
	s.RecordUsage("nabuwrite", "openai", "gpt-4", 50, 10, 0.001)
	s.RecordDenied("nabuwrite")
	if err := s.Persist(); err != nil {
		t.Fatalf("Persist: %v", err)
	}

	// The whole point of persisting: a redeploy used to reset every counter to
	// zero, which made the console look like nothing had ever run.
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.Usage()["nabuwrite"]
	if got.Requests != 2 || got.PromptTokens != 150 || got.CompletionTokens != 50 || got.Denied != 1 {
		t.Errorf("counters after reopen = %+v", got)
	}
}

func TestDuplicateProjectNameRefused(t *testing.T) {
	s := newStore(t)
	if _, _, err := s.NewToken("nabuwrite", []string{"write-*"}, 0, nil, "admin@test.com", nil); err != nil {
		t.Fatal(err)
	}
	// Two tokens under one name would split that project's usage in two.
	if _, _, err := s.NewToken("NabuWrite", []string{"write-*"}, 0, nil, "admin@test.com", nil); err == nil {
		t.Error("a duplicate project name was accepted")
	}
}
