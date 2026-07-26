package router

import (
	"context"
	"strings"
	"testing"

	"nabugate/internal/config"
	"nabugate/internal/provider"
)

// emptyStreamer answers a stream cleanly without ever emitting content, which
// is what some upstreams do for a model that works fine non-streaming.
type emptyStreamer struct{ called *int }

func (e emptyStreamer) Name() string { return "flaky" }

func (e emptyStreamer) Chat(context.Context, provider.ChatRequest) (provider.ChatResponse, error) {
	return provider.ChatResponse{}, nil
}

func (e emptyStreamer) ChatStream(_ context.Context, _ provider.ChatRequest, _ provider.DeltaFunc) (provider.Usage, error) {
	*e.called++
	return provider.Usage{}, nil
}

// goodStreamer emits one delta.
type goodStreamer struct{ called *int }

func (g goodStreamer) Name() string { return "solid" }

func (g goodStreamer) Chat(context.Context, provider.ChatRequest) (provider.ChatResponse, error) {
	return provider.ChatResponse{Content: "hi"}, nil
}

func (g goodStreamer) ChatStream(_ context.Context, _ provider.ChatRequest, onDelta provider.DeltaFunc) (provider.Usage, error) {
	*g.called++
	return provider.Usage{}, onDelta("سلام")
}

func TestChatStreamFallsBackOnEmptyStream(t *testing.T) {
	emptyCalls, goodCalls := 0, 0

	r := New(
		map[string]provider.Adapter{
			"flaky": emptyStreamer{called: &emptyCalls},
			"solid": goodStreamer{called: &goodCalls},
		},
		map[string]config.ModelRoute{
			"alias": {
				Primary:  config.Target{Provider: "flaky", Model: "m1"},
				Fallback: []config.Target{{Provider: "solid", Model: "m2"}},
			},
		},
		nil, nil, nil, nil, discardLogger(),
	)

	var got strings.Builder
	res, err := r.ChatStream(context.Background(), "alias", provider.ChatRequest{},
		func(string, string) {},
		func(d string) error { got.WriteString(d); return nil })

	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if got.String() != "سلام" {
		t.Errorf("content = %q, want the second target's output", got.String())
	}
	if res.Provider != "solid" {
		t.Errorf("provider = %q, want solid — an empty stream must not end the chain", res.Provider)
	}
	if emptyCalls != 1 || goodCalls != 1 {
		t.Errorf("calls: flaky=%d solid=%d, want 1 and 1", emptyCalls, goodCalls)
	}
}
