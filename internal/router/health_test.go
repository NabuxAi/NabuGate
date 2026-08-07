package router

import (
	"testing"

	"nabugate/internal/config"
	"nabugate/internal/provider"
)

// healthRouter builds a router whose only keyed provider is "live". "dead" is
// named by config but never reached an adapter, which is exactly what happens
// when a provider's API key env is empty in one deployment and set in another.
func healthRouter(models map[string]config.ModelRoute, embeddings map[string]config.ModelRoute) *Router {
	adapters := map[string]provider.Adapter{
		"live":  provider.NewOpenAIAdapter("live", "https://live.test/v1", "k", nil),
		"other": provider.NewOpenAIAdapter("other", "https://other.test/v1", "k", nil),
	}

	return New(adapters, models, nil, nil, embeddings, nil, nil, discardLogger())
}

func findAlias(t *testing.T, health []AliasHealth, id string) AliasHealth {
	t.Helper()
	for _, a := range health {
		if a.ID == id {
			return a
		}
	}
	t.Fatalf("alias %q missing from health report", id)
	return AliasHealth{}
}

func TestAliasWithNoKeyedProviderIsReportedUnusable(t *testing.T) {
	r := healthRouter(map[string]config.ModelRoute{
		"orphan": {Primary: config.Target{Provider: "dead", Model: "m"}},
	}, nil)

	got := findAlias(t, r.AliasHealthAll(), "orphan")

	// The whole class of fault this endpoint exists for: permitted, listed,
	// and fails every single request with an error written for us, not for
	// the caller.
	if got.Healthy() {
		t.Fatalf("an alias whose only provider has no key must not read as healthy: %+v", got)
	}
	if len(got.Warnings) == 0 {
		t.Fatal("an unusable alias must say why")
	}
}

func TestAFallbackThatDoesNotCrossVendorsIsFlagged(t *testing.T) {
	r := healthRouter(map[string]config.ModelRoute{
		"same-vendor": {
			Primary:  config.Target{Provider: "live", Model: "a"},
			Fallback: []config.Target{{Provider: "live", Model: "b"}},
		},
	}, nil)

	got := findAlias(t, r.AliasHealthAll(), "same-vendor")

	// Two models behind one key share one quota and one outage — one target,
	// not two, and the chain fails twice at once.
	if len(got.Warnings) != 1 {
		t.Fatalf("want exactly the cross-vendor warning, got %v", got.Warnings)
	}
	if got.Live != 2 || len(got.Providers) != 1 {
		t.Fatalf("want 2 live targets on 1 provider, got %d on %v", got.Live, got.Providers)
	}
}

func TestACrossVendorChainIsClean(t *testing.T) {
	r := healthRouter(map[string]config.ModelRoute{
		"good": {
			Primary:  config.Target{Provider: "live", Model: "a"},
			Fallback: []config.Target{{Provider: "other", Model: "b"}},
		},
	}, nil)

	got := findAlias(t, r.AliasHealthAll(), "good")

	if len(got.Warnings) != 0 {
		t.Fatalf("a two-vendor chain is the shape we want; it must not warn: %v", got.Warnings)
	}
}

func TestAChainWornDownToOneRungSaysSo(t *testing.T) {
	r := healthRouter(map[string]config.ModelRoute{
		"worn": {
			Primary:  config.Target{Provider: "live", Model: "a"},
			Fallback: []config.Target{{Provider: "dead", Model: "b"}},
		},
	}, nil)

	got := findAlias(t, r.AliasHealthAll(), "worn")

	if got.Live != 1 || got.Configured != 2 {
		t.Fatalf("want 1 live of 2 configured, got %d of %d", got.Live, got.Configured)
	}
	if len(got.Warnings) == 0 {
		t.Fatal("a chain shorter than its author intended must be visible before it is needed")
	}
}

func TestASingleRungEmbeddingAliasIsNotScolded(t *testing.T) {
	r := healthRouter(nil, map[string]config.ModelRoute{
		"desk-embed": {Primary: config.Target{Provider: "live", Model: "e"}},
	})

	got := findAlias(t, r.AliasHealthAll(), "desk-embed")

	// No fallback is the *correct* configuration for a stored index: crossing
	// vector widths mid-flight corrupts the corpus with no error raised.
	// Warning about it would train people to ignore these warnings.
	if len(got.Warnings) != 0 {
		t.Fatalf("a deliberate single-rung embedding alias must not warn: %v", got.Warnings)
	}
}

func TestARegistryEntryCountsOnlyProvidersWithKeys(t *testing.T) {
	r := healthRouter(map[string]config.ModelRoute{
		"logical": {Primary: config.Target{Model: "gpt-5.5"}},
	}, nil)

	r.SetRegistry(map[string]config.ModelEntry{
		"gpt-5.5": {Serves: []config.Serving{
			{Provider: "live", Model: "openai/gpt-5.5"},
			{Provider: "other", Model: "gpt-5.5"},
			{Provider: "dead", Model: "gpt-5.5"},
		}},
	})

	got := findAlias(t, r.AliasHealthAll(), "logical")

	// One configured rung fans out to the providers that can serve it, and
	// counting the unkeyed one would promise a retry that never happens.
	if got.Live != 2 {
		t.Fatalf("want 2 live targets from the registry, got %d (%v)", got.Live, got.Providers)
	}
	if len(got.Warnings) != 0 {
		t.Fatalf("two vendors behind one rung is a healthy chain: %v", got.Warnings)
	}
}
