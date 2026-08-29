package adminstore

import (
	"path/filepath"
	"testing"
)

func ownerStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "console.json"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return st
}

// A user panel exists to show somebody their own keys. Everything here is about
// the one way that can go wrong.
func TestOwnerScopingKeepsKeysApart(t *testing.T) {
	st := ownerStore(t)
	if _, _, err := st.NewToken("alice-app", []string{"nabu-*"}, 60, nil, "Alice@nabuxai.com", nil); err != nil {
		t.Fatalf("mint alice: %v", err)
	}
	if _, _, err := st.NewToken("bob-app", []string{"nabu-*"}, 60, nil, "bob@nabuxai.com", nil); err != nil {
		t.Fatalf("mint bob: %v", err)
	}

	alice := st.TokensForOwner("alice@nabuxai.com")
	if len(alice) != 1 || alice[0].Name != "alice-app" {
		t.Fatalf("alice sees %d keys: %+v", len(alice), alice)
	}
	// Owners are compared without regard to case, because the address that was
	// stored came from whatever the person typed and the one being looked up
	// came from a session.
	if len(st.TokensForOwner("ALICE@NABUXAI.COM")) != 1 {
		t.Fatal("owner lookup is case-sensitive; the same person would see nothing")
	}

	// The secret never comes back out, not even to its owner.
	if alice[0].Hash != "" {
		t.Fatal("TokensForOwner returned the token hash")
	}
}

func TestAnEmptyOwnerSeesNothingRatherThanEverything(t *testing.T) {
	st := ownerStore(t)
	st.NewToken("someones-app", []string{"nabu-*"}, 60, nil, "someone@nabuxai.com", nil)

	// An unauthenticated caller looks exactly like an empty owner string. If
	// that matched every key with no owner set — or worse, all of them — the
	// panel would hand the whole deployment's keys to a stranger.
	if got := st.TokensForOwner(""); len(got) != 0 {
		t.Fatalf("an empty owner saw %d keys", len(got))
	}
	if got := st.TokensForOwner("   "); len(got) != 0 {
		t.Fatalf("a blank owner saw %d keys", len(got))
	}
}

func TestDeleteRefusesSomebodyElsesKey(t *testing.T) {
	st := ownerStore(t)
	st.NewToken("bob-app", []string{"nabu-*"}, 60, nil, "bob@nabuxai.com", nil)

	// Alice must not be able to delete Bob's key, and must not learn it exists.
	if err := st.DeleteTokenForOwner("alice@nabuxai.com", "bob-app"); err != ErrTokenNotFound {
		t.Fatalf("deleting another owner's key returned %v, want ErrTokenNotFound", err)
	}
	if len(st.TokensForOwner("bob@nabuxai.com")) != 1 {
		t.Fatal("Bob's key was deleted by Alice")
	}
	if err := st.DeleteTokenForOwner("", "bob-app"); err != ErrTokenNotFound {
		t.Fatalf("an empty owner deleted a key: %v", err)
	}

	if err := st.DeleteTokenForOwner("bob@nabuxai.com", "bob-app"); err != nil {
		t.Fatalf("Bob could not delete his own key: %v", err)
	}
	if len(st.TokensForOwner("bob@nabuxai.com")) != 0 {
		t.Fatal("the key survived its own owner deleting it")
	}
}

func TestUsageIsScopedToTheOwnersProjects(t *testing.T) {
	st := ownerStore(t)
	st.NewToken("alice-app", []string{"nabu-*"}, 60, nil, "alice@nabuxai.com", nil)
	st.NewToken("bob-app", []string{"nabu-*"}, 60, nil, "bob@nabuxai.com", nil)

	st.RecordUsage("alice-app", "gemini", "nabu-fast", 100, 50, 0.001)
	st.RecordUsage("bob-app", "gemini", "nabu-fast", 900, 400, 0.09)

	usage := st.UsageForOwner("alice@nabuxai.com")
	if len(usage) != 1 {
		t.Fatalf("alice's usage covers %d projects: %+v", len(usage), usage)
	}
	if _, leaked := usage["bob-app"]; leaked {
		t.Fatal("bob's spend appeared in alice's usage")
	}
	if got := usage["alice-app"].PromptTokens; got != 100 {
		t.Fatalf("alice's prompt tokens = %d, want 100", got)
	}
}
