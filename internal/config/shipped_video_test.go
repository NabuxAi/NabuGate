package config

import (
	"math"
	"testing"

	"nabugate/internal/policy"
	"nabugate/internal/provider"
	"nabugate/internal/usage"
)

// nabu-video is Wan 2.6 on DashScope's video API, priced per output second at
// both tiers for both the text- and image-to-video models it runs, and
// NabuGen's key may call it.
func TestShippedVideoAlias(t *testing.T) {
	t.Setenv("DASHSCOPE_API_KEY", "sk-test")
	cfg, err := Load("../../config.default.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	route, ok := cfg.Videos["nabu-video"]
	if !ok {
		t.Fatal("config.default.yaml has no nabu-video video alias")
	}
	if route.Primary.Provider != "dashscope-video" || route.Primary.Model != "wan2.6-t2v" || route.Primary.ImageModel != "wan2.6-i2v" {
		t.Fatalf("nabu-video = %+v; want dashscope-video/wan2.6-t2v with image model wan2.6-i2v", route.Primary)
	}

	adapters, warnings := cfg.BuildAdapters()
	if _, ok := adapters["dashscope-video"].(provider.VideoAdapter); !ok {
		t.Fatalf("dashscope-video did not build as a video adapter; warnings: %v", warnings)
	}

	for model, perSecond := range map[string]float64{
		"wan2.6-t2v": 0.10, "wan2.6-t2v@1080P": 0.15,
		"wan2.6-i2v": 0.10, "wan2.6-i2v@1080P": 0.15,
	} {
		p, ok := usage.Lookup(cfg.Pricing, "dashscope-video", model)
		if got := p.Of(usage.Metered{Seconds: 1}); !ok || math.Abs(got-perSecond) > 1e-9 {
			t.Errorf("dashscope-video/%s costs $%v a second, want $%v", model, got, perSecond)
		}
	}

	for _, k := range cfg.Server.Keys {
		if k.Project != "nabugen" {
			continue
		}
		if !(policy.Policy{Allow: k.Allow}).Allows("nabu-video") {
			t.Fatalf("nabugen key allow-list %v does not permit nabu-video", k.Allow)
		}
		return
	}
	t.Fatal("config.default.yaml has no nabugen key")
}

// Every video alias NabuGen calls names a text model and an image model the
// adapter knows, each of the right kind, with no fallback; every tier either
// model renders is priced at Model Studio's international list price; and
// NabuGen's key may call it.
func TestShippedVideoAliasTable(t *testing.T) {
	t.Setenv("DASHSCOPE_API_KEY", "sk-test")
	cfg, err := Load("../../config.default.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// $ a second per tier, from the price list cited in config.default.yaml.
	perSecond := map[string]map[string]float64{
		"wan2.6-t2v":         {"720P": 0.10, "1080P": 0.15},
		"wan2.6-i2v":         {"720P": 0.10, "1080P": 0.15},
		"wan2.5-t2v-preview": {"480P": 0.05, "720P": 0.10, "1080P": 0.15},
		"wan2.5-i2v-preview": {"480P": 0.05, "720P": 0.10, "1080P": 0.15},
		"wan2.2-t2v-plus":    {"480P": 0.02, "1080P": 0.10},
		"wan2.2-i2v-plus":    {"480P": 0.02, "1080P": 0.10},
		"wan2.2-i2v-flash":   {"480P": 0.015, "720P": 0.036},
		"wan2.1-t2v-turbo":   {"480P": 0.036, "720P": 0.036},
		"wan2.1-t2v-plus":    {"720P": 0.10},
		"wan2.1-i2v-plus":    {"720P": 0.10},
	}
	want := map[string][2]string{
		"nabu-video":       {"wan2.6-t2v", "wan2.6-i2v"},
		"nabu-video-wan25": {"wan2.5-t2v-preview", "wan2.5-i2v-preview"},
		"nabu-video-wan22": {"wan2.2-t2v-plus", "wan2.2-i2v-plus"},
		"nabu-video-fast":  {"wan2.1-t2v-turbo", "wan2.2-i2v-flash"},
		"nabu-video-wan21": {"wan2.1-t2v-plus", "wan2.1-i2v-plus"},
	}
	var allow []string
	for _, k := range cfg.Server.Keys {
		if k.Project == "nabugen" {
			allow = k.Allow
		}
	}
	for alias, pair := range want {
		route, ok := cfg.Videos[alias]
		if !ok {
			t.Errorf("config.default.yaml has no %s video alias", alias)
			continue
		}
		p := route.Primary
		if p.Provider != "dashscope-video" || p.Model != pair[0] || p.ImageModel != pair[1] || len(route.Fallback) != 0 {
			t.Errorf("%s = %+v (fallback %v); want dashscope-video %s / %s and no fallback", alias, p, route.Fallback, pair[0], pair[1])
		}
		for i, model := range pair {
			i2v, tiers, known := provider.WanVideoModel(model)
			if !known || i2v != (i == 1) {
				t.Errorf("%s: %s known %v, image-to-video %v", alias, model, known, i2v)
				continue
			}
			for _, tier := range tiers {
				key := model
				if tier != "720P" {
					key += "@" + tier
				}
				price, ok := usage.Lookup(cfg.Pricing, "dashscope-video", key)
				got := price.Of(usage.Metered{Seconds: 1})
				if !ok || math.Abs(got-perSecond[model][tier]) > 1e-9 {
					t.Errorf("dashscope-video/%s costs $%v a second (priced %v), want $%v", key, got, ok, perSecond[model][tier])
				}
			}
		}
		if !(policy.Policy{Allow: allow}).Allows(alias) {
			t.Errorf("nabugen key allow-list %v does not permit %s", allow, alias)
		}
	}
}
