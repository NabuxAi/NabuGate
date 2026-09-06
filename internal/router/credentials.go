package router

import (
	"context"
	"errors"
	"strings"
	"sync"

	"nabugate/internal/config"
	"nabugate/internal/provider"
)

// --- Bring your own key ---
//
// A caller can send its own upstream credentials with a request and have the
// gateway spend those instead of — or before — the ones the deployment holds.
// The point is not only cost: a customer with a paid Gemini account gets that
// account's quota rather than sharing the gateway's, and a customer who is not
// willing to route their audio through someone else's key can say so.
//
// Nothing is stored. The keys live in the request and are gone with it, which
// is deliberate: a persisted vendor credential is a breach surface for someone
// else's account, and it is not needed to make this useful.

// Key modes decide the order the two credential sources are tried in. An
// unrecognised value behaves as ModeOwnFirst when the caller sent keys and as
// ModeGlobal when it did not, so a typo degrades to the sensible default
// instead of failing the request.
const (
	// ModeGlobal ignores the caller's keys entirely and spends the gateway's.
	ModeGlobal = "global"
	// ModeOwn spends only the caller's keys. A provider the caller holds no key
	// for is skipped with a reason saying so, rather than quietly falling back
	// to the gateway's credentials — which is the whole point of asking for
	// this mode.
	ModeOwn = "own"
	// ModeOwnFirst tries the caller's key, then the gateway's. The default when
	// keys are supplied: the caller pays their own vendor bill, and the gateway
	// is the safety net.
	ModeOwnFirst = "own-first"
	// ModeGlobalFirst tries the gateway's key, then the caller's. For a caller
	// whose own key is the backstop for the gateway running out of quota.
	ModeGlobalFirst = "global-first"
)

// CallerKeysCtxKey is the context key for the caller-supplied credentials.
type CallerKeysCtxKey struct{}

// CallerKeys is what a request carries: a key per provider name, and the order
// to try them in.
type CallerKeys struct {
	Keys map[string]string
	Mode string
}

// Normalize resolves the mode against what the caller actually sent.
func (c CallerKeys) Normalize() CallerKeys {
	switch c.Mode {
	case ModeGlobal, ModeOwn, ModeOwnFirst, ModeGlobalFirst:
	default:
		if len(c.Keys) > 0 {
			c.Mode = ModeOwnFirst
		} else {
			c.Mode = ModeGlobal
		}
	}
	return c
}

// KeySourceCtxKey is the context key for the recorder below.
type KeySourceCtxKey struct{}

// KeySource records which credential actually served a request. It is a
// mutable value passed down the context on purpose — the alternative is adding
// a field to every result type and threading it through every handler, for one
// bit of information that only billing reads. httptrace works the same way.
//
// The zero value is unusable; the server installs one per request.
type KeySource struct {
	mu     sync.Mutex
	source string
}

// NewKeySource returns a recorder for one request.
func NewKeySource() *KeySource { return &KeySource{} }

