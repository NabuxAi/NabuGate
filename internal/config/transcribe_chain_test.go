package config

import (
	"strings"
	"testing"

	"nabugate/internal/provider"
)

// The shipped config must actually produce a transcription chain that reaches
// the new vendors: a rung whose provider never builds is a slower path to the
// same 502.
func TestShippedTranscribeChainReachesNewVendors(t *testing.T) {
	t.Setenv("SPEECHMATICS_API_KEY", "test-key")
	t.Setenv("GEMINI_API_KEY", "test-key")

	c, err := Load("../../config.default.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	route, ok := c.Transcription["nabu-transcribe"]
	if !ok {
		t.Fatal("nabu-transcribe alias missing")
	}

	adapters, warnings := c.BuildAdapters()
	seen := map[string]bool{}
	for _, tgt := range append([]Target{route.Primary}, route.Fallback...) {
		a, built := adapters[tgt.Provider]
		if !built {
			continue
		}
		if _, ok := a.(provider.TranscriptionAdapter); ok {
			seen[tgt.Provider] = true
		}
	}
	for _, want := range []string{"speechmatics", "gemini"} {
		if !seen[want] {
			t.Errorf("chain has no usable %s rung; warnings: %s", want, strings.Join(warnings, "; "))
		}
	}
}
