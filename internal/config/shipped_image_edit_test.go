package config

import (
	"testing"

	"nabugate/internal/policy"
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
