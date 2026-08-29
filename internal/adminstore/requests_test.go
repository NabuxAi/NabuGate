package adminstore

import (
	"testing"
	"time"
)

func TestTheLogReturnsNewestFirst(t *testing.T) {
	l := NewRequestLog(10)
	for _, m := range []string{"first", "second", "third"} {
		l.Add(RequestEntry{Project: "app", Model: m})
	}
	got := l.Recent(nil, 10)
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3", len(got))
	}
	if got[0].Model != "third" || got[2].Model != "first" {
		t.Errorf("wrong order: %v", []string{got[0].Model, got[1].Model, got[2].Model})
	}
}

func TestTheOldestEntriesAreOverwritten(t *testing.T) {
	l := NewRequestLog(3)
	for _, m := range []string{"a", "b", "c", "d", "e"} {
		l.Add(RequestEntry{Project: "app", Model: m})
	}
	got := l.Recent(nil, 10)
	if len(got) != 3 {
		t.Fatalf("the ring grew past its size: %d entries", len(got))
	}
	// Newest three, newest first. If the wrap-around index is wrong this comes
	// back rotated rather than short, which a length check alone would miss.
	for i, want := range []string{"e", "d", "c"} {
		if got[i].Model != want {
			t.Errorf("entry %d = %q, want %q", i, got[i].Model, want)
		}
	}
}

// An unauthenticated caller looks exactly like an empty owner. The failure mode
// worth ruling out is that they are handed the whole deployment's traffic.
func TestAnEmptyProjectSetSeesNothingRatherThanEverything(t *testing.T) {
	l := NewRequestLog(10)
	l.Add(RequestEntry{Project: "somebody-elses-app", Model: "nabu-fast"})

	if got := l.Recent(map[string]bool{}, 10); len(got) != 0 {
		t.Fatalf("an empty project set returned %d entries", len(got))
	}
	// nil is the administrator's case and must still see it, or the check above
	// would pass by returning nothing to anyone.
	if got := l.Recent(nil, 10); len(got) != 1 {
		t.Fatalf("nil (administrator) returned %d entries, want 1", len(got))
	}
}

func TestOneOwnerDoesNotSeeAnothersTraffic(t *testing.T) {
	l := NewRequestLog(10)
	l.Add(RequestEntry{Project: "mine", Model: "nabu-fast"})
	l.Add(RequestEntry{Project: "theirs", Model: "nabu-smart"})

	got := l.Recent(map[string]bool{"mine": true}, 10)
	if len(got) != 1 || got[0].Project != "mine" {
		t.Fatalf("owner filter leaked: %+v", got)
	}
}

// Project names are matched without regard to case, as they are everywhere else
// the store compares an owner or a project — otherwise "MyApp" and "myapp" are
// two projects and one of them is invisible to the person who made it.
func TestProjectMatchingIgnoresCase(t *testing.T) {
	l := NewRequestLog(10)
	l.Add(RequestEntry{Project: "MyApp", Model: "nabu-fast"})

	if got := l.Recent(map[string]bool{"myapp": true}, 10); len(got) != 1 {
		t.Fatalf("a project differing only in case was hidden from its owner")
	}
}

func TestAnUnusedRingReportsNoTraffic(t *testing.T) {
	l := NewRequestLog(5)
	// Zero-valued slots must not be reported as calls that happened at the zero
	// time — the ring is allocated full-length from the start.
	if got := l.Recent(nil, 10); len(got) != 0 {
		t.Fatalf("an empty log returned %d entries", len(got))
	}
}

func TestTheLimitIsRespected(t *testing.T) {
	l := NewRequestLog(50)
	for i := 0; i < 20; i++ {
		l.Add(RequestEntry{Project: "app", At: time.Now()})
	}
	if got := l.Recent(nil, 5); len(got) != 5 {
		t.Fatalf("limit 5 returned %d entries", len(got))
	}
}

// A nil log is what a Server built without the constructor has. Recording into
// one must be a no-op rather than a panic, or a test helper that builds a
// Server by hand takes the gateway down on its first billed call.
func TestANilLogIsSafe(t *testing.T) {
	var l *RequestLog
	l.Add(RequestEntry{Project: "app"})
	if got := l.Recent(nil, 10); got != nil {
		t.Fatalf("a nil log returned %v", got)
	}
}
