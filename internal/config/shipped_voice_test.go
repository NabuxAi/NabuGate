package config

import "testing"

// nabu-voice-gemini is the voice a caller chose by ear (NabuCRM's spoken
// console replies), so its first rung must be Gemini whatever happens to the
// other aliases: nabu-voice puts OpenAI first and would quietly change voice
// the day OpenAI's budget returns. Its fallback must read Persian.
func TestShippedConfigsPinTheGeminiVoice(t *testing.T) {
	for _, path := range []string{"../../config.example.yaml", "../../config.default.yaml"} {
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("%s: load: %v", path, err)
		}
		route, ok := cfg.Audio["nabu-voice-gemini"]
		if !ok {
			t.Fatalf("%s: no nabu-voice-gemini speech alias", path)
		}
		if route.Primary.Provider != "gemini" {
			t.Errorf("%s: nabu-voice-gemini starts at %q, want gemini", path, route.Primary.Provider)
		}
		if len(route.Fallback) == 0 || route.Fallback[0].Provider != "elevenlabs" || route.Fallback[0].Model != "eleven_v3" {
			t.Errorf("%s: nabu-voice-gemini should fall back to ElevenLabs eleven_v3, which reads Persian; got %+v", path, route.Fallback)
		}
	}
}
