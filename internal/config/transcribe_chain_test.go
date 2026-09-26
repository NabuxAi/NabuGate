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

// nabu-transcribe-live is for a person waiting on their own words, so its
// first rung must be a hosted engine that answers in a second, asked for plain
// json (the gpt-4o models refuse verbose_json), and the self-hosted whisper
// must still be there at the end for the day every vendor is down.
func TestShippedLiveTranscribeLeadsWithAHostedEngine(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("ELEVENLABS_API_KEY", "test-key")
	t.Setenv("WHISPER_BASE_URL", "http://whisper:8000/v1")

	c, err := Load("../../config.default.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	route, ok := c.Transcription["nabu-transcribe-live"]
	if !ok {
		t.Fatal("nabu-transcribe-live alias missing")
	}
	if route.Primary.Provider != "openai-transcribe" || route.Primary.Model != "gpt-4o-transcribe" {
		t.Errorf("primary = %s/%s, want openai-transcribe/gpt-4o-transcribe", route.Primary.Provider, route.Primary.Model)
	}
	if p, ok := c.Providers["openai-transcribe"]; !ok || p.TranscribeFormat != "json" {
		t.Errorf("openai-transcribe must declare transcribe_format json, got %+v", p)
	}

	adapters, warnings := c.BuildAdapters()
	chain := append([]Target{route.Primary}, route.Fallback...)
	for _, want := range []string{"openai-transcribe", "elevenlabs", "whisper"} {
		found := false
		for _, tgt := range chain {
			if tgt.Provider != want {
				continue
			}
			if a, built := adapters[want]; built {
				if _, ok := a.(provider.TranscriptionAdapter); ok {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("chain has no usable %s rung; warnings: %s", want, strings.Join(warnings, "; "))
		}
	}
	if last := chain[len(chain)-1]; last.Provider != "whisper" {
		t.Errorf("last rung = %s, want the self-hosted whisper", last.Provider)
	}
}

