package provider

import "testing"

// The gateway speaks OpenAI's wire protocol, so a caller reaching an audio
// alias sends an OpenAI voice name. Gemini rejects those outright, which made
// every Gemini rung of every audio alias unreachable: the fallback existed in
// the config and had never once served a request.
func TestGeminiVoiceMapsOpenAINames(t *testing.T) {
	for in, want := range map[string]string{
		"alloy":   "Kore",
		"ALLOY":   "Kore", // callers do not agree on case
		"onyx":    "Orus",
		"shimmer": "Leda",
		" nova ":  "Aoede", // and they do not agree on whitespace
	} {
		if got := geminiVoice(in); got != want {
			t.Errorf("geminiVoice(%q) = %q, want %q", in, got, want)
		}
	}
}

// A caller who names a Gemini voice deliberately must still get that voice —
// mapping is a translation for foreign names, not a filter on native ones.
func TestGeminiVoicePassesThroughNativeNames(t *testing.T) {
	for _, native := range []string{"Puck", "Charon", "Zephyr"} {
		if got := geminiVoice(native); got != native {
			t.Errorf("geminiVoice(%q) = %q, want it unchanged", native, got)
		}
	}
}

// An unset voice has to resolve to something Gemini accepts, because the
// OpenAI request that produced it may legitimately have omitted the field.
func TestGeminiVoiceDefaults(t *testing.T) {
	for _, empty := range []string{"", "   "} {
		if got := geminiVoice(empty); got != "Kore" {
			t.Errorf("geminiVoice(%q) = %q, want %q", empty, got, "Kore")
		}
	}
}
