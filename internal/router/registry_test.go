package router

import (
	"context"
	"errors"
	"testing"

	"nabugate/internal/config"
	"nabugate/internal/provider"
)

// A model is an identity, not a provider coordinate. "gpt-5.5" is the same
// model whoever serves it, so when one provider fails the gateway must reach
// the next one serving that same model without the caller being involved.

type namedAdapter struct {
	name string
	err  error
	said string
	hits *int
}

func (a namedAdapter) Name() string { return a.name }

func (a namedAdapter) Chat(context.Context, provider.ChatRequest) (provider.ChatResponse, error) {
	*a.hits++
	if a.err != nil {
		return provider.ChatResponse{}, a.err
	}
	return provider.ChatResponse{Content: a.said}, nil
}

func registryRouter(t *testing.T, adapters map[string]provider.Adapter, models map[string]config.ModelRoute) *Router {
	t.Helper()
	// Passthrough must be enabled for the pinned-coordinate cases.
	r := New(adapters, models, nil, nil, nil,
		map[string][]string{"parspack": nil, "avalai": nil, "gapgpt": nil},
		discardLogger())
	r.SetRegistry(map[string]config.ModelEntry{
		"gpt-5.5": {
			ParamStyle: provider.ParamStyleReasoning,
			Serves: []config.Serving{
				{Provider: "parspack", Model: "openai/gpt-5.5"},
				{Provider: "avalai", Model: "gpt-5.5"},
				{Provider: "gapgpt", Model: "gpt-5.5"},
			},
		},
	})
	return r
}

func TestRegistrySwitchesProviderOnFailure(t *testing.T) {
	first, second := 0, 0

	r := registryRouter(t, map[string]provider.Adapter{
		"parspack": namedAdapter{name: "parspack", err: errors.New("502"), hits: &first},
		"avalai":   namedAdapter{name: "avalai", said: "hi", hits: &second},
	}, map[string]config.ModelRoute{
		// The alias names the model only — no provider coordinate anywhere.
		"nabu-smart": {Primary: config.Target{Model: "gpt-5.5"}},
	})

	res, err := r.Chat(context.Background(), "nabu-smart", provider.ChatRequest{})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if res.Provider != "avalai" || res.Response.Content != "hi" {
		t.Errorf("served by %q (%q); want avalai to take over silently", res.Provider, res.Response.Content)
	}
	if first != 1 || second != 1 {
		t.Errorf("attempts: parspack=%d avalai=%d, want 1 and 1", first, second)
	}
}

func TestRegistrySkipsProvidersWithNoAdapter(t *testing.T) {
	hits := 0
	// Only gapgpt is keyed on this gateway; the two ahead of it in the registry
	// must cost nothing rather than be attempted.
	r := registryRouter(t, map[string]provider.Adapter{
		"gapgpt": namedAdapter{name: "gapgpt", said: "hi", hits: &hits},
	}, map[string]config.ModelRoute{
		"nabu-smart": {Primary: config.Target{Model: "gpt-5.5"}},
	})

	targets, ok := r.resolveChatTargets("nabu-smart")
	if !ok {
		t.Fatal("alias did not resolve")
	}
	if len(targets) != 1 || targets[0].Provider != "gapgpt" {
		t.Fatalf("targets = %+v, want only gapgpt", targets)
	}
}

func TestRegistryCarriesParamStyle(t *testing.T) {
	hits := 0
	r := registryRouter(t, map[string]provider.Adapter{
		"parspack": namedAdapter{name: "parspack", said: "hi", hits: &hits},
	}, map[string]config.ModelRoute{
		"nabu-smart": {Primary: config.Target{Model: "gpt-5.5"}},
	})

	targets, _ := r.resolveChatTargets("nabu-smart")
	// The dialect belongs to the model, so every provider serving it inherits
	// the declaration without repeating it.
	if targets[0].ParamStyle != provider.ParamStyleReasoning {
		t.Errorf("param_style = %q, want %q", targets[0].ParamStyle, provider.ParamStyleReasoning)
	}
}

func TestBareModelNameResolvesThroughRegistry(t *testing.T) {
	hits := 0
	r := registryRouter(t, map[string]provider.Adapter{
		"avalai": namedAdapter{name: "avalai", said: "hi", hits: &hits},
	}, map[string]config.ModelRoute{})

	// A caller may name the model directly rather than an alias.
	res, err := r.Chat(context.Background(), "gpt-5.5", provider.ChatRequest{})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if res.Provider != "avalai" {
		t.Errorf("provider = %q, want avalai", res.Provider)
	}
}

func TestConcreteTargetsAreUnaffected(t *testing.T) {
	hits := 0
	r := registryRouter(t, map[string]provider.Adapter{
		"parspack": namedAdapter{name: "parspack", said: "hi", hits: &hits},
	}, map[string]config.ModelRoute{
		"direct": {Primary: config.Target{Provider: "parspack", Model: "openai/gpt-4o-mini"}},
	})

	targets, ok := r.resolveChatTargets("direct")
	if !ok || len(targets) != 1 {
		t.Fatalf("targets = %+v, want the single configured coordinate", targets)
	}
	if targets[0].Model != "openai/gpt-4o-mini" {
		t.Errorf("model = %q, want it passed through untouched", targets[0].Model)
	}
}

func TestPinnedProviderRetriesTheSameModelElsewhere(t *testing.T) {
	first, second := 0, 0

	r := registryRouter(t, map[string]provider.Adapter{
		"parspack": namedAdapter{name: "parspack", err: errors.New("502"), hits: &first},
		"avalai":   namedAdapter{name: "avalai", said: "hi", hits: &second},
	}, map[string]config.ModelRoute{})

	// The caller pinned a provider, but what they asked for is gpt-5.5. When
	// that provider fails the retry must stay on gpt-5.5 — a different model
	// would not be what was requested.
	res, err := r.Chat(context.Background(), "parspack/openai/gpt-5.5", provider.ChatRequest{})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if res.Provider != "avalai" {
		t.Errorf("provider = %q, want avalai to serve the same model", res.Provider)
	}
	if res.Model != "gpt-5.5" {
		t.Errorf("model = %q, want gpt-5.5 — the model must not change on retry", res.Model)
	}
	if first != 1 || second != 1 {
		t.Errorf("attempts: parspack=%d avalai=%d, want 1 and 1", first, second)
	}
}

func TestUnknownPassthroughModelHasNoSiblings(t *testing.T) {
	hits := 0
	r := registryRouter(t, map[string]provider.Adapter{
		"parspack": namedAdapter{name: "parspack", said: "hi", hits: &hits},
		"avalai":   namedAdapter{name: "avalai", said: "hi", hits: &hits},
	}, map[string]config.ModelRoute{})

	// A model absent from the registry cannot be assumed available elsewhere.
	targets, ok := r.resolveChatTargets("parspack/some/unlisted-model")
	if !ok {
		t.Fatal("passthrough did not resolve")
	}
	if len(targets) != 1 {
		t.Errorf("targets = %+v, want only the pinned coordinate", targets)
	}
}
