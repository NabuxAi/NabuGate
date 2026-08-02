package router

import (
	"log/slog"
	"testing"

	"nabugate/internal/config"
	"nabugate/internal/provider"
)

// An alias nothing can serve must not be advertised.
//
// Eight of thirteen chat aliases answered `provider "x" not available` on the
// live gateway — every one because that provider's API key is unset, so it is
// skipped at start-up. Skipping is deliberate. Listing the resulting aliases on
// /v1/models is not: one consumer presents this catalogue directly as its users'
// model picker, which made them options a person could choose and could not use.

func routerWith(adapters map[string]provider.Adapter, models map[string]config.ModelRoute) *Router {
	return &Router{
		adapters:   adapters,
		models:     models,
		images:     map[string]config.ModelRoute{},
		audio:      map[string]config.ModelRoute{},
		embeddings: map[string]config.ModelRoute{},
		log:        slog.New(slog.DiscardHandler),
	}
}

func route(primary string, fallbacks ...string) config.ModelRoute {
	r := config.ModelRoute{Primary: config.Target{Provider: primary, Model: "m"}}
	for _, f := range fallbacks {
		r.Fallback = append(r.Fallback, config.Target{Provider: f, Model: "m"})
	}
	return r
}

func ids(infos []AliasInfo) map[string]string {
	out := map[string]string{}
	for _, i := range infos {
		out[i.ID] = i.Owner
	}
	return out
}

func TestAliasWithNoReachableProviderIsHidden(t *testing.T) {
	r := routerWith(
		map[string]provider.Adapter{"live": nil},
		map[string]config.ModelRoute{
			"works": route("live"),
			"dead":  route("missing"),
		},
	)

	got := ids(r.AliasInfos())

	if _, ok := got["dead"]; ok {
		t.Error("an alias whose only provider is unavailable must not be advertised")
	}
	if _, ok := got["works"]; !ok {
		t.Error("a serviceable alias must still be advertised")
	}
}

func TestAliasSurvivesWhenOnlyAFallbackIsReachable(t *testing.T) {
	// nabu-embed's real shape: primary unavailable, a middle rung that works.
	r := routerWith(
		map[string]provider.Adapter{"live": nil},
		map[string]config.ModelRoute{"mixed": route("missing", "live")},
	)

	got := ids(r.AliasInfos())

	if _, ok := got["mixed"]; !ok {
		t.Fatal("an alias with a working fallback is serviceable and must be listed")
	}
	if got["mixed"] != "live" {
		t.Errorf("owner should name the provider that will serve it, got %q", got["mixed"])
	}
}

func TestEveryAliasHiddenWhenNoProviderIsConfigured(t *testing.T) {
	// A gateway booted with no provider keys at all advertises nothing rather
	// than a full catalogue that fails on every call.
	r := routerWith(
		map[string]provider.Adapter{},
		map[string]config.ModelRoute{"a": route("x"), "b": route("y", "z")},
	)

	if got := r.AliasInfos(); len(got) != 0 {
		t.Errorf("expected no aliases advertised, got %v", ids(got))
	}
}
