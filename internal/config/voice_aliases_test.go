package config

import (
	"strings"
	"testing"
)

// Speech models that read Persian script with Persian phonology, checked
// against each vendor's language list. A model that is not here may still
// "speak" Persian text — in the wrong phonology, which sounds like a bad voice
// rather than a wrong model, so nobody files it as a bug.
var persianVoiceModels = map[string]bool{
	"eleven_v3":                    true,
	"eleven_v3_conversational":     true,
	"eleven_v4":                    true,
	"eleven_v4_turbo":              true,
	"gemini-2.5-flash-preview-tts": true,
	"gemini-3.8-flash-tts":         true,
	"gpt-4o-mini-tts":              true,
	"pocket-tts-farsi-v2":          true,
}

// Aliases a product may send Persian text to without choosing by language.
// Every rung of each must speak Persian: a fallback is taken exactly when the
// rungs above it fail, which is when nobody is listening for a wrong accent.
func TestFastVoiceAliasesSpeakPersian(t *testing.T) {
	cfg, err := Load("../../config.default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"nabu-voice-fast", "nabu-voice-fa", "nabu-voice-fa-local"} {
		route, ok := cfg.Audio[alias]
		if !ok {
			t.Errorf("audio alias %s is missing", alias)
			continue
		}
		for _, target := range append([]Target{route.Primary}, route.Fallback...) {
			if !persianVoiceModels[target.Model] {
				t.Errorf("%s routes to %s/%s, which is not known to speak Persian", alias, target.Provider, target.Model)
			}
		}
	}
}

// Grok's voice has no Persian. Only the alias that says so in its name may
// route to it; anywhere else it would be a rung that some Persian request
// eventually falls through to.
func TestGrokVoiceOnlyBehindItsOwnAlias(t *testing.T) {
	cfg, err := Load("../../config.default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if route := cfg.Audio["nabu-voice-grok"]; route.Primary.Provider != "xai-voice" {
		t.Errorf("nabu-voice-grok should open on xai-voice, got %q", route.Primary.Provider)
	}
	for alias, route := range cfg.Audio {
		if alias == "nabu-voice-grok" {
			continue
		}
		for _, target := range append([]Target{route.Primary}, route.Fallback...) {
			if target.Provider == "xai-voice" {
				t.Errorf("%s routes to Grok's voice, which cannot read Persian", alias)
			}
		}
	}
}

// The call brain answers a caller who is waiting in silence. One vendor's
// outage must not silence it, so its rungs span at least three providers.
func TestVoiceBrainSpansThreeVendors(t *testing.T) {
	cfg, err := Load("../../config.default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	route, ok := cfg.Models["nabu-voice-brain"]
	if !ok {
		t.Fatal("chat alias nabu-voice-brain is missing")
	}
	vendors := map[string]bool{}
	for _, target := range append([]Target{route.Primary}, route.Fallback...) {
		coords, ok := cfg.coordinates(target)
		if !ok {
			t.Errorf("nabu-voice-brain rung %s/%s is not served by any provider", target.Provider, target.Model)
			continue
		}
		for _, coord := range coords {
			prov, _, _ := strings.Cut(coord, "/")
			vendors[strings.TrimSuffix(prov, "2")] = true
		}
	}
	if len(vendors) < 3 {
		t.Errorf("nabu-voice-brain reaches only %d vendors: %v", len(vendors), vendors)
	}
}
