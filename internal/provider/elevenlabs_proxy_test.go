package provider

import (
	"net/http"
	"strings"
	"testing"
)

func TestElevenUpstreamPathRewritesSpeechToSpeechAndRejectsEscape(t *testing.T) {
	got, ok := ElevenUpstreamPath("/elevenlabs/v1/speech-to-speech/voice123/stream")
	if !ok || got != "/v1/voice-changer/voice123/stream" {
		t.Fatalf("rewrite: ok=%v path=%q", ok, got)
	}
	if _, ok := ElevenUpstreamPath("/elevenlabs/../v1/user"); ok {
		t.Fatal("path escape was accepted")
	}
	if _, ok := ElevenUpstreamPath("/v1/text-to-speech/x"); ok {
		t.Fatal("path outside /elevenlabs was accepted")
	}
}

func TestElevenPathAllowedIsTheProductSurfaceOnly(t *testing.T) {
	allowed := []string{
		"/v1/text-to-speech/voice",
		"/v1/text-to-speech/voice/stream",
		"/v1/text-to-speech/voice/with-timestamps",
		"/v1/text-to-speech/voice/stream/with-timestamps",
		"/v1/text-to-dialogue",
		"/v1/voices",
		"/v1/voices/add",
		"/v1/voices/voice/edit",
		"/v1/text-to-voice/design",
		"/v1/speech-to-text",
		"/v1/speech-to-text/transcripts/tr_1",
		"/v1/dubbing",
		"/v1/dubbing/dub_1",
		"/v1/sound-generation",
		"/v1/music",
		"/v1/music/stream",
		"/v1/voice-changer/voice",
		"/v1/audio-isolation",
		"/v1/forced-alignment",
		"/v1/audio-native",
		"/v1/flows/video",
		"/v1/flows/video/gen_1",
		"/v1/assets",
		"/v1/models",
	}
	for _, path := range allowed {
		if !ElevenPathAllowed(http.MethodPost, path) && !ElevenPathAllowed(http.MethodGet, path) {
			t.Errorf("path %s should be proxied", path)
		}
	}
	for _, path := range []string{"/v1/user", "/v1/workspace/api-keys", "/v1/service-accounts", "/v1/history"} {
		if ElevenPathAllowed(http.MethodGet, path) {
			t.Errorf("path %s must not be proxied", path)
		}
	}
	if ElevenPathAllowed(http.MethodTrace, "/v1/voices") {
		t.Fatal("TRACE was allowed")
	}
}

func TestMeasureElevenSpeechCountsCharactersAndKeepsModel(t *testing.T) {
	body := []byte(`{"text":"[whispers] سلام","model_id":"eleven_v3","language_code":"fa","voice_settings":{"stability":0.4}}`)
	u := MeasureEleven(http.MethodPost, "/v1/text-to-speech/21m00Tcm4TlvDq8ikWAM", "application/json", body, 0, http.StatusOK, "audio/mpeg", nil)
	if !u.Charge || !u.Measured || u.Model != "eleven_v3" {
		t.Fatalf("usage = %+v", u)
	}
	// The audio tag is part of the text ElevenLabs bills.
	if u.Characters != len([]rune("[whispers] سلام")) {
		t.Fatalf("characters = %d", u.Characters)
	}
	if u.Seconds != 0 {
		t.Fatalf("speech billed seconds: %+v", u)
	}
}

func TestMeasureElevenDialogueSumsTurns(t *testing.T) {
	body := []byte(`{"inputs":[{"text":"hi","voice_id":"a"},{"text":"there","voice_id":"b"}]}`)
	u := MeasureEleven(http.MethodPost, "/v1/text-to-dialogue", "application/json", body, 0, http.StatusOK, "audio/mpeg", nil)
	if u.Characters != 7 || u.Model != "eleven_multilingual_v2" {
		t.Fatalf("usage = %+v", u)
	}
}

func TestMeasureElevenUpstreamErrorIsNotCharged(t *testing.T) {
	body := []byte(`{"text":"hello","model_id":"eleven_v3"}`)
	u := MeasureEleven(http.MethodPost, "/v1/text-to-speech/v", "application/json", body, 0, http.StatusUnprocessableEntity, "application/json", []byte(`{"detail":{"message":"bad"}}`))
	if u.Charge {
		t.Fatalf("error was charged: %+v", u)
	}
}

