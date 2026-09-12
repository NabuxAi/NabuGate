package router

import (
	"context"
	"errors"
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

// SetLiveProblems marks the live aliases the gateway must refuse, with why —
// config.LiveProblems, read once at start-up. A refused alias answers every
// session request with a LiveMisconfiguredError, is left out of AliasInfos,
// and says what is wrong with it in AliasHealthAll.
func (r *Router) SetLiveProblems(problems map[string]string) {
	r.liveProblems = problems
}

// LiveMisconfiguredError is a live alias refused on purpose: serving it would
// work and be wrong, like a call billed at nothing a minute.
type LiveMisconfiguredError struct {
	Alias  string
	Reason string
}

func (e *LiveMisconfiguredError) Error() string {
	return "live alias misconfigured: " + e.Reason
}

// liveChainError is every rung of a live alias failing. Its message is the
// whole chain, as for every other capability; it also carries the last vendor
// refusal, so the server can answer in that refusal's class.
type liveChainError struct {
	msg      string
	upstream *provider.LiveUpstreamError
}

func (e *liveChainError) Error() string { return e.msg }

func (e *liveChainError) Unwrap() error {
	if e.upstream == nil {
		return nil
	}
	return e.upstream
}

// liveStatus is what the gateway last heard from upstream for a live alias.
// Health is otherwise decided from config alone; a live alias is the one whose
// upstream is never contacted until somebody places a call, so the last real
// answer is the only evidence there is.
type liveStatus struct {
	lastOK    time.Time
	lastErr   string
	lastErrAt time.Time
}

func (r *Router) noteLive(alias string, err error) {
	r.liveMu.Lock()
	defer r.liveMu.Unlock()
	if r.liveStatus == nil {
		r.liveStatus = make(map[string]liveStatus)
	}
	st := r.liveStatus[alias]
	if err == nil {
		st.lastOK = r.now()
	} else {
		st.lastErr = err.Error()
		st.lastErrAt = r.now()
	}
	r.liveStatus[alias] = st
}

// liveHealth adds what only a live alias has: a refusal decided at start-up,
// and the last answer from an upstream that health otherwise never contacts.
func (r *Router) liveHealth(alias string, h *AliasHealth) {
	if reason, bad := r.liveProblems[alias]; bad {
		h.Disabled = true
		h.Warnings = append(h.Warnings, "refused: "+reason)
	}
	r.liveMu.Lock()
	st, seen := r.liveStatus[alias]
	r.liveMu.Unlock()
	if !seen {
		return
	}
	if !st.lastOK.IsZero() {
		ok := st.lastOK
		h.LastOKAt = &ok
	}
	if !st.lastErrAt.IsZero() {
		at := st.lastErrAt
		h.LastErrorAt = &at
		h.LastError = st.lastErr
		if st.lastErrAt.After(st.lastOK) {
			h.Warnings = append(h.Warnings, "the last session request failed")
		}
	}
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
	if reason, bad := r.liveProblems[alias]; bad {
		return LiveResult{}, &LiveMisconfiguredError{Alias: alias, Reason: reason}
	}
	targets := append([]config.Target{route.Primary}, route.Fallback...)
	var failures targetErrors
	var lastRefusal *provider.LiveUpstreamError

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
			var refusal *provider.LiveUpstreamError
			if errors.As(err, &refusal) {
				lastRefusal = refusal
			}
			r.log.Warn("upstream failed", append(attrs, "error", err.Error())...)
			continue
		}
		recordKeySource(ctx, t.caller)
		r.noteLive(alias, nil)
		r.log.Info("upstream ok", append(attrs, "session", resp.ID)...)
		return LiveResult{Alias: alias, Provider: t.Provider, Model: t.Model, ID: resp.ID, Body: resp.Body}, nil
	}
	chainErr := &liveChainError{msg: failures.err("live", alias).Error(), upstream: lastRefusal}
	r.noteLive(alias, chainErr)
	return LiveResult{}, chainErr
}
