package config

import (
	"sort"
	"strings"
	"testing"
)

// Every route an alias can take has a price. An unpriced call is served and
// metered at nothing: on 2026-09-26 that was a hundred of this file's routes,
// nabu-concierge among them, and the console showed a busy month that cost
// nothing. A model that is free on purpose is priced at zero, which is a
// decision; leaving it out is not.
func TestEveryRouteIsPriced(t *testing.T) {
	cfg, err := Load("../../config.default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	unpriced := cfg.UnpricedRoutes()
	if len(unpriced) == 0 {
		return
	}
	routes := make([]string, 0, len(unpriced))
	for route, aliases := range unpriced {
		routes = append(routes, route+" ("+strings.Join(aliases, ", ")+")")
	}
	sort.Strings(routes)
	t.Fatalf("%d routes have no price in config.default.yaml:\n  %s", len(routes), strings.Join(routes, "\n  "))
}

// A named voice is only useful if the vendors it names are ones this file
// routes speech or live calls to; a typo here would silently send nothing.
func TestNamedVoicesNameKnownProviders(t *testing.T) {
	cfg, err := Load("../../config.default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for name, byProvider := range cfg.Voices {
		for prov, voice := range byProvider {
			if _, ok := cfg.Providers[prov]; !ok {
				t.Errorf("voice %q names unknown provider %q", name, prov)
			}
			if strings.TrimSpace(voice) == "" {
				t.Errorf("voice %q has an empty %s voice", name, prov)
			}
		}
	}
	for alias, route := range cfg.Live {
		if v := route.Voice; v != "" && cfg.Voices[v] == nil {
			t.Errorf("live alias %s defaults to %q, which is not a named voice", alias, v)
		}
	}
}
