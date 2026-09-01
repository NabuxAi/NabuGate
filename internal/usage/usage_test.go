package usage

import (
	"math"
	"testing"

	"nabugate/internal/provider"
)

// Cost is the number that leaves a customer's balance. The price table is USD
// per million tokens, split by direction; a slip in either the divisor or the
// direction would over- or under-charge every call by orders of magnitude.
func TestCostSplitsByDirectionPerMillionTokens(t *testing.T) {
	tr := New(map[string]Price{
		"openai/gpt-4o": {Input: 2.5, Output: 10},
	})
	got := tr.Cost("openai", "gpt-4o", provider.Usage{PromptTokens: 1_000_000, CompletionTokens: 100_000})
	if want := 2.5 + 1.0; math.Abs(got-want) > 1e-9 {
		t.Fatalf("cost = %v, want %v", got, want)
	}
}

// An unpriced model costs nothing. That is the current contract and callers
// rely on it, but it also means a model missing from the price table is served
// free — so the behaviour is pinned here, where a change to it is deliberate.
func TestUnpricedModelCostsZero(t *testing.T) {
	tr := New(nil)
	if got := tr.Cost("openai", "gpt-4o", provider.Usage{PromptTokens: 5000}); got != 0 {
		t.Fatalf("unpriced cost = %v, want 0", got)
	}
}

// Record attributes to both the project and the provider/model key, returns
// the cost it charged, and files an unscoped call under a stable name rather
// than the empty string.
func TestRecordAttributesToProjectAndModel(t *testing.T) {
	tr := New(map[string]Price{"anthropic/claude": {Input: 3, Output: 15}})
	u := provider.Usage{PromptTokens: 2_000_000, CompletionTokens: 1_000_000}

	cost := tr.Record("desk", "anthropic", "claude", u)
	if want := 6.0 + 15.0; math.Abs(cost-want) > 1e-9 {
		t.Fatalf("recorded cost = %v, want %v", cost, want)
	}
	tr.Record("", "anthropic", "claude", u)

	byProject, byModel := tr.Snapshot()
	if s := byProject["desk"]; s.Requests != 1 || s.PromptTokens != 2_000_000 || s.CompletionTokens != 1_000_000 || math.Abs(s.CostUSD-21) > 1e-9 {
		t.Fatalf("desk stat = %+v", s)
	}
	if s := byProject["(unscoped)"]; s.Requests != 1 {
		t.Fatalf("unscoped stat = %+v, want one request", s)
	}
	if s := byModel["anthropic/claude"]; s.Requests != 2 || math.Abs(s.CostUSD-42) > 1e-9 {
		t.Fatalf("model stat = %+v", s)
	}
	if s := tr.ProjectSnapshot("desk"); s.Requests != 1 {
		t.Fatalf("ProjectSnapshot(desk) = %+v", s)
	}
}

// Snapshot hands back copies: mutating what it returns must not reach the
// tracker, or a dashboard reading usage could corrupt what is billed.
func TestSnapshotIsACopy(t *testing.T) {
	tr := New(nil)
	tr.Record("p", "x", "m", provider.Usage{PromptTokens: 1})
	byProject, _ := tr.Snapshot()
	s := byProject["p"]
	s.Requests = 99
	byProject["p"] = s
	if got := tr.ProjectSnapshot("p").Requests; got != 1 {
		t.Fatalf("tracker requests = %d after mutating snapshot, want 1", got)
	}
}
