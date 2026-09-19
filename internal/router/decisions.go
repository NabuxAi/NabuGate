package router

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"nabugate/internal/config"
	"nabugate/internal/provider"
)

// SetDecisions installs the structured decisions routes (System One).
func (r *Router) SetDecisions(decisions map[string]config.ModelRoute) {
	r.decisions = decisions
}

// DecisionResult is the outcome of a structured decision evaluation.
type DecisionResult struct {
	Alias    string
	Provider string
	Model    string
	Answers  json.RawMessage
	Usage    provider.Usage
}

// Decide resolves a decision alias (or passthrough) and evaluates primary then fallbacks.
func (r *Router) Decide(ctx context.Context, alias string, req provider.DecisionRequest) (DecisionResult, error) {
	var targets []config.Target
	if route, ok := r.decisions[alias]; ok {
		targets = append([]config.Target{route.Primary}, route.Fallback...)
	} else if t, ok := r.resolvePassthrough(alias); ok {
		targets = []config.Target{t}
	} else {
		return DecisionResult{}, fmt.Errorf("unknown decision alias %q", alias)
	}

	var failures targetErrors
	for i, t := range r.attempts(ctx, targets) {
		adapter, ok := t.adapter, t.adapter != nil
		if !ok {
			failures.add(t.label, t.Model, t.unavailable())
			continue
		}
		decAdapter, ok := adapter.(provider.DecisionAdapter)
		if !ok {
			failures.add(t.label, t.Model, fmt.Errorf("provider does not support decisions"))
			r.log.Warn("skip decision target", "alias", alias, "provider", t.Provider, "reason", "no decisions support")
			continue
		}

		if !providerAllowed(ctx, t.Provider) {
			failures.add(t.label, t.Model, fmt.Errorf("provider not allowed by token policy"))
			continue
		}

		req.Model = t.Model
		start := time.Now()
		resp, err := decAdapter.Decide(ctx, req)
		attrs := []any{
			"capability", "decision",
			"alias", alias,
			"provider", t.Provider,
			"model", t.Model,
			"attempt", i + 1,
			"latency_ms", time.Since(start).Milliseconds(),
		}
		if err != nil {
			failures.add(t.label, t.Model, err)
			r.log.Warn("upstream failed", append(attrs, "error", err.Error())...)
			continue
		}

		recordKeySource(ctx, t.caller)
		r.log.Info("upstream ok", append(attrs, "total_tokens", resp.Usage.TotalTokens)...)
		return DecisionResult{
			Alias:    alias,
			Provider: t.Provider,
			Model:    t.Model,
			Answers:  resp.Answers,
			Usage:    resp.Usage,
		}, nil
	}

	return DecisionResult{}, failures.err("decision", alias)
}
