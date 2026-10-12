package adminstore

import (
	"os"
	"path/filepath"
	"testing"
)

// Counters written by an older build carry none of the honesty counters or
// the physical units. Loading such a state file must leave them at their zero
// values rather than failing to open — the console keeps reading the numbers
// it always read, and the new ones simply start counting from here.
func TestOldStateFileWithoutTheNewCountersStillLoads(t *testing.T) {
	old := `{
	  "usage": {"desk": {"requests": 3, "prompt_tokens": 100, "completion_tokens": 40, "cost_usd": 0.5, "provider_cost_usd": 0.4}},
	  "usage_by_model": {"openai/gpt": {"requests": 3, "cost_usd": 0.5}},
	  "usage_by_prov": {"openai": {"requests": 3, "cost_usd": 0.5}},
	  "sessions": {},
	  "user_sessions": {}
	}`
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("open old state: %v", err)
	}
	projects, models, provs := s.Usage()
	if c := projects["desk"]; c.Requests != 3 || c.CostUSD != 0.5 {
		t.Fatalf("desk counters = %+v", c)
	}
	if c := projects["desk"]; c.UnpricedRequests != 0 || c.EstimatedRequests != 0 || c.FailedRequests != 0 ||
		c.AudioSeconds != 0 || c.Images != 0 || c.Characters != 0 {
		t.Fatalf("old state grew non-zero new counters: %+v", c)
	}
	if c := models["openai/gpt"]; c.Requests != 3 {
		t.Fatalf("model counters = %+v", c)
	}
	if c := provs["openai"]; c.Requests != 3 {
		t.Fatalf("provider counters = %+v", c)
	}

	// The first new entry on top of the old numbers adds only its own fields:
	// the older spend is untouched, the newer fields start from zero and grow.
	s.RecordUsage("desk", "openai", "gpt", UsageEntry{
		Requests:         1,
		PromptTokens:     10,
		CompletionTokens: 5,
		Unpriced:         true,
		Estimated:        true,
		Failed:           true,
		AudioSeconds:     12.5,
		Images:           2,
		Characters:       700,
	})
	projects, _, _ = s.Usage()
	c := projects["desk"]
	if c.Requests != 4 || c.PromptTokens != 110 || c.CostUSD != 0.5 || c.ProviderCostUSD != 0.4 {
		t.Errorf("accumulated counters = %+v", c)
	}
	if c.UnpricedRequests != 1 || c.EstimatedRequests != 1 || c.FailedRequests != 1 {
		t.Errorf("honesty counters = %d/%d/%d, want 1/1/1", c.UnpricedRequests, c.EstimatedRequests, c.FailedRequests)
	}
	if c.AudioSeconds != 12.5 || c.Images != 2 || c.Characters != 700 {
		t.Errorf("units = %v/%d/%d, want 12.5/2/700", c.AudioSeconds, c.Images, c.Characters)
	}
}

// A failed booking keeps its vendor cost visible while counting no served
// request — that is the whole point of the flag, so pin it at the store's
// own level too.
func TestFailedEntryCostsTheVendorWithoutCountingARequest(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.RecordUsage("desk", "openai", "gpt", UsageEntry{
		Requests:        0,
		PromptTokens:    90,
		ProviderCostUSD: 0.01,
		Failed:          true,
	})
	projects, _, _ := s.Usage()
	c := projects["desk"]
	if c.Requests != 0 {
		t.Errorf("requests = %d, want 0 — a failed call is not a served call", c.Requests)
	}
	if c.FailedRequests != 1 {
		t.Errorf("failed_requests = %d, want 1", c.FailedRequests)
	}
	if c.PromptTokens != 90 || c.ProviderCostUSD != 0.01 {
		t.Errorf("counters = %+v", c)
	}
}
