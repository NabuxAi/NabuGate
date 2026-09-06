package server

import (
	"context"
	"net/http"
	"strings"

	"nabugate/internal/adminstore"
	"nabugate/internal/router"
)

// --- Bring your own key, over HTTP ---
//
// A request names a credential per provider:
//
//	X-Nabu-Key-Gemini:  AIza...
//	X-Nabu-Key-Gemini:  AIza...another
//	X-Nabu-Key-Openai:  sk-...
//	X-Nabu-Key-Mode:    own-first
//
// The header suffix is the provider name as this deployment's config spells it,
// case-insensitively. Nothing is stored: the keys live for the request and are
// gone with it, which is why this is a header and not a console setting.
//
// Repeating a header adds a fallback: the second key is tried when the first
// fails. HTTP already carries repeated headers as a list, so this needs no
// separator — and it deliberately does not accept one. Splitting on commas
// would turn a single vendor key that happens to contain one into two invalid
// keys, which fails in a way nobody would think to look for.

// callerKeyPrefix is the header namespace that carries caller credentials.
const callerKeyPrefix = "X-Nabu-Key-"

// modeHeader selects the order the credentials are tried in. It sits inside the
// same prefix, so "mode" is not a usable provider name here — no provider is
// called that, and reserving it costs nothing.
const modeHeader = callerKeyPrefix + "Mode"

// callerKeysFrom reads the caller's credentials off a request.
func callerKeysFrom(r *http.Request) router.CallerKeys {
	out := router.CallerKeys{Mode: strings.ToLower(strings.TrimSpace(r.Header.Get(modeHeader)))}
	for name, values := range r.Header {
		if !strings.HasPrefix(name, callerKeyPrefix) || len(values) == 0 {
			continue
		}
		provider := strings.ToLower(strings.TrimPrefix(name, callerKeyPrefix))
		if provider == "" || provider == "mode" {
			continue
		}
		for _, v := range values {
			key := strings.TrimSpace(v)
			if key == "" {
				continue
			}
			if out.Keys == nil {
				out.Keys = map[string][]string{}
			}
			// Same cap the store puts on saved keys, for the same reason: every
			// key is a rung, every rung is an upstream connection, and the
			// transcription client waits 45 minutes. Without this the header
			// path walks around the limit the store enforces.
			if len(out.Keys[provider]) >= adminstore.MaxKeysPerProvider {
				continue
			}
			out.Keys[provider] = append(out.Keys[provider], key)
		}
	}
	return out.Normalize()
}

// withCallerKeys attaches the caller's credentials and a recorder for whichever
// one ends up serving. Both are needed on every metered path, so this wraps the
// context once rather than being repeated per handler.
func withCallerKeys(ctx context.Context, r *http.Request) (context.Context, *router.KeySource) {
	rec := router.NewKeySource()
	ctx = context.WithValue(ctx, router.CallerKeysCtxKey{}, callerKeysFrom(r))
	ctx = context.WithValue(ctx, router.KeySourceCtxKey{}, rec)
	return ctx, rec
}

// servedByCaller reports whether the caller's own credential paid for this
// request, which is what stops the gateway billing for it.
func servedByCaller(ctx context.Context) bool {
	rec, _ := ctx.Value(router.KeySourceCtxKey{}).(*router.KeySource)
	return rec.Caller()
}

// withOwnerCredentials layers a console user's saved state onto the request:
// the upstream keys they stored, and the providers whose gateway credential
// they are not (yet) allowed to spend.
//
// A header always wins over a stored key — explicit beats saved — which with
// lists means the headers are tried first and the saved keys become further
// fallbacks behind them, rather than one replacing the other. X-Nabu-Key-Mode:
// global still routes through the gateway, so saving a key once does not trap a
// user into their own credential forever.
func (s *Server) withOwnerCredentials(ctx context.Context, owner string) context.Context {
	if s.admin == nil || strings.TrimSpace(owner) == "" {
		return ctx
	}

	if stored := s.admin.ProviderKeys(owner); len(stored) > 0 {
		creds, _ := ctx.Value(router.CallerKeysCtxKey{}).(router.CallerKeys)
		// Re-derive the mode from the merged set rather than keeping the one
		// Normalize picked from headers alone: a user with a saved key and no
		// headers sent has keys, and "global" would ignore every one of them.
		mode := creds.Mode
		if len(creds.Keys) == 0 {
			mode = ""
		}
		merged := make(map[string][]string, len(stored)+len(creds.Keys))
		for name, keys := range creds.Keys {
			merged[name] = append(merged[name], keys...)
		}
		for name, keys := range stored {
			merged[name] = append(merged[name], keys...)
		}
		// The two sources are capped separately, so their sum is not. Trim the
		// tail rather than refusing: the keys most likely to serve are at the
		// front, and a request is not the place to argue about the limit.
		for name, keys := range merged {
			if len(keys) > adminstore.MaxKeysPerProvider {
				merged[name] = keys[:adminstore.MaxKeysPerProvider]
			}
		}
		ctx = context.WithValue(ctx, router.CallerKeysCtxKey{},
			router.CallerKeys{Keys: merged, Mode: mode}.Normalize())
	}

	// Providers this deployment holds back until asked. Only ever the
	// gateway's own credential: the user's key for the same provider is
	// unaffected, so a grant can add access and never remove it.
	//
	// Two ways to be let through, and they are alternatives, not a sequence: an
	// admin approved this user for this provider, or their subscription covers
	// it. Adding the second as another way to satisfy the same gate is what
	// keeps this from breaking anyone — nothing that worked before stops.
	denied := map[string]bool{}
	approved := s.admin.ApprovedProviders(owner)
	sub, subscribed := s.admin.ActiveSubscription(owner)
	for name, meta := range s.providers {
		if meta.Access != "request" || approved[name] {
			continue
		}
		if subscribed && sub.Covers(name) {
			continue
		}
		denied[name] = true
	}
	if len(denied) > 0 {
		ctx = context.WithValue(ctx, router.GatewayDeniedCtxKey{}, denied)
	}
	// What a call served on the gateway's own key costs this user. Read here,
	// where the subscription is already in hand, rather than again at billing
	// time — one store lookup per request instead of two, and the rate cannot
	// change between the gate and the bill.
	if subscribed {
		ctx = context.WithValue(ctx, gatewayRateCtxKey{}, gatewayRate{
			markup:    sub.Markup,
			minCharge: sub.MinChargeUSD,
		})
	}
	return ctx
}

// gatewayRate is what the user's plan charges for spending the gateway's
// credential. A caller on their own key never reaches this: their vendor
// already billed them, and the gateway charges zero for the passing along.
type gatewayRate struct {
	markup    float64
	minCharge float64
}

type gatewayRateCtxKey struct{}

// apply prices one call served on the gateway's key.
//
// The floor is why this is not a bare multiplication. A model with no `pricing:`
// entry meters at zero, and any multiple of zero is zero — without a floor a
// plan could bill nothing for a month of transcription, which is the exact
// traffic this feature exists to sell.
func (g gatewayRate) apply(cost float64) float64 {
	if g.markup > 1 {
		cost *= g.markup
	}
	if cost < g.minCharge {
		cost = g.minCharge
	}
	return cost
}

// gatewayRateFrom returns the plan rate on this request, and the zero rate —
// which charges exactly the metered cost — when there is no subscription.
func gatewayRateFrom(ctx context.Context) gatewayRate {
	g, _ := ctx.Value(gatewayRateCtxKey{}).(gatewayRate)
	return g
}
