package router

import (
	"context"
	"errors"
	"testing"

	"nabugate/internal/config"
	"nabugate/internal/provider"
)

// speaker is a speech adapter that writes down the voice each rung was asked for.
type speaker struct {
	fakeAdapter
	heard *[]string
}

func (s speaker) Speech(_ context.Context, req provider.SpeechRequest) (provider.SpeechResponse, error) {
	*s.heard = append(*s.heard, s.name+"="+req.Voice)
	if s.err != nil {
		return provider.SpeechResponse{}, s.err
	}
	return provider.SpeechResponse{Audio: []byte("a"), ContentType: "audio/mpeg"}, nil
}

// caller is a live adapter that keeps the session body each rung was sent.
type caller struct {
	fakeAdapter
	bodies *[]string
}

func (c caller) CreateLiveSession(_ context.Context, req provider.LiveSessionRequest) (provider.LiveSessionResponse, error) {
	*c.bodies = append(*c.bodies, c.name+" "+string(req.Body))
	if c.err != nil {
		return provider.LiveSessionResponse{}, c.err
	}
	return provider.LiveSessionResponse{ID: "s1", Body: []byte(`{}`)}, nil
}

func chatRouter() *Router {
	adapters := map[string]provider.Adapter{
		"openai": fakeAdapter{name: "openai", resp: provider.ChatResponse{Content: "from openai"}},
		"gemini": fakeAdapter{name: "gemini", resp: provider.ChatResponse{Content: "from gemini"}},
	}
	models := map[string]config.ModelRoute{
		"nabu-fast": {
			Primary:  config.Target{Provider: "openai", Model: "gpt-4o-mini"},
			Fallback: []config.Target{{Provider: "gemini", Model: "gemini-2.5-flash"}},
		},
	}
	return New(adapters, models, nil, nil, nil, nil, nil, discardLogger())
}

// The console moves one configured rung to the front; the others stay behind it
// as fallbacks, and the next request is served by the choice.
func TestTheChosenRungAnswersFirst(t *testing.T) {
	r := chatRouter()
	r.SetAliasSettings(map[string]AliasSetting{"nabu-fast": {Primary: "gemini/gemini-2.5-flash"}})

	res, err := r.Chat(context.Background(), "nabu-fast", provider.ChatRequest{})
	if err != nil || res.Provider != "gemini" {
		t.Fatalf("served by %q, %v; want gemini", res.Provider, err)
	}
	if got := r.rungs("nabu-fast", r.models["nabu-fast"]); len(got) != 2 || got[1].Provider != "openai" {
		t.Fatalf("the config's primary should stay behind as a fallback, got %+v", got)
	}

	// A choice naming nothing the alias routes to any more is ignored, never
	// followed somewhere the config does not go.
	r.SetAliasSettings(map[string]AliasSetting{"nabu-fast": {Primary: "anthropic/claude"}})
	if res, _ := r.Chat(context.Background(), "nabu-fast", provider.ChatRequest{}); res.Provider != "openai" {
		t.Fatalf("stale choice served by %q, want the config's openai", res.Provider)
	}
}

func speechRouter(heard *[]string, elevenErr error) *Router {
	adapters := map[string]provider.Adapter{
		"elevenlabs": speaker{fakeAdapter: fakeAdapter{name: "elevenlabs", err: elevenErr}, heard: heard},
		"openai":     speaker{fakeAdapter: fakeAdapter{name: "openai"}, heard: heard},
	}
	audio := map[string]config.ModelRoute{
		"nabu-voice-hd": {
			Primary:  config.Target{Provider: "elevenlabs", Model: "eleven_v3"},
			Fallback: []config.Target{{Provider: "openai", Model: "gpt-4o-mini-tts"}},
			Voice:    "narrator",
		},
	}
	r := New(adapters, nil, nil, audio, nil, nil, nil, discardLogger())
	r.SetVoices(map[string]map[string]string{
		"narrator": {"elevenlabs": "21m00Tcm4TlvDq8ikWAM", "openai": "marin"},
		"bright":   {"openai": "coral"},
	})
	return r
}

