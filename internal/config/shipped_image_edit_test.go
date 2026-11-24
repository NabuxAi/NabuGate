package config

import (
	"math"
	"testing"

	"nabugate/internal/policy"
	"nabugate/internal/provider"
	"nabugate/internal/usage"
)

// nabu-image-edit is Wan 2.6 Image with no fallback, billed per output picture
// at the wan2.6-t2i rate, and NabuGen's key may call it.
func TestShippedImageEditAlias(t *testing.T) {
	cfg, err := Load("../../config.default.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	route, ok := cfg.Images["nabu-image-edit"]
	if !ok {
		t.Fatal("config.default.yaml has no nabu-image-edit image alias")
	}
	if route.Primary.Provider != "dashscope-image" || route.Primary.Model != "wan2.6-image" || len(route.Fallback) != 0 {
		t.Fatalf("nabu-image-edit = %+v; want dashscope-image/wan2.6-image and no fallback", route)
	}

	edit, ok := usage.Lookup(cfg.Pricing, "dashscope-image", "wan2.6-image")
	t2i, _ := usage.Lookup(cfg.Pricing, "dashscope-image", "wan2.6-t2i")
	if !ok || edit.PerImage <= 0 || edit != t2i {
		t.Fatalf("wan2.6-image price %+v; want the wan2.6-t2i price %+v", edit, t2i)
	}

	for _, k := range cfg.Server.Keys {
		if k.Project != "nabugen" {
			continue
		}
		if !(policy.Policy{Allow: k.Allow}).Allows("nabu-image-edit") {
			t.Fatalf("nabugen key allow-list %v does not permit nabu-image-edit", k.Allow)
		}
		return
	}
	t.Fatal("config.default.yaml has no nabugen key")
}

// The older Wan text-to-image models each have their own alias, with no
// fallback, priced per picture at the international list price; none takes
// an input picture; NabuGen may call them.
func TestShippedWanText2ImageAliases(t *testing.T) {
	t.Setenv("DASHSCOPE_API_KEY", "sk-test")
	cfg, err := Load("../../config.default.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	adapters, _ := cfg.BuildAdapters()
	in, ok := adapters["dashscope-image"].(provider.ImageInputAdapter)
	if !ok {
		t.Fatal("dashscope-image did not build as an image adapter that reports input support")
	}
	var allow []string
	for _, k := range cfg.Server.Keys {
		if k.Project == "nabugen" {
			allow = k.Allow
		}
	}
	for alias, want := range map[string]struct {
		model string
		price float64
	}{
		"nabu-wan25":     {"wan2.5-t2i-preview", 0.03},
		"nabu-wan22":     {"wan2.2-t2i-plus", 0.05},
		"nabu-wan-flash": {"wan2.2-t2i-flash", 0.025},
	} {
		route, ok := cfg.Images[alias]
		if !ok || route.Primary.Provider != "dashscope-image" || route.Primary.Model != want.model || len(route.Fallback) != 0 {
			t.Errorf("%s = %+v (present %v); want dashscope-image/%s and no fallback", alias, route, ok, want.model)
			continue
		}
		p, ok := usage.Lookup(cfg.Pricing, "dashscope-image", want.model)
		if !ok || math.Abs(p.PerImage-want.price) > 1e-9 {
			t.Errorf("dashscope-image/%s priced %+v, want $%v a picture", want.model, p, want.price)
		}
		if in.AcceptsInputImages(want.model) {
			t.Errorf("%s claims to take input pictures", want.model)
		}
		if !(policy.Policy{Allow: allow}).Allows(alias) {
			t.Errorf("nabugen key allow-list %v does not permit %s", allow, alias)
		}
	}
}

// Both OpenRouter keys make pictures through chat completions, so their
// image-output models take input pictures; and NabuGen's openrouter/* grant
// covers those models' passthrough ids on /v1/images/generations.
func TestShippedOpenRouterImages(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")
	t.Setenv("OPENROUTER_API_KEY_2", "sk-or-test-2")
	cfg, err := Load("../../config.default.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	adapters, _ := cfg.BuildAdapters()
	for _, name := range []string{"openrouter", "openrouter2"} {
		if cfg.Providers[name].ImageAPI != "chat" || !cfg.Providers[name].Passthrough {
			t.Errorf("%s: image_api %q, passthrough %v; want chat and passthrough", name, cfg.Providers[name].ImageAPI, cfg.Providers[name].Passthrough)
		}
		in, ok := adapters[name].(provider.ImageInputAdapter)
		if !ok || !in.AcceptsInputImages("google/gemini-3.1-flash-image") {
			t.Errorf("%s does not take input pictures for its image models", name)
		}
		if _, ok := cfg.Passthroughs(adapters)[name]; !ok {
			t.Errorf("%s is not routed as a passthrough provider", name)
		}
	}
	var allow []string
	for _, k := range cfg.Server.Keys {
		if k.Project == "nabugen" {
			allow = k.Allow
		}
	}
	for _, id := range []string{
		"openrouter/google/gemini-3.1-flash-image", "openrouter/google/gemini-3-pro-image",
		"openrouter/google/gemini-2.5-flash-image", "openrouter/google/gemini-nano-banana-2.1",
		"openrouter/openai/gpt-5-image", "openrouter/openai/gpt-5-image-mini", "openrouter/openai/gpt-5.4-image-2",
	} {
		if !(policy.Policy{Allow: allow}).Allows(id) {
			t.Errorf("nabugen key allow-list %v does not permit %s", allow, id)
		}
	}
}
