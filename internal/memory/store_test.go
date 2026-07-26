package memory

import (
	"strings"
	"testing"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}

func TestAppendAndLoad(t *testing.T) {
	s := newStore(t)

	if _, err := s.Append("app-a", "conv1",
		Message{Role: "user", Content: "سلام"},
		Message{Role: "assistant", Content: "سلام! چطور می‌توانم کمک کنم؟"},
	); err != nil {
		t.Fatalf("Append: %v", err)
	}

	c, err := s.Load("app-a", "conv1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Messages) != 2 || c.Messages[0].Content != "سلام" {
		t.Fatalf("messages = %+v", c.Messages)
	}
	if c.Messages[0].At.IsZero() {
		t.Error("a stored turn has no timestamp")
	}
}

// The property the whole design rests on: one project must never be able to
// read another's conversation, however it asks.
func TestProjectsCannotSeeEachOther(t *testing.T) {
	s := newStore(t)

	if _, err := s.Append("app-a", "shared-id", Message{Role: "user", Content: "a's secret"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append("app-b", "shared-id", Message{Role: "user", Content: "b's secret"}); err != nil {
		t.Fatal(err)
	}

	a, err := s.Load("app-a", "shared-id")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Load("app-b", "shared-id")
	if err != nil {
		t.Fatal(err)
	}

	// Same id, two projects, two separate conversations.
	if a.Messages[0].Content != "a's secret" || b.Messages[0].Content != "b's secret" {
		t.Fatalf("conversations bled across projects: a=%q b=%q",
			a.Messages[0].Content, b.Messages[0].Content)
	}

	list, err := s.List("app-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Errorf("app-a sees %d conversations; it owns 1", len(list))
	}
}

func TestConversationIDCannotBeAPath(t *testing.T) {
	s := newStore(t)

	// A client supplies this value, so it must never be usable to walk out of
	// the project directory or land on another project's file.
	for _, bad := range []string{
		"../app-b/shared-id",
		"..",
		"a/b",
		"a\\b",
		"conv 1",
		"",
		strings.Repeat("x", MaxIDLen+1),
	} {
		if _, err := s.Append("app-a", bad, Message{Role: "user", Content: "x"}); err == nil {
			t.Errorf("id %q was accepted", bad)
		}
		if _, err := s.Load("app-a", bad); err == nil {
			t.Errorf("id %q was loadable", bad)
		}
	}
}

func TestAdminKeyWithNoProjectIsRefused(t *testing.T) {
	s := newStore(t)
	// An admin key has no project, so there is no namespace to store under.
	// Falling back to a shared one would put every project's history together.
	if _, err := s.Append("", "conv1", Message{Role: "user", Content: "x"}); err == nil {
		t.Error("a conversation was stored without a project")
	}
}

func TestTrimKeepsTheRecentTurns(t *testing.T) {
	s := newStore(t)

	for i := 0; i < MaxTurns+15; i++ {
		if _, err := s.Append("app-a", "long", Message{Role: "user", Content: string(rune('a' + i%26))}); err != nil {
			t.Fatal(err)
		}
	}
	c, err := s.Load("app-a", "long")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Messages) != MaxTurns {
		t.Errorf("kept %d messages, want %d", len(c.Messages), MaxTurns)
	}
}

func TestTrimBoundsTotalSize(t *testing.T) {
	s := newStore(t)

	// A few very long turns: the count limit would let these through, but the
	// character limit is what stops a conversation from outgrowing the model's
	// context.
	big := strings.Repeat("x", MaxChars/3)
	for i := 0; i < 6; i++ {
		if _, err := s.Append("app-a", "big", Message{Role: "user", Content: big}); err != nil {
			t.Fatal(err)
		}
	}

	c, err := s.Load("app-a", "big")
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, m := range c.Messages {
		total += len(m.Content)
	}
	if total > MaxChars {
		t.Errorf("stored %d chars, over the %d limit", total, MaxChars)
	}
	if len(c.Messages) == 0 {
		t.Error("trimming removed everything")
	}
}

func TestEmptyContentIsNotStored(t *testing.T) {
	s := newStore(t)
	if _, err := s.Append("app-a", "c", Message{Role: "assistant", Content: "   "}); err != nil {
		t.Fatal(err)
	}
	c, err := s.Load("app-a", "c")
	if err != nil {
		t.Fatal(err)
	}
	// An empty assistant turn would be replayed into every later call.
	if len(c.Messages) != 0 {
		t.Errorf("stored %d empty messages", len(c.Messages))
	}
}

func TestListReturnsSummariesNotFullHistory(t *testing.T) {
	s := newStore(t)
	if _, err := s.Append("app-a", "c1", Message{Role: "user", Content: "hello"}); err != nil {
		t.Fatal(err)
	}
	list, err := s.List("app-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("got %d conversations", len(list))
	}
	// Listing every message of every conversation would ship the project's
	// whole history on one call.
	if list[0].Messages != nil {
		t.Error("List returned message bodies")
	}
}

func TestDelete(t *testing.T) {
	s := newStore(t)
	if _, err := s.Append("app-a", "c1", Message{Role: "user", Content: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("app-a", "c1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Load("app-a", "c1"); err != ErrNotFound {
		t.Errorf("Load after delete = %v, want ErrNotFound", err)
	}
	// Deleting another project's conversation must not succeed either.
	if err := s.Delete("app-b", "c1"); err != ErrNotFound {
		t.Errorf("cross-project delete = %v, want ErrNotFound", err)
	}
}
