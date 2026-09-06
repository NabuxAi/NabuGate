package server

import (
	"context"
	"net/http"
	"strings"

	"nabugate/internal/router"
)

// --- Bring your own key, over HTTP ---
//
// A request names a credential per provider:
//
//	X-Nabu-Key-Gemini:  AIza...
//	X-Nabu-Key-Openai:  sk-...
//	X-Nabu-Key-Mode:    own-first
//
// The header suffix is the provider name as this deployment's config spells it,
// case-insensitively. Nothing is stored: the keys live for the request and are
// gone with it, which is why this is a header and not a console setting.

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
		key := strings.TrimSpace(values[0])
		if key == "" {
			continue
		}
		if out.Keys == nil {
			out.Keys = map[string]string{}
		}
		out.Keys[provider] = key
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