// Set records the credential that served. Only the first success is kept: a
// request is served once, and a later write could only come from a retry the
// caller never saw.
func (k *KeySource) Set(source string) {
	if k == nil {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.source == "" {
		k.source = source
	}
}

// Caller reports whether the caller's own credential served this request, which
// is what decides that the gateway must not bill for it.
func (k *KeySource) Caller() bool {
	if k == nil {
		return false
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.source == "caller"
}

// recordKeySource marks the winning credential on the request's recorder.
func recordKeySource(ctx context.Context, caller bool) {
	rec, _ := ctx.Value(KeySourceCtxKey{}).(*KeySource)
	if rec == nil {
		return
	}
	if caller {
		rec.Set("caller")
		return
	}
	rec.Set("gateway")
}

// CallerAdapterFunc builds an adapter for one provider against a caller-supplied
// key. It returns false for a provider the deployment does not define — a key
// for a provider that does not exist here cannot be honoured.
type CallerAdapterFunc func(providerName, apiKey string) (provider.Adapter, bool)

// attempt is one rung of a fallback chain after credentials are resolved. It
// embeds the Target so every loop that used to range over targets keeps working
// on t.Provider, t.Model and t.ParamStyle unchanged.
type attempt struct {
	config.Target

	adapter provider.Adapter
	// label is what failures and logs name this rung. It carries the key source
	// because "gemini failed" twice in one error, once per credential, is not a
	// diagnosis.
	label  string
	caller bool
	reason error // why adapter is nil, when the default message would mislead
}

func (a attempt) unavailable() error {
	if a.reason != nil {
		return a.reason
	}
	return errors.New("provider not available (is its API key set?)")
}

// attempts expands a target list into the rungs actually to be tried, in order.
// A target can become two rungs (the caller's key and the gateway's) or one, and
// a rung with no adapter is kept rather than dropped so the caller still learns
// why it was skipped.
func (r *Router) attempts(ctx context.Context, targets []config.Target) []attempt {
	creds, _ := ctx.Value(CallerKeysCtxKey{}).(CallerKeys)
	creds = creds.Normalize()

	out := make([]attempt, 0, len(targets))
	for _, t := range targets {
		global, hasGlobal := r.adapters[t.Provider]
		globalRung := attempt{Target: t, label: t.Provider}
		switch {
		case gatewayDenied(ctx, t.Provider):
			globalRung.reason = errors.New("using the gateway's key for this provider needs approval — request it in the console, or send your own key")
		case hasGlobal:
			globalRung.adapter = global
		}

		key := creds.Keys[strings.ToLower(t.Provider)]
		ownRung := attempt{
			Target: t,
			label:  t.Provider + " (your key)",
			caller: true,
		}
		switch {
		case key == "":
			ownRung.reason = errors.New("no key of yours for this provider")
		case r.callerAdapter == nil:
			ownRung.reason = errors.New("this gateway does not accept caller-supplied keys")
		default:
			if a, ok := r.callerAdapter(t.Provider, key); ok {
				ownRung.adapter = a
			} else {
				ownRung.reason = errors.New("this gateway does not define that provider, so your key for it cannot be used")
			}
		}

		switch creds.Mode {
		case ModeOwn:
			out = append(out, ownRung)
		case ModeOwnFirst:
			// A key the caller never sent is not a failure worth reporting on
			// every rung; drop the empty own rung and go straight to the
			// gateway's, which is what own-first means in that case.
			if ownRung.adapter != nil || key != "" {
				out = append(out, ownRung)
			}
			out = append(out, globalRung)
		case ModeGlobalFirst:
			out = append(out, globalRung)
			if ownRung.adapter != nil || key != "" {
				out = append(out, ownRung)
			}
		default: // ModeGlobal
			out = append(out, globalRung)
		}
	}
	return out
}

// SetCallerAdapter enables bring-your-own-key. Without it, caller-supplied keys
// are refused with a reason rather than silently ignored.
func (r *Router) SetCallerAdapter(f CallerAdapterFunc) { r.callerAdapter = f }

// GatewayDeniedCtxKey is the context key for the set of providers whose
// *gateway* credential this caller may not spend. It gates nothing else: the
// caller's own key for the same provider is untouched, because a grant is about
// the deployment's money, not about the provider.
type GatewayDeniedCtxKey struct{}

// gatewayDenied reports whether the caller must ask before spending the
// gateway's credential on this provider.
func gatewayDenied(ctx context.Context, providerName string) bool {
	denied, _ := ctx.Value(GatewayDeniedCtxKey{}).(map[string]bool)
	return denied[strings.ToLower(providerName)]
}

// HasProvider reports whether an adapter for this provider came up, which is
// the same test routing applies.
func (r *Router) HasProvider(name string) bool {
	_, ok := r.adapters[name]
	return ok
}
