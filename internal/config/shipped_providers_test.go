package config

import (
	"os"
	"testing"

	"nabugate/internal/provider"
)

// The shipped configs must load and build the providers they declare. Both
// files are hand-edited YAML, and the failure mode of a typo in one is not a
// crash — a provider that cannot be built is skipped with a warning, so the
// gateway boots looking healthy with that provider silently missing.
//
// Replicate and RunPod are the two worth pinning: Replicate because it is the
// only provider whose `type` has no base_url to make a mistake visible, and
// RunPod because its base_url is templated from an env var, which is exactly
// the shape that quietly resolves to a malformed URL.
func TestShippedConfigsBuildReplicateAndRunpod(t *testing.T) {
	t.Setenv("REPLICATE_API_TOKEN", "r8-test")
	t.Setenv("RUNPOD_API_KEY", "rp-test")
	t.Setenv("RUNPOD_ENDPOINT_ID", "abc123")

	for _, path := range []string{"../../config.example.yaml", "../../config.default.yaml"} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("%s: load: %v", path, err)
		}
		adapters, warnings := cfg.BuildAdapters()
		for _, name := range []string{"replicate", "runpod"} {
			if adapters[name] == nil {
				t.Errorf("%s: %s did not build; warnings: %v", path, name, warnings)
			}
		}
		if pt := cfg.Passthroughs(adapters); pt["replicate"] == nil || len(pt["replicate"]) == 0 {
			t.Errorf("%s: replicate is not a passthrough with a static catalogue", path)
		}
		if _, ok := cfg.Passthroughs(adapters)["runpod"]; !ok {
			t.Errorf("%s: runpod is not a passthrough", path)
		}
	}
}

// v1m (v1m.ir) rides the typesafe adapter with its own base_url, the way
// arvan/9router ride the openai one. A typo in its base_url or type would not
// fail the boot — the provider either builds against the wrong URL and every
// call 404s, or falls to an unknown type and is skipped with a warning — so
// pin what both shipped files declare, and that it builds as a TypeSafe
// adapter with passthrough and a priced decision alias.
func TestShippedConfigsBuildV1M(t *testing.T) {
	t.Setenv("V1M_API_KEY", "v1m-test")

	for _, path := range []string{"../../config.example.yaml", "../../config.default.yaml"} {
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("%s: load: %v", path, err)
		}
		p, ok := cfg.Providers["v1m"]
		if !ok {
			t.Fatalf("%s: no v1m provider declared", path)
		}
		if p.Type != "typesafe" {
			t.Errorf("%s: v1m type = %q, want typesafe", path, p.Type)
		}
		if p.BaseURL != "https://v1m.ir/v1" {
			t.Errorf("%s: v1m base_url = %q, want https://v1m.ir/v1", path, p.BaseURL)
		}
		if !p.Passthrough {
			t.Errorf("%s: v1m is not passthrough", path)
		}
		if p.APIKeyEnv != "V1M_API_KEY" {
			t.Errorf("%s: v1m api_key_env = %q, want V1M_API_KEY", path, p.APIKeyEnv)
		}

		adapters, warnings := cfg.BuildAdapters()
		a, ok := adapters["v1m"]
		if !ok {
			t.Fatalf("%s: v1m did not build; warnings: %v", path, warnings)
		}
		if _, isTypeSafe := a.(*provider.TypeSafeAdapter); !isTypeSafe {
			t.Errorf("%s: v1m built as %T, want *provider.TypeSafeAdapter", path, a)
		}

		route, ok := cfg.Decisions["nabu-v1m"]
		if !ok {
			t.Fatalf("%s: no nabu-v1m decision alias declared", path)
		}
		if route.Primary.Provider != "v1m" || route.Primary.Model != "v1m-latest" {
			t.Errorf("%s: nabu-v1m primary = %+v, want v1m/v1m-latest", path, route.Primary)
		}
		if len(route.Fallback) != 1 || route.Fallback[0].Provider != "typesafe" || route.Fallback[0].Model != "jev-latest" {
			t.Errorf("%s: nabu-v1m fallback = %+v, want typesafe/jev-latest", path, route.Fallback)
		}

		if _, priced := cfg.Pricing["v1m/v1m-latest"]; !priced {
			t.Errorf("%s: v1m/v1m-latest has no pricing entry", path)
		}
	}
}