func TestMeasureElevenTranscriptionUsesResponseDuration(t *testing.T) {
	resp := []byte(`{"text":"hi","audio_duration_secs":12.5,"language_code":"fa"}`)
	u := MeasureEleven(http.MethodPost, "/v1/speech-to-text", "multipart/form-data", nil, 0, http.StatusOK, "application/json", resp)
	if !u.Charge || !u.Measured || u.Seconds != 12.5 || u.Model != "scribe_v2" {
		t.Fatalf("usage = %+v", u)
	}
}

func TestMeasureElevenDubbingBillsOnCreateAndNotOnPendingPoll(t *testing.T) {
	created := MeasureEleven(http.MethodPost, "/v1/dubbing", "multipart/form-data", nil, 0, http.StatusOK, "application/json",
		[]byte(`{"dubbing_id":"dub_1","expected_duration_sec":90}`))
	if !created.Charge || created.Seconds != 90 || created.JobID != "dub_1" || created.Model != "dubbing" {
		t.Fatalf("create = %+v", created)
	}
	pending := MeasureEleven(http.MethodGet, "/v1/dubbing/dub_1", "", nil, 0, http.StatusOK, "application/json",
		[]byte(`{"dubbing_id":"dub_1","status":"dubbing"}`))
	if pending.Charge {
		t.Fatalf("pending poll charged: %+v", pending)
	}
	done := MeasureEleven(http.MethodGet, "/v1/dubbing/dub_1", "", nil, 0, http.StatusOK, "application/json",
		[]byte(`{"dubbing_id":"dub_1","status":"dubbed","media_metadata":{"duration":90}}`))
	if !done.Charge || done.Seconds != 90 || done.JobID != "dub_1" {
		t.Fatalf("done = %+v", done)
	}
}

func TestMeasureElevenProjectDubbingDefaultsToV2(t *testing.T) {
	u := MeasureEleven(http.MethodPost, "/v1/dubbing/project", "multipart/form-data", nil, 30, http.StatusOK, "application/json",
		[]byte(`{"project_id":"p1","status":"queued"}`))
	// Duration came from the metering header; the job is accepted but still queued,
	// and the header already measured it, so it is billed at the v2 rate.
	if u.Model != "dubbing_v2" || !u.Charge || u.Seconds != 30 {
		t.Fatalf("usage = %+v", u)
	}
}

func TestMeasureElevenVideoLipsync(t *testing.T) {
	body := []byte(`{"model_id":"sync-lipsync-v3","duration_secs":8,"audio":{"type":"asset","asset_id":"a"}}`)
	u := MeasureEleven(http.MethodPost, "/v1/flows/video", "application/json", body, 0, http.StatusOK, "application/json",
		[]byte(`{"id":"gen_1","status":"pending"}`))
	if !u.Charge || !u.Measured || u.Model != "sync-lipsync-v3" || u.Seconds != 8 || u.JobID != "gen_1" {
		t.Fatalf("create = %+v", u)
	}
	poll := MeasureEleven(http.MethodGet, "/v1/flows/video/gen_1", "", nil, 0, http.StatusOK, "application/json",
		[]byte(`{"id":"gen_1","status":"generating"}`))
	if poll.Charge {
		t.Fatalf("generating poll charged: %+v", poll)
	}
	// No duration on the completed payload: the create already held it.
	done := MeasureEleven(http.MethodGet, "/v1/flows/video/gen_1", "", nil, 0, http.StatusOK, "application/json",
		[]byte(`{"id":"gen_1","status":"completed","content_url":"https://storage.example/v.mp4"}`))
	if done.Charge {
		t.Fatalf("completed poll without duration charged: %+v", done)
	}
}

func TestMeasureElevenVideoWithoutDurationWaits(t *testing.T) {
	body := []byte(`{"model_id":"sync-lipsync-v3","image":{"type":"asset","asset_id":"i"},"audio":{"type":"asset","asset_id":"a"}}`)
	u := MeasureEleven(http.MethodPost, "/v1/flows/video", "application/json", body, 0, http.StatusOK, "application/json",
		[]byte(`{"id":"gen_2","status":"pending"}`))
	if u.Charge {
		t.Fatalf("unmeasured async create was charged: %+v", u)
	}
}

