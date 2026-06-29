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
	r := New(adapters, models, nil, nil, nil, discardLogger())

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
	r := New(map[string]provider.Adapter{}, map[string]config.ModelRoute{}, nil, nil, nil, discardLogger())
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
	r := New(adapters, models, nil, nil, nil, discardLogger())
	if _, err := r.Chat(context.Background(), "nabu-fast", provider.ChatRequest{}); err == nil {
		t.Fatal("expected error when all targets fail")
	}
}
