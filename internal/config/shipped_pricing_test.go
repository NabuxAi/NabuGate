package config

import (
	"testing"
)

// The "Requested Fictional Models" block used to sit at the end of
// config.example.yaml: ten alias-shaped entries written inside `pricing:`,
// which is not a routing table. yaml.v3 (deliberately non-strict) parsed each
// as a zero-priced Price entry with a "primary" key it ignored, and since
// they were never under `models:`, no caller could reach them anyway — dead
// twice over. They were removed rather than moved, because moving them would
// have needed real prices for coordinates the vendors never sold and a
// /v1/models surface change for every caller.
//
// This pins the removal on the one file that carried the block.
// config.default.yaml legitimately prices models with these same names
// (gpt-5.5, claude-sonnet-5, …) as real vendor coordinates, so the assertion
// must not touch it.
func TestFictionalModelBlockIsGoneFromTheExampleConfig(t *testing.T) {
	cfg, err := Load("../../config.example.yaml")
	if err != nil {
		t.Fatalf("load config.example.yaml: %v", err)
	}
	for _, name := range []string{
		"claude-3-5-haiku-latest", "claude-haiku-4.5", "claude-opus-4.8", "claude-opus-5",
		"claude-sonnet-4", "claude-sonnet-5", "gpt-5.5", "gpt-5.6-luna", "gpt-5.6-sol", "gpt-5.6-terra",
	} {
		if _, ok := cfg.Pricing[name]; ok {
			t.Errorf("pricing key %q is a fictional-model leftover: the block inside `pricing:` was removed because nothing could ever route to it", name)
		}
	}
}
