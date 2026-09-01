package adminstore

import (
	"path/filepath"
	"testing"
)

// Usage under a token whose name differs from the project only by case must
// still bill its owner. Everything else in the store matches token names
// case-insensitively; the debit path compared exactly, so such a key ran free.
func TestUsageDebitsTheOwnerWhateverTheCase(t *testing.T) {
	st := ownerStore(t)
	st.NewToken("Alice-App", []string{"nabu-*"}, 60, nil, "alice@nabuxai.com", nil)
	st.AddPayment("alice@nabuxai.com", 10, "success", "inv-1")

	st.RecordUsage("alice-app", "gemini", "nabu-fast", 100, 50, 2.5)

	if got := st.GetUser("alice@nabuxai.com").Balance; got != 7.5 {
		t.Fatalf("balance = %v after a $2.50 call, want 7.5", got)
	}
}

// Debits ride the periodic flush, except the one that empties the account:
// that must reach disk at once, or a restart serves the customer again on
// money they already spent.
func TestTheDebitThatExhaustsABalanceIsPersistedImmediately(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admin.json")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	st.NewToken("bob-app", []string{"nabu-*"}, 60, nil, "bob@nabuxai.com", nil)
	st.AddPayment("bob@nabuxai.com", 1, "success", "inv-1")

	// Well short of zero: stays in memory only, as a per-request write would
	// cost more than the completion it records.
	st.RecordUsage("bob-app", "gemini", "nabu-fast", 10, 5, 0.25)
	if got := mustOpen(t, path).GetUser("bob@nabuxai.com").Balance; got != 1 {
		t.Fatalf("an ordinary debit was flushed eagerly: on-disk balance %v, want 1", got)
	}

	// The call that crosses zero is written through.
	st.RecordUsage("bob-app", "gemini", "nabu-fast", 10, 5, 0.80)
	if got := mustOpen(t, path).GetUser("bob@nabuxai.com").Balance; got > 0 {
		t.Fatalf("exhausting debit not on disk: on-disk balance %v, want <= 0", got)
	}
}

func mustOpen(t *testing.T, path string) *Store {
	t.Helper()
	st, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	return st
}
