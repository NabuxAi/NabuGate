package config

import (
	"strings"
	"testing"

	"nabugate/internal/provider"
	"nabugate/internal/usage"
)

// The MiniMax and MiMo aliases are the public names a caller sends. Each one
// has to resolve through the registry onto hosts whose prices are written
// down: an unpriced call is served and metered at nothing.
func TestMiniMaxAndMiMoAliases(t *testing.T) {
	cfg, err := Load("../../config.default.yaml")
	if err != nil {
		t.Fatal(err)
	}

	aliases := []string{
		"minimax-m2.7", "minimax-m3", "minimax-m3.1-flash",
		"mimo-v2.5", "mimo-v2.5-pro", "mimo-v2.6-flash", "mimo-v2.6-pro",
		"mimo-v2.6-pro-ultraspeed",
	}
	for _, name := range aliases {
		route, ok := cfg.Models[name]
		if !ok {
			t.Errorf("alias %s is not in models:", name)
			continue
		}
		if route.Primary.Provider != "" || route.Primary.Model != name {
			t.Errorf("alias %s primary = %+v, want registry model %s", name, route.Primary, name)
		}
		entry, ok := cfg.Registry[name]
		if !ok || len(entry.Serves) == 0 {
			t.Errorf("alias %s has no registry serves", name)
			continue
		}
		for _, sv := range entry.Serves {
			if _, priced := usage.Lookup(cfg.Pricing, sv.Provider, sv.Model); !priced {
				t.Errorf("%s serve %s/%s has no price", name, sv.Provider, sv.Model)
			}
		}
	}

	// List prices, whoever serves the model. Host-specific rows are checked
	// separately because a ":novita" suffix is not the bare model name.
	list := map[string]usage.Price{
		"minimax-m3":               {Input: 0.30, Output: 1.20},
		"minimax-m3.1-flash":       {Input: 0.15, Output: 0.60},
		"mimo-v2.5":                {Input: 0.14, Output: 0.28},
		"mimo-v2.5-pro":            {Input: 0.435, Output: 0.87},
		"mimo-v2.6-flash":          {Input: 0.14, Output: 0.28},
		"mimo-v2.6-pro":            {Input: 0.435, Output: 0.87},
		"mimo-v2.6-pro-ultraspeed": {Input: 4.35, Output: 8.70},
	}
	for id, want := range list {
		prov := "minimax"
		if strings.HasPrefix(id, "mimo-") {
			prov = "mimo"
		}
		got, ok := usage.Lookup(cfg.Pricing, prov, id)
		if !ok || got != want {
			t.Errorf("price %s = %+v (ok=%v), want %+v", id, got, ok, want)
		}
	}

	// The OpenRouter free id must stay free. Pricing it at the paid MiniMax
	// rate would bill a rate-limited $0 route.
	free, ok := usage.Lookup(cfg.Pricing, "openrouter", "minimax/minimax-m3.1-flash:free")
	if !ok || free != (usage.Price{}) {
		t.Errorf("openrouter free flash = %+v (ok=%v), want a free price", free, ok)
	}

	hosts := map[string]usage.Price{
		"huggingface/MiniMaxAI/MiniMax-M3:novita":          {Input: 0.30, Output: 1.20},
		"huggingface/XiaomiMiMo/MiMo-V2.5-Pro:novita":      {Input: 0.522, Output: 1.044},
		"huggingface/XiaomiMiMo/MiMo-V2.6-Flash-RL:novita": {Input: 0.14, Output: 0.28},
		"huggingface/XiaomiMiMo/MiMo-V2.6-Pro-RL:novita":   {Input: 0.435, Output: 0.87},
	}
	for coord, want := range hosts {
		prov, model, _ := strings.Cut(coord, "/")
		got, ok := usage.Lookup(cfg.Pricing, prov, model)
		if !ok || got != want {
			t.Errorf("price %s = %+v (ok=%v), want %+v", coord, got, ok, want)
		}
	}

	var dadebaran bool
	for _, k := range cfg.Server.Keys {
		if k.Project != "dadebaran" {
			continue
		}
		dadebaran = true
		if !allows(k.Allow, "mimo-*") || !allows(k.Allow, "minimax-*") {
			t.Errorf("dadebaran allow = %v, want minimax-* and mimo-*", k.Allow)
		}
	}
	if !dadebaran {
		t.Fatal("dadebaran key missing from config.default.yaml")
	}
}

func allows(patterns []string, want string) bool {
	for _, p := range patterns {
		if p == want {
			return true
		}
	}
	return false
}

// An unset key must not build the provider, and a set key must. Both providers
// are OpenAI-wire, so no adapter of their own.
func TestMiniMaxAndMiMoProvidersFollowTheKey(t *testing.T) {
	cfg, err := Load("../../config.default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"minimax", "mimo"} {
		p, ok := cfg.Providers[name]
		if !ok || !p.Enabled || p.Type != "openai" || p.BaseURL == "" || p.APIKeyEnv == "" {
			t.Fatalf("provider %s = %+v", name, p)
		}
	}
	if cfg.Providers["minimax"].BaseURL != "https://api.minimax.io/v1" {
		t.Errorf("minimax base_url = %q", cfg.Providers["minimax"].BaseURL)
	}
	if cfg.Providers["mimo"].BaseURL != "https://api.xiaomimimo.com/v1" {
		t.Errorf("mimo base_url = %q", cfg.Providers["mimo"].BaseURL)
	}

	t.Setenv("MINIMAX_API_KEY", "")
	t.Setenv("MIMO_API_KEY", "")
	adapters, warnings := cfg.BuildAdapters()
	if adapters["minimax"] != nil || adapters["mimo"] != nil {
		t.Fatalf("empty keys still built adapters: minimax=%v mimo=%v", adapters["minimax"], adapters["mimo"])
	}
	if len(warnings) == 0 {
		t.Fatal("expected a skip warning for the empty keys")
	}

	t.Setenv("MINIMAX_API_KEY", "mm-test")
	t.Setenv("MIMO_API_KEY", "mimo-test")
	adapters, warnings = cfg.BuildAdapters()
	for _, name := range []string{"minimax", "mimo"} {
		a := adapters[name]
		if _, ok := a.(*provider.OpenAIAdapter); !ok {
			t.Errorf("%s built as %T, warnings %v", name, a, warnings)
		}
	}
}
