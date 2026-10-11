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
	Engine   string
	Answers  json.RawMessage
	Usage    provider.Usage
}

// localDecisionAdapter adapts a LocalDecisionAdapter to DecisionAdapter so the
// fallback loop below stays engine-agnostic.
type localDecisionAdapter struct{ inner provider.LocalDecisionAdapter }

func (l localDecisionAdapter) Decide(ctx context.Context, req provider.DecisionRequest) (provider.DecisionResponse, error) {
	return l.inner.DecideLocal(ctx, req)
}

// decisionTargets resolves what a decision alias names. A dedicated decisions
// alias comes first; after that any chat alias may be driven as a decision
// model by the local engine (its rungs, in their configured order), then a
// "provider/model" passthrough name.
func (r *Router) decisionTargets(alias string) ([]config.Target, string, bool) {
	if route, ok := r.decisions[alias]; ok {
		return r.rungs(alias, route), "decision", true
	}
	if route, ok := r.models[alias]; ok {
		return r.rungs(alias, route), "chat", true
	}
	if t, ok := r.resolvePassthrough(alias); ok {
		return []config.Target{t}, "passthrough", true
	}
	return nil, "", false
}

// decisionCandidate is one way a target can serve decisions.
type decisionCandidate struct {
	label string
	adapt provider.DecisionAdapter
}

// decisionCandidates maps an adapter to the ways it can serve a decision, in
// the order they should be tried for the requested engine. Auto prefers a
// native decisions API (TypeSafe, OpenRouter /alpha/decisions for Jev) and
// falls back to the local logprob engine on the same rung — an OpenAI-wire
// provider whose /alpha/decisions 404s still answers through its chat
// endpoint. "local"/"native" pin one way and refuse the other.
func decisionCandidates(adapter provider.Adapter, engine string) []decisionCandidate {
	local, hasLocal := adapter.(provider.LocalDecisionAdapter)
	native, hasNative := adapter.(provider.DecisionAdapter)
	switch engine {
	case "local":
		if hasLocal {
			return []decisionCandidate{{"local", localDecisionAdapter{local}}}
		}
		return nil
	case "native":
		if hasNative {
			return []decisionCandidate{{"native", native}}
		}
		return nil
	default: // "" or "auto"
		var out []decisionCandidate
		if hasNative {
			out = append(out, decisionCandidate{"native", native})
		}
		if hasLocal {
			out = append(out, decisionCandidate{"local", localDecisionAdapter{local}})
		}
		return out
	}
}

// Decide resolves a decision alias (decisions alias, chat alias or passthrough)
// and evaluates primary then fallbacks.
func (r *Router) Decide(ctx context.Context, alias string, req provider.DecisionRequest) (DecisionResult, error) {
	targets, kind, ok := r.decisionTargets(alias)
	if !ok {
		return DecisionResult{}, fmt.Errorf("unknown decision alias %q", alias)
	}

	var failures targetErrors
	for i, t := range r.attempts(ctx, targets) {
		if t.adapter == nil {
			failures.add(t.label, t.Model, t.unavailable())
			continue
		}
		candidates := decisionCandidates(t.adapter, req.Engine)
		if len(candidates) == 0 {
			failures.add(t.label, t.Model, fmt.Errorf("provider does not support decisions"))
			r.log.Warn("skip decision target", "alias", alias, "provider", t.Provider, "reason", "no decisions support")
			continue
		}

		if !providerAllowed(ctx, t.Provider) {
			failures.add(t.label, t.Model, fmt.Errorf("provider not allowed by token policy"))
			continue
		}

		req.Model = t.Model
		for _, cand := range candidates {
			start := time.Now()
			resp, err := cand.adapt.Decide(ctx, req)
			attrs := []any{
				"capability", "decision",
				"alias", alias,
				"provider", t.Provider,
				"model", t.Model,
				"engine", cand.label,
				"alias_kind", kind,
				"attempt", i + 1,
				"latency_ms", time.Since(start).Milliseconds(),
			}
			if err != nil {
				r.log.Warn("upstream failed", append(attrs, "error", err.Error())...)
				// The last candidate's error is the one the rung is judged by.
				if cand.label == candidates[len(candidates)-1].label {
					failures.add(t.label, t.Model, err)
				}
				continue
			}

			recordKeySource(ctx, t.caller)
			r.log.Info("upstream ok", append(attrs, "total_tokens", resp.Usage.TotalTokens)...)
			return DecisionResult{
				Alias:    alias,
				Provider: t.Provider,
				Model:    t.Model,
				Engine:   cand.label,
				Answers:  resp.Answers,
				Usage:    resp.Usage,
			}, nil
		}
	}

	return DecisionResult{}, failures.err("decision", alias)
}
