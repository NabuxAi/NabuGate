package router

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"nabugate/internal/config"
	"nabugate/internal/provider"
)

// fakeAdapter is a minimal provider.Adapter for routing tests.
type fakeAdapter struct {
	name string
	err  error
	resp provider.ChatResponse
}

func (f fakeAdapter) Name() string { return f.name }
func (f fakeAdapter) Chat(_ context.Context, _ provider.ChatRequest) (provider.ChatResponse, error) {
	if f.err != nil {
		return provider.ChatResponse{}, f.err
	}
	return f.resp, nil
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// TestRouterFallback verifies the router skips a failing primary and serves the
// fallback target.
func TestRouterFallback(t *testing.T) {
	adapters := map[string]provider.Adapter{
		"primary": fakeAdapter{name: "primary", err: errors.New("down")},
		"backup":  fakeAdapter{name: "backup", resp: provider.ChatResponse{Content: "hi"}},
	}
	models := map[string]config.ModelRoute{
		"nabu-fast": {
			Primary:  config.Target{Provider: "primary", Model: "m1"},
			Fallback: []config.Target{{Provider: "backup", Model: "m2"}},
		},
	}
	r := New(adapters, models, nil, nil, nil, nil, discardLogger())

	res, err := r.Chat(context.Background(), "nabu-fast", provider.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Provider != "backup" || res.Response.Content != "hi" {
		t.Fatalf("expected backup/hi, got %s/%q", res.Provider, res.Response.Content)
	}
}

// TestRouterUnknownAlias verifies an unknown alias is an error (mapped to 400 by
// the server).
func TestRouterUnknownAlias(t *testing.T) {
	r := New(map[string]provider.Adapter{}, map[string]config.ModelRoute{}, nil, nil, nil, nil, discardLogger())
	if _, err := r.Chat(context.Background(), "nope", provider.ChatRequest{}); err == nil {
		t.Fatal("expected error for unknown alias")
	}
}

// TestRouterAllTargetsFail verifies an error when every target fails.
func TestRouterAllTargetsFail(t *testing.T) {
	adapters := map[string]provider.Adapter{
		"a": fakeAdapter{name: "a", err: errors.New("x")},
	}
	models := map[string]config.ModelRoute{
		"nabu-fast": {Primary: config.Target{Provider: "a", Model: "m"}},
	}
	r := New(adapters, models, nil, nil, nil, nil, discardLogger())
	if _, err := r.Chat(context.Background(), "nabu-fast", provider.ChatRequest{}); err == nil {
		t.Fatal("expected error when all targets fail")
	}
}

// listerAdapter implements Adapter + ModelLister, counting discovery calls.
type listerAdapter struct {
	name   string
	models []string
	err    error
	calls  int
}

func (l *listerAdapter) Name() string { return l.name }
func (l *listerAdapter) Chat(_ context.Context, _ provider.ChatRequest) (provider.ChatResponse, error) {
	return provider.ChatResponse{Content: "ok"}, nil
}
func (l *listerAdapter) ListModels(_ context.Context) ([]string, error) {
	l.calls++
	return l.models, l.err
}

// TestRouterPassthroughRouting verifies that "<provider>/<model>" with a nested
// (multi-slash) upstream ID routes directly to a passthrough provider, with the
// first "/" splitting provider from the intact upstream model name.
func TestRouterPassthroughRouting(t *testing.T) {
	adapters := map[string]provider.Adapter{
		"parspack": fakeAdapter{name: "parspack", resp: provider.ChatResponse{Content: "hi"}},
	}
	r := New(adapters, map[string]config.ModelRoute{}, nil, nil, nil,
		map[string][]string{"parspack": nil}, discardLogger())

	res, err := r.Chat(context.Background(), "parspack/openai/gpt-5.5", provider.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Provider != "parspack" || res.Model != "openai/gpt-5.5" {
		t.Fatalf("routed to %s/%s, want parspack/openai/gpt-5.5", res.Provider, res.Model)
	}
}

// TestRouterPassthroughDisabled verifies a "<provider>/<model>" name is an
// unknown alias when the provider is not passthrough-enabled.
func TestRouterPassthroughDisabled(t *testing.T) {
	adapters := map[string]provider.Adapter{
		"parspack": fakeAdapter{name: "parspack"},
	}
	r := New(adapters, map[string]config.ModelRoute{}, nil, nil, nil, nil, discardLogger())
	if _, err := r.Chat(context.Background(), "parspack/openai/gpt-5.5", provider.ChatRequest{}); err == nil {
		t.Fatal("expected unknown-alias error when passthrough is disabled")
	}
}

// TestRouterCatalogModels verifies discovered + static models are surfaced as
// "<provider>/<id>", deduped, and cached across calls.
func TestRouterCatalogModels(t *testing.T) {
	l := &listerAdapter{name: "parspack", models: []string{"openai/gpt-5.5", "google/gemini-2.5-flash"}}
	adapters := map[string]provider.Adapter{"parspack": l}
	r := New(adapters, map[string]config.ModelRoute{}, nil, nil, nil,
		map[string][]string{"parspack": {"curated/model-x"}}, discardLogger())

	got := r.CatalogModels(context.Background())
	ids := make(map[string]string, len(got))
	for _, a := range got {
		ids[a.ID] = a.Owner
	}
	for _, want := range []string{"parspack/curated/model-x", "parspack/openai/gpt-5.5", "parspack/google/gemini-2.5-flash"} {
		if ids[want] != "parspack" {
			t.Errorf("missing catalog model %q (got %v)", want, ids)
		}
	}
	// Second call must hit the cache, not re-query the provider.
	_ = r.CatalogModels(context.Background())
	if l.calls != 1 {
		t.Fatalf("discovery called %d times, want 1 (cached)", l.calls)
	}
}

// TestRouterCatalogModelsDiscoveryError verifies a discovery failure degrades to
// the static list instead of erroring.
func TestRouterCatalogModelsDiscoveryError(t *testing.T) {
	l := &listerAdapter{name: "parspack", err: errors.New("upstream down")}
	adapters := map[string]provider.Adapter{"parspack": l}
	r := New(adapters, map[string]config.ModelRoute{}, nil, nil, nil,
		map[string][]string{"parspack": {"curated/model-x"}}, discardLogger())

	got := r.CatalogModels(context.Background())
	if len(got) != 1 || got[0].ID != "parspack/curated/model-x" {
		t.Fatalf("expected static-only fallback, got %v", got)
	}
}
