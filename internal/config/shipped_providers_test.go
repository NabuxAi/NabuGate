package config

import (
	"os"
	"testing"
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
