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
	got := tr.Cost("openai", "gpt-4o", Tokens(provider.Usage{PromptTokens: 1_000_000, CompletionTokens: 100_000}))
	if want := 2.5 + 1.0; math.Abs(got-want) > 1e-9 {
		t.Fatalf("cost = %v, want %v", got, want)
	}
}

// An unpriced model costs nothing. That is the current contract and callers
// rely on it, but it also means a model missing from the price table is served
// free — so the behaviour is pinned here, where a change to it is deliberate.
func TestUnpricedModelCostsZero(t *testing.T) {
	tr := New(nil)
	if got := tr.Cost("openai", "gpt-4o", Tokens(provider.Usage{PromptTokens: 5000})); got != 0 {
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

// A passthrough router's whole point is that a caller may name any model in a
// catalogue of hundreds that changes weekly, so no config file can list them
// all. Without a per-provider default every one of those calls priced at zero
// and a user could spend the gateway's credential all day without their
// balance moving — which is the bug this wildcard exists to close.
func TestAProviderDefaultPricesTheModelsNobodyListed(t *testing.T) {
	tr := New(map[string]Price{
		"9router/*":              {Input: 1, Output: 2, PerMinute: 3},
		"9router/gemini-2.5-pro": {Input: 10, Output: 20},
		"9router/gpt-live":       {PerMinute: 6},
	})

	u := Tokens(provider.Usage{PromptTokens: 1_000_000, CompletionTokens: 1_000_000})

	// An unlisted model falls back to the provider's default instead of free.
	if got := tr.Cost("9router", "some-model-shipped-last-tuesday", u); math.Abs(got-3) > 1e-9 {
		t.Fatalf("unlisted model cost = %v, want 3", got)
	}
	// An exact entry still wins: the default is a floor for the long tail, not
	// a replacement for a price anyone bothered to write down.
	if got := tr.Cost("9router", "gemini-2.5-pro", u); math.Abs(got-30) > 1e-9 {
		t.Fatalf("listed model cost = %v, want 30", got)
	}
	// A provider with no default keeps behaving exactly as it did before the
	// wildcard existed, so adding this cannot start billing for anything.
	if got := tr.Cost("openai", "gpt-4o", u); got != 0 {
		t.Fatalf("unpriced provider cost = %v, want 0", got)
	}
	// Per-minute pricing reads the same table, so a live model is covered too.
	if got := tr.Cost("9router", "unlisted-live", Metered{Seconds: 60}); math.Abs(got-3) > 1e-9 {
		t.Fatalf("wildcard per-minute cost = %v, want 3", got)
	}
	if got := tr.Cost("9router", "gpt-live", Metered{Seconds: 60}); math.Abs(got-6) > 1e-9 {
		t.Fatalf("listed per-minute cost = %v, want 6", got)
	}
}

// The same model is sold by five resellers under five spellings. Its list price
// is written once, under its own name, and reaches every one of them — while a
// price written for one provider still wins for that provider.
func TestAModelsListPriceReachesEveryoneServingIt(t *testing.T) {
	prices := map[string]Price{
		"gpt-4o-mini":        {Input: 0.15, Output: 0.6},
		"avalai/gpt-4o-mini": {Input: 0.2, Output: 0.8},
		"parspack/*":         {Input: 9, Output: 9},
	}
	for _, c := range []struct {
		provider, model string
		want            Price
	}{
		{"gapgpt", "gpt-4o-mini", prices["gpt-4o-mini"]},
		// The vendor prefix and the case are not part of the model.
		{"parspack", "openai/gpt-4o-mini", prices["gpt-4o-mini"]},
		{"9router", "openrouter/openai/GPT-4o-mini", prices["gpt-4o-mini"]},
		// A provider's own price is more specific than the list price.
		{"avalai", "gpt-4o-mini", prices["avalai/gpt-4o-mini"]},
		// The list price is more specific than a provider's catch-all.
		{"parspack", "openai/gpt-5.5", prices["parspack/*"]},
	} {
		got, ok := Lookup(prices, c.provider, c.model)
		if !ok || got != c.want {
			t.Errorf("Lookup(%s, %s) = %+v, %v; want %+v", c.provider, c.model, got, ok, c.want)
		}
	}
}

// A ":free" model costs nothing even behind a provider whose catch-all would
// otherwise bill it — that suffix is the router's own word that it is free.
func TestAFreeModelIsFree(t *testing.T) {
	prices := map[string]Price{"openrouter/*": {Input: 1, Output: 1}}
	got, ok := Lookup(prices, "openrouter", "nvidia/nemotron-3-super-120b-a12b:free")
	if !ok || got != (Price{}) {
		t.Fatalf("free model price = %+v, %v; want zero and priced", got, ok)
	}
}

// Each unit is billed only for the work that was done in it: a minute of audio,
// an image, a million characters spoken.
func TestEveryUnitIsBilledAtItsOwnRate(t *testing.T) {
	p := Price{Input: 2, Output: 8, PerMinute: 0.006, PerImage: 0.04, PerMillionChars: 15, PerCredit: 0.004}
	for _, c := range []struct {
		name string
		m    Metered
		want float64
	}{
		{"tokens", Tokens(provider.Usage{PromptTokens: 500_000, CompletionTokens: 250_000}), 1 + 2},
		{"ninety seconds", Metered{Seconds: 90}, 0.009},
		{"two images", Metered{Images: 2}, 0.08},
		{"two thousand characters", Metered{Characters: 2000}, 0.03},
		{"fifty credits", Tokens(provider.Usage{Credits: 50}), 0.2},
	} {
		if got := p.Of(c.m); math.Abs(got-c.want) > 1e-12 {
			t.Errorf("%s: cost = %v, want %v", c.name, got, c.want)
		}
	}
}

// RecordEntry books one call's whole metered event — both costs, the honesty
// flags and the physical units — into every snapshot the tracker keeps. The
// flags are what keeps a metered-at-nothing call from reading as a free
// month: the tokens are real, the vendor's charge is real, and each gap has
// to stay visible in its own counter.
func TestRecordEntryCarriesTheNewFields(t *testing.T) {
	tr := New(map[string]Price{"anthropic/claude": {Input: 3, Output: 15}})

	tr.RecordEntry("desk", "anthropic", "claude", Entry{
		Requests:         1,
		PromptTokens:     1_000_000,
		CompletionTokens: 100_000,
		CostUSD:          3,
		ProviderCostUSD:  4.5,
		Unpriced:         true,
		Estimated:        true,
		Failed:           true,
		AudioSeconds:     90.5,
		Images:           2,
		Characters:       2000,
	})

	byProject, byModel := tr.Snapshot()
	for name, got := range map[string]Stat{"desk": byProject["desk"], "anthropic/claude": byModel["anthropic/claude"]} {
		if got.UnpricedRequests != 1 || got.EstimatedRequests != 1 || got.FailedRequests != 1 {
			t.Errorf("%s honesty counters = %d/%d/%d, want 1/1/1", name,
				got.UnpricedRequests, got.EstimatedRequests, got.FailedRequests)
		}
		if math.Abs(got.ProviderCostUSD-4.5) > 1e-9 || math.Abs(got.CostUSD-3) > 1e-9 {
			t.Errorf("%s costs = %v charged / %v vendor, want 3 / 4.5", name, got.CostUSD, got.ProviderCostUSD)
		}
		if got.AudioSeconds != 90.5 || got.Images != 2 || got.Characters != 2000 {
			t.Errorf("%s units = %vs/%d imgs/%d chars, want 90.5/2/2000", name, got.AudioSeconds, got.Images, got.Characters)
		}
	}

	// Record and RecordAt keep their old behaviour through the same path: a
	// plain record carries none of the flags and computes its own cost.
	tr.Record("desk", "anthropic", "claude", provider.Usage{PromptTokens: 1_000_000})
	tr.RecordAt("desk", "anthropic", "claude", 1, provider.Usage{PromptTokens: 1}, 0.25)
	byProject, _ = tr.Snapshot()
	s := byProject["desk"]
	if s.UnpricedRequests != 1 || s.EstimatedRequests != 1 || s.FailedRequests != 1 {
		t.Errorf("flag counters moved through Record/RecordAt: %+v", s)
	}
	if want := 3 + 3 + 0.25; math.Abs(s.CostUSD-want) > 1e-9 {
		t.Errorf("cost = %v, want %v", s.CostUSD, want)
	}
	if got := tr.Cost("openai", "unlisted", Tokens(provider.Usage{PromptTokens: 1})); got != 0 {
		t.Errorf("unpriced cost = %v, want 0", got)
	}
	if _, priced := tr.PriceOf("openai", "unlisted", Tokens(provider.Usage{PromptTokens: 1})); priced {
		t.Error("PriceOf reports an unlisted model as priced")
	}
	if got, priced := tr.PriceOf("anthropic", "claude", Tokens(provider.Usage{PromptTokens: 1_000_000})); !priced || math.Abs(got-3) > 1e-9 {
		t.Errorf("PriceOf = %v, %v; want 3, true", got, priced)
	}
}

// Anthropic's cache tokens are billed at rates of their own on the real
// invoice. Writing the rates prices them; leaving them out prices them at
// zero, which is exactly what a deployment that has not opted in paid
// yesterday — so no invoice moves without an operator deciding it should.
func TestCacheTokenRatesAreOptIn(t *testing.T) {
	m := Tokens(provider.Usage{
		PromptTokens:       1_000_000,
		CompletionTokens:   100_000,
		CacheReadInput:     2_000_000,
		CacheCreationInput: 500_000,
	})

	without := Price{Input: 3, Output: 15}
	if got := without.Of(m); math.Abs(got-4.5) > 1e-9 {
		t.Errorf("cache tokens priced without rates = %v, want 4.5 (the plain input/output charge)", got)
	}

	with := Price{Input: 3, Output: 15, InputCacheRead: 0.3, InputCacheWrite: 3.75}
	want := 3 + 1.5 + 0.6 + 1.875
	if got := with.Of(m); math.Abs(got-want) > 1e-9 {
		t.Errorf("cache tokens priced with rates = %v, want %v", got, want)
	}
}
