package router

import (
	"context"
	"fmt"
	"time"

	"nabugate/internal/config"
	"nabugate/internal/provider"
)

// SetLive installs the realtime voice aliases. A separate setter rather than
// a New parameter so every existing caller of New — the tests included —
// keeps compiling; a deployment with no live aliases behaves as before.
func (r *Router) SetLive(live map[string]config.ModelRoute) {
	r.live = live
}

// LiveResult is a signalled realtime session: which upstream took it and what
// it answered.
type LiveResult struct {
	Alias    string
	Provider string
	Model    string
	ID       string
	Body     []byte
}

// LiveSession resolves a live alias and asks the first available target to
// create the session. The fallback chain works as for speech: a rung whose
// provider is down, keyless or not permitted for this key is skipped, and
// the caller learns why every rung failed if none took it.
func (r *Router) LiveSession(ctx context.Context, alias string, body []byte) (LiveResult, error) {
	route, ok := r.live[alias]
	if !ok {
		return LiveResult{}, fmt.Errorf("unknown live alias %q", alias)
	}
	targets := append([]config.Target{route.Primary}, route.Fallback...)
	var failures targetErrors

	for i, t := range r.attempts(ctx, targets) {
		adapter, ok := t.adapter, t.adapter != nil
		if !ok {
			failures.add(t.label, t.Model, t.unavailable())
			continue
		}
		liveAdapter, ok := adapter.(provider.LiveAdapter)
		if !ok {
			failures.add(t.label, t.Model, fmt.Errorf("provider does not support live sessions"))
			r.log.Warn("skip live target", "alias", alias, "provider", t.Provider, "reason", "no live support")
			continue
		}
		if !providerAllowed(ctx, t.Provider) {
			failures.add(t.label, t.Model, fmt.Errorf("provider not allowed by token policy"))
			continue
		}
		start := time.Now()
		resp, err := liveAdapter.CreateLiveSession(ctx, provider.LiveSessionRequest{Model: t.Model, Body: body})
		attrs := []any{"capability", "live", "alias", alias, "provider", t.Provider, "model", t.Model, "attempt", i + 1, "latency_ms", time.Since(start).Milliseconds()}
		if err != nil {
			failures.add(t.label, t.Model, err)
			r.log.Warn("upstream failed", append(attrs, "error", err.Error())...)
			continue
		}
		recordKeySource(ctx, t.caller)
		r.log.Info("upstream ok", append(attrs, "session", resp.ID)...)
		return LiveResult{Alias: alias, Provider: t.Provider, Model: t.Model, ID: resp.ID, Body: resp.Body}, nil
	}
	return LiveResult{}, failures.err("live", alias)
}
