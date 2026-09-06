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

// withOwnerCredentials layers a console user's saved state onto the request:
// the upstream keys they stored, and the providers whose gateway credential
// they are not (yet) allowed to spend.
//
// A header always wins over a stored key — explicit beats saved — and
// X-Nabu-Key-Mode: global still routes through the gateway, so saving a key
// once does not trap a user into their own credential forever.
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
		merged := make(map[string]string, len(stored)+len(creds.Keys))
		for name, key := range stored {
			merged[name] = key
		}
		for name, key := range creds.Keys {
			merged[name] = key
		}
		ctx = context.WithValue(ctx, router.CallerKeysCtxKey{},
			router.CallerKeys{Keys: merged, Mode: mode}.Normalize())
	}

	// Providers this deployment holds back until asked. Only ever the
	// gateway's own credential: the user's key for the same provider is
	// unaffected, so a grant can add access and never remove it.
	denied := map[string]bool{}
	approved := s.admin.ApprovedProviders(owner)
	for name, meta := range s.providers {
		if meta.Access == "request" && !approved[name] {
			denied[name] = true
		}
	}
	if len(denied) > 0 {
		ctx = context.WithValue(ctx, router.GatewayDeniedCtxKey{}, denied)
	}
	return ctx
}
