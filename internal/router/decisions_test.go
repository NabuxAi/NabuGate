package router

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	"nabugate/internal/config"
	"nabugate/internal/provider"
)

type mockDecisionAdapter struct {
	name   string
	fail   bool
	answer string
}

func (m *mockDecisionAdapter) Name() string { return m.name }
func (m *mockDecisionAdapter) Chat(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
	return provider.ChatResponse{}, errors.New("chat not supported")
}
func (m *mockDecisionAdapter) Decide(ctx context.Context, req provider.DecisionRequest) (provider.DecisionResponse, error) {
	if m.fail {
		return provider.DecisionResponse{}, errors.New("upstream failure")
	}
	return provider.DecisionResponse{
		Model:   req.Model,
		Answers: json.RawMessage(m.answer),
		Usage: provider.Usage{
			PromptTokens: 100,
			TotalTokens:  100,
		},
	}, nil
}

func TestRouterDecideFallback(t *testing.T) {
	adapters := map[string]provider.Adapter{
		"primary-fail": &mockDecisionAdapter{name: "primary-fail", fail: true},
		"fallback-ok":  &mockDecisionAdapter{name: "fallback-ok", fail: false, answer: `{"res": 1}`},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := New(adapters, nil, nil, nil, nil, nil, nil, logger)
	r.SetDecisions(map[string]config.ModelRoute{
		"nabu-decision": {
			Primary: config.Target{Provider: "primary-fail", Model: "jev-latest"},
			Fallback: []config.Target{
				{Provider: "fallback-ok", Model: "typesafe/jev-latest"},
			},
		},
	})

	res, err := r.Decide(context.Background(), "nabu-decision", provider.DecisionRequest{
		Model:     "nabu-decision",
		State:     json.RawMessage(`"test"`),
		Questions: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Provider != "fallback-ok" {
		t.Errorf("expected provider fallback-ok, got %s", res.Provider)
	}
	if res.Model != "typesafe/jev-latest" {
		t.Errorf("expected model typesafe/jev-latest, got %s", res.Model)
	}
	if string(res.Answers) != `{"res": 1}` {
		t.Errorf("expected answers {\"res\": 1}, got %s", string(res.Answers))
	}
}

func TestRouterDecideUnknownAlias(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := New(map[string]provider.Adapter{}, nil, nil, nil, nil, nil, nil, logger)
	_, err := r.Decide(context.Background(), "unknown-decision", provider.DecisionRequest{})
	if err == nil {
		t.Fatal("expected error for unknown alias, got nil")
	}
}
