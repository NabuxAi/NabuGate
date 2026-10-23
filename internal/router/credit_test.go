package router

import (
	"context"
	"errors"
	"testing"
	"time"

	"nabugate/internal/config"
	"nabugate/internal/provider"
)

// countingAdapter counts calls and fails with err while it is set.
type countingAdapter struct {
	name  string
	err   error
	calls int
}

func (c *countingAdapter) Name() string { return c.name }
func (c *countingAdapter) Chat(_ context.Context, _ provider.ChatRequest) (provider.ChatResponse, error) {
	c.calls++
	if c.err != nil {
		return provider.ChatResponse{}, c.err
	}
	return provider.ChatResponse{Content: c.name}, nil
}

// The error OpenAI returned on 2026-10-23, verbatim as the adapter wraps it.
var noCredit = errors.New("openai: upstream error (status 429): You have no credits remaining. Add credits to continue using the API at https://platform.openai.com/settings/organization/billing/.")

func creditRouter(primary, backup *countingAdapter) (*Router, *time.Time) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	r := New(
		map[string]provider.Adapter{primary.name: primary, backup.name: backup},
		map[string]config.ModelRoute{"nabu-fast": {
			Primary:  config.Target{Provider: primary.name, Model: "m"},
			Fallback: []config.Target{{Provider: backup.name, Model: "m"}},
		}},
		nil, nil, nil, nil, nil, discardLogger())
	r.now = func() time.Time { return now }
	return r, &now
}

func chatVia(t *testing.T, r *Router) string {
	t.Helper()
	res, err := r.Chat(context.Background(), "nabu-fast", provider.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	return res.Provider
}

func TestOutOfCreditProviderIsTriedLast(t *testing.T) {
	openai := &countingAdapter{name: "openai", err: noCredit}
	gemini := &countingAdapter{name: "gemini"}
	r, _ := creditRouter(openai, gemini)

	chatVia(t, r) // learns that openai has no credit
	chatVia(t, r)

	if openai.calls != 1 {
		t.Fatalf("openai was asked %d times; once it said it has no credit, it should not be asked first again", openai.calls)
	}
	if got := r.OutOfCredit(); len(got) != 1 || got[0] != "openai" {
		t.Fatalf("OutOfCredit() = %v, want [openai]", got)
	}
}

func TestRateLimitIsNotOutOfCredit(t *testing.T) {
	// DashScope's answer the same day: a per-minute cap, gone in seconds.
	dashscope := &countingAdapter{name: "dashscope", err: errors.New("dashscope: upstream error (status 429): You have exceeded your current request limit.")}
	gemini := &countingAdapter{name: "gemini"}
	r, _ := creditRouter(dashscope, gemini)

	chatVia(t, r)
	chatVia(t, r)

	if dashscope.calls != 2 {
		t.Fatalf("a rate limit demoted the provider (%d calls); it says nothing about the balance", dashscope.calls)
	}
}

func TestOutOfCreditExpiresAfterCooldown(t *testing.T) {
	openai := &countingAdapter{name: "openai", err: noCredit}
	gemini := &countingAdapter{name: "gemini"}
	r, now := creditRouter(openai, gemini)

	chatVia(t, r)
	openai.err = nil // someone topped up
	*now = now.Add(creditCooldown)

	if got := chatVia(t, r); got != "openai" {
		t.Fatalf("after the cooldown the primary should lead again; %s served", got)
	}
}

func TestOutOfCreditStillTriedWhenNothingElseAnswers(t *testing.T) {
	// Demoting must never turn a request that could succeed into a failure.
	openai := &countingAdapter{name: "openai", err: noCredit}
	gemini := &countingAdapter{name: "gemini"}
	r, _ := creditRouter(openai, gemini)

	chatVia(t, r)
	openai.err = nil // topped up within the cooldown
	gemini.err = errors.New("gemini: upstream error (status 503): overloaded")

	if got := chatVia(t, r); got != "openai" {
		t.Fatalf("the demoted provider should still be the last resort; %s served", got)
	}
	if got := r.OutOfCredit(); len(got) != 0 {
		t.Fatalf("serving on the gateway key proves there is credit again; still marked %v", got)
	}
}

func TestCallerKeyOutOfCreditDoesNotDemoteGatewayKey(t *testing.T) {
	// A caller's empty wallet is theirs; it says nothing about the gateway's.
	openai := &countingAdapter{name: "openai"}
	gemini := &countingAdapter{name: "gemini"}
	r, _ := creditRouter(openai, gemini)
	r.SetCallerAdapter(func(name, key string) (provider.Adapter, bool) {
		return &countingAdapter{name: name, err: noCredit}, true
	})

	ctx := context.WithValue(context.Background(), CallerKeysCtxKey{}, CallerKeys{Keys: map[string][]string{"openai": {"sk-caller"}}})
	if _, err := r.Chat(ctx, "nabu-fast", provider.ChatRequest{}); err != nil {
		t.Fatal(err)
	}
	if got := r.OutOfCredit(); len(got) != 0 {
		t.Fatalf("the caller's key ran dry and the gateway's was demoted: %v", got)
	}
}

func TestOutOfCreditSignals(t *testing.T) {
	for _, msg := range []string{
		"openrouter: upstream error (status 402): Insufficient credits",
		"anthropic: upstream error (status 400): Your credit balance is too low to access the Anthropic API",
		"x: upstream error (status 429): You exceeded your current quota, please check your plan and billing details",
	} {
		if !outOfCredit(errors.New(msg)) {
			t.Errorf("not recognised as out of credit: %s", msg)
		}
	}
	for _, msg := range []string{
		"gemini: upstream error (status 503): overloaded",
		"groq: upstream error (status 429): Rate limit reached for requests per minute",
	} {
		if outOfCredit(errors.New(msg)) {
			t.Errorf("wrongly read as out of credit: %s", msg)
		}
	}
}
