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
	if route.Primary.Provider != "dashscope-video" || route.Primary.Model != "wan2.6-t2v" {
		t.Fatalf("nabu-video = %+v; want dashscope-video/wan2.6-t2v", route.Primary)
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