// A named voice is a different vendor voice on each rung, so falling back from
// ElevenLabs to OpenAI keeps speaking in the voice that was chosen instead of
// sending OpenAI an ElevenLabs id.
func TestANamedVoiceIsSpokenByWhicheverVendorServes(t *testing.T) {
	var heard []string
	r := speechRouter(&heard, errors.New("quota"))

	if _, err := r.Speech(context.Background(), "nabu-voice-hd", provider.SpeechRequest{Input: "hi", Voice: "narrator"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"elevenlabs=21m00Tcm4TlvDq8ikWAM", "openai=marin"}
	if len(heard) != 2 || heard[0] != want[0] || heard[1] != want[1] {
		t.Fatalf("voices sent = %v, want %v", heard, want)
	}
}

func TestTheVoiceACallGetsWhenItNamesNone(t *testing.T) {
	cases := []struct {
		name    string
		setting AliasSetting
		asked   string
		want    string
	}{
		{"the config's default", AliasSetting{}, "", "elevenlabs=21m00Tcm4TlvDq8ikWAM"},
		{"the console's choice over the config's", AliasSetting{Voice: "bright"}, "", "elevenlabs="},
		{"the caller's own over both", AliasSetting{Voice: "bright"}, "Rachel", "elevenlabs=Rachel"},
	}
	for _, c := range cases {
		var heard []string
		r := speechRouter(&heard, nil)
		r.SetAliasSettings(map[string]AliasSetting{"nabu-voice-hd": c.setting})
		if _, err := r.Speech(context.Background(), "nabu-voice-hd", provider.SpeechRequest{Input: "hi", Voice: c.asked}); err != nil {
			t.Fatal(err)
		}
		// "bright" has no ElevenLabs voice, so ElevenLabs is sent none and
		// speaks in its own default rather than refusing a name it cannot read.
		if len(heard) != 1 || heard[0] != c.want {
			t.Errorf("%s: sent %v, want %s", c.name, heard, c.want)
		}
	}
}

// A live session's voice is set per rung inside the vendor's own body, and the
// rest of the body travels exactly as the caller wrote it.
func TestALiveCallSpeaksInTheNamedVoice(t *testing.T) {
	var bodies []string
	adapters := map[string]provider.Adapter{
		"openai": caller{fakeAdapter: fakeAdapter{name: "openai"}, bodies: &bodies},
	}
	r := New(adapters, nil, nil, nil, nil, nil, nil, discardLogger())
	r.SetLive(map[string]config.ModelRoute{
		"nabu-live": {Primary: config.Target{Provider: "openai", Model: "gpt-realtime"}, Voice: "narrator"},
	})
	r.SetVoices(map[string]map[string]string{"narrator": {"openai": "cedar"}})

	// No voice in the body: the alias's default is written in.
	if _, err := r.LiveSession(context.Background(), "nabu-live", []byte(`{"model":"nabu-live","session":{"type":"realtime","instructions":"hi"}}`)); err != nil {
		t.Fatal(err)
	}
	want := `openai {"model":"nabu-live","session":{"audio":{"output":{"voice":"cedar"}},"instructions":"hi","type":"realtime"}}`
	if bodies[0] != want {
		t.Fatalf("body sent =\n%s\nwant\n%s", bodies[0], want)
	}

	// A vendor voice the caller chose is not a name here, and goes untouched.
	untouched := `{"model":"nabu-live","session":{"audio":{"output":{"voice":"marin"}}}}`
	if _, err := r.LiveSession(context.Background(), "nabu-live", []byte(untouched)); err != nil {
		t.Fatal(err)
	}
	if bodies[1] != "openai "+untouched {
		t.Fatalf("a vendor voice was rewritten: %s", bodies[1])
	}
}