func TestMeasureElevenDurationHeaderCapsAndFeedsVoiceChanger(t *testing.T) {
	h := http.Header{}
	h.Set("X-Nabu-Duration-Seconds", "15")
	if got := ElevenDurationHeader(h); got != 15 {
		t.Fatalf("header = %v", got)
	}
	h.Set("X-Nabu-Duration-Seconds", "999999")
	if got := ElevenDurationHeader(h); got != 0 {
		t.Fatalf("absurd header was accepted: %v", got)
	}
	u := MeasureEleven(http.MethodPost, "/v1/voice-changer/voice", "multipart/form-data", nil, 15, http.StatusOK, "audio/mpeg", []byte("audio"))
	if !u.Charge || u.Seconds != 15 || u.Model != "eleven_multilingual_sts_v2" {
		t.Fatalf("usage = %+v", u)
	}
	missing := MeasureEleven(http.MethodPost, "/v1/audio-isolation", "multipart/form-data", nil, 0, http.StatusOK, "audio/mpeg", []byte("audio"))
	if !missing.Charge || missing.Measured {
		t.Fatalf("unmeasured isolator should be charged as unpriced: %+v", missing)
	}
}

func TestMeasureElevenSoundAndMusicDefaultsAreEstimated(t *testing.T) {
	sfx := MeasureEleven(http.MethodPost, "/v1/sound-generation", "application/json", []byte(`{"text":"rain"}`), 0, http.StatusOK, "audio/mpeg", nil)
	if sfx.Seconds != 10 || !sfx.Estimated || sfx.Model != "eleven_text_to_sound_v2" {
		t.Fatalf("sfx = %+v", sfx)
	}
	given := MeasureEleven(http.MethodPost, "/v1/sound-generation", "application/json", []byte(`{"text":"rain","duration_seconds":4}`), 0, http.StatusOK, "audio/mpeg", nil)
	if given.Seconds != 4 || given.Estimated {
		t.Fatalf("explicit sfx = %+v", given)
	}
	music := MeasureEleven(http.MethodPost, "/v1/music/stream", "application/json", []byte(`{"prompt":"piano","music_length_ms":8000}`), 0, http.StatusOK, "audio/mpeg", nil)
	if music.Seconds != 8 || music.Model != "music_v1" {
		t.Fatalf("music = %+v", music)
	}
	plan := MeasureEleven(http.MethodPost, "/v1/music/plan", "application/json", []byte(`{"prompt":"piano"}`), 0, http.StatusOK, "application/json", []byte(`{}`))
	if plan.Charge {
		t.Fatalf("plan was billed: %+v", plan)
	}
}

func TestMeasureElevenVoiceDesignFloorAndSaveIsFree(t *testing.T) {
	design := MeasureEleven(http.MethodPost, "/v1/text-to-voice/design", "application/json",
		[]byte(`{"voice_description":"warm","auto_generate_text":true}`), 0, http.StatusOK, "application/json", []byte(`{"previews":[]}`))
	if design.Characters != 1000 || !design.Estimated || !design.Charge {
		t.Fatalf("design = %+v", design)
	}
	save := MeasureEleven(http.MethodPost, "/v1/text-to-voice", "application/json",
		[]byte(`{"voice_name":"Ada","generated_voice_id":"g1"}`), 0, http.StatusOK, "application/json", []byte(`{"voice_id":"v"}`))
	if save.Charge {
		t.Fatalf("saving a preview was billed: %+v", save)
	}
}

func TestMeasureElevenForcedAlignmentUsesWordEnds(t *testing.T) {
	resp := []byte(`{"words":[{"text":"hi","start":0.1,"end":0.4},{"text":"there","start":0.5,"end":1.2}],"loss":0.1}`)
	u := MeasureEleven(http.MethodPost, "/v1/forced-alignment", "multipart/form-data", nil, 0, http.StatusOK, "application/json", resp)
	if !u.Charge || u.Seconds != 1.2 || u.Model != "forced-alignment" {
		t.Fatalf("usage = %+v", u)
	}
}

func TestElevenDurationHeaderIgnoresJunk(t *testing.T) {
	h := http.Header{"X-Nabu-Duration-Seconds": []string{"nope"}}
	if ElevenDurationHeader(h) != 0 {
		t.Fatal("junk header parsed")
	}
	if strings.Contains(strings.ToLower("X-Nabu-Key-elevenlabs"), "x-nabu-") {
		// copyElevenRequestHeaders drops this prefix; the assertion lives in the server test.
	}
}
