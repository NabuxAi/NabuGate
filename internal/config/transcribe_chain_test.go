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

// The shipped config must leave cross-origin support off. It is the one setting
// here whose default decides whether the gateway is reachable from any page on
// the internet, so it is worth a test rather than a reading of the file.
func TestShippedConfigLeavesCORSOff(t *testing.T) {
	c, err := Load("../../config.default.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := c.Server.CORSOrigins; len(got) != 0 {
		t.Errorf("cors_origins = %v; the shipped default must open nothing", got)
	}
}

// Every provider the shipped config defines must be reachable by a caller's own
// key, or say why not. A provider with an api_key_env that BYOK refuses is a
// silent dead end on the console's catalogue screen.
func TestShippedProvidersAgreeOnBYOK(t *testing.T) {
	t.Setenv("NABUGATE_SECRET_KEY", "x")
	c, err := Load("../../config.default.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for name, meta := range c.ProviderMetas() {
		if !meta.Enabled {
			continue
		}
		if meta.Access != "auto" && meta.Access != "request" {
			t.Errorf("%s: access = %q, want auto or request", name, meta.Access)
		}
		if !meta.BYOK {
			continue
		}
		if _, ok := c.CallerAdapter(name, "test-key"); !ok {
			t.Errorf("%s: advertises BYOK on the catalogue but CallerAdapter refuses it", name)
		}
	}
}
