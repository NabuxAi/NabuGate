package provider

import (
	"bytes"
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
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
	u := MeasureEleven(http.MethodPost, "/v1/text-to-speech/21m00Tcm4TlvDq8ikWAM", "application/json", body, http.StatusOK, "audio/mpeg", nil)
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
	u := MeasureEleven(http.MethodPost, "/v1/text-to-dialogue", "application/json", body, http.StatusOK, "audio/mpeg", nil)
	if u.Characters != 7 || u.Model != "eleven_multilingual_v2" {
		t.Fatalf("usage = %+v", u)
	}
}

func TestMeasureElevenUpstreamErrorIsNotCharged(t *testing.T) {
	body := []byte(`{"text":"hello","model_id":"eleven_v3"}`)
	u := MeasureEleven(http.MethodPost, "/v1/text-to-speech/v", "application/json", body, http.StatusUnprocessableEntity, "application/json", []byte(`{"detail":{"message":"bad"}}`))
	if u.Charge {
		t.Fatalf("error was charged: %+v", u)
	}
}

func TestMeasureElevenTranscriptionUsesResponseDuration(t *testing.T) {
	resp := []byte(`{"text":"hi","audio_duration_secs":12.5,"language_code":"fa"}`)
	u := MeasureEleven(http.MethodPost, "/v1/speech-to-text", "multipart/form-data", nil, http.StatusOK, "application/json", resp)
	if !u.Charge || !u.Measured || u.Seconds != 12.5 || u.Model != "scribe_v2" {
		t.Fatalf("usage = %+v", u)
	}
}

func TestMeasureElevenDubbingBillsOnCreateAndNotOnPendingPoll(t *testing.T) {
	created := MeasureEleven(http.MethodPost, "/v1/dubbing", "multipart/form-data", nil, http.StatusOK, "application/json",
		[]byte(`{"dubbing_id":"dub_1","expected_duration_sec":90}`))
	if !created.Charge || created.Seconds != 90 || created.JobID != "dub_1" || created.Model != "dubbing" {
		t.Fatalf("create = %+v", created)
	}
	pending := MeasureEleven(http.MethodGet, "/v1/dubbing/dub_1", "", nil, http.StatusOK, "application/json",
		[]byte(`{"dubbing_id":"dub_1","status":"dubbing"}`))
	if pending.Charge {
		t.Fatalf("pending poll charged: %+v", pending)
	}
	done := MeasureEleven(http.MethodGet, "/v1/dubbing/dub_1", "", nil, http.StatusOK, "application/json",
		[]byte(`{"dubbing_id":"dub_1","status":"dubbed","media_metadata":{"duration":90}}`))
	if !done.Charge || done.Seconds != 90 || done.JobID != "dub_1" {
		t.Fatalf("done = %+v", done)
	}
}

func TestMeasureElevenProjectDubbingDefaultsToV2(t *testing.T) {
	u := MeasureEleven(http.MethodPost, "/v1/dubbing/project", "multipart/form-data", nil, http.StatusOK, "application/json",
		[]byte(`{"project_id":"p1","status":"queued"}`))
	// Queued, and the body did not name a duration. A caller header must not
	// invent one. The model default is still v2 so the later poll prices right.
	if u.Model != "dubbing_v2" || u.Charge {
		t.Fatalf("usage = %+v", u)
	}
}

func TestMeasureElevenVideoLipsync(t *testing.T) {
	body := []byte(`{"model_id":"sync-lipsync-v3","duration_secs":8,"audio":{"type":"asset","asset_id":"a"}}`)
	u := MeasureEleven(http.MethodPost, "/v1/flows/video", "application/json", body, http.StatusOK, "application/json",
		[]byte(`{"id":"gen_1","status":"pending"}`))
	if !u.Charge || !u.Measured || u.Model != "sync-lipsync-v3" || u.Seconds != 8 || u.JobID != "gen_1" {
		t.Fatalf("create = %+v", u)
	}
	poll := MeasureEleven(http.MethodGet, "/v1/flows/video/gen_1", "", nil, http.StatusOK, "application/json",
		[]byte(`{"id":"gen_1","status":"generating"}`))
	if poll.Charge {
		t.Fatalf("generating poll charged: %+v", poll)
	}
	// No duration on the completed payload: the create already held it.
	done := MeasureEleven(http.MethodGet, "/v1/flows/video/gen_1", "", nil, http.StatusOK, "application/json",
		[]byte(`{"id":"gen_1","status":"completed","content_url":"https://storage.example/v.mp4"}`))
	if done.Charge {
		t.Fatalf("completed poll without duration charged: %+v", done)
	}
}

func TestMeasureElevenVideoWithoutDurationWaits(t *testing.T) {
	body := []byte(`{"model_id":"sync-lipsync-v3","image":{"type":"asset","asset_id":"i"},"audio":{"type":"asset","asset_id":"a"}}`)
	u := MeasureEleven(http.MethodPost, "/v1/flows/video", "application/json", body, http.StatusOK, "application/json",
		[]byte(`{"id":"gen_2","status":"pending"}`))
	if u.Charge {
		t.Fatalf("unmeasured async create was charged: %+v", u)
	}
}

func TestMeasureElevenVoiceChangerIsUnpricedUntilTheFileIsRead(t *testing.T) {
	u := MeasureEleven(http.MethodPost, "/v1/voice-changer/voice", "multipart/form-data", nil, http.StatusOK, "audio/mpeg", []byte("audio"))
	if !u.Charge || u.Measured || u.Seconds != 0 || u.Model != "eleven_multilingual_sts_v2" {
		t.Fatalf("header-free voice changer = %+v", u)
	}
	u.ApplyUpload(2, 0, false)
	if !u.Measured || u.Seconds != 2 {
		t.Fatalf("upload duration not applied: %+v", u)
	}
	known := MeasureEleven(http.MethodPost, "/v1/speech-to-text", "multipart/form-data", nil, http.StatusOK, "application/json",
		[]byte(`{"audio_duration_secs":9}`))
	known.ApplyUpload(1, 0, false)
	if known.Seconds != 9 {
		t.Fatalf("upload overrode the vendor duration: %+v", known)
	}
}

func makeWAV(seconds, rate, channels, bits int) []byte {
	dataSize := seconds * rate * channels * bits / 8
	byteRate := rate * channels * bits / 8
	block := channels * bits / 8
	buf := make([]byte, 44+dataSize)
	copy(buf[0:], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:], uint32(36+dataSize))
	copy(buf[8:], "WAVE")
	copy(buf[12:], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:], 16)
	binary.LittleEndian.PutUint16(buf[20:], 1)
	binary.LittleEndian.PutUint16(buf[22:], uint16(channels))
	binary.LittleEndian.PutUint32(buf[24:], uint32(rate))
	binary.LittleEndian.PutUint32(buf[28:], uint32(byteRate))
	binary.LittleEndian.PutUint16(buf[32:], uint16(block))
	binary.LittleEndian.PutUint16(buf[34:], uint16(bits))
	copy(buf[36:], "data")
	binary.LittleEndian.PutUint32(buf[40:], uint32(dataSize))
	return buf
}

func TestMeasureElevenSoundAndMusicDefaultsAreEstimated(t *testing.T) {
	sfx := MeasureEleven(http.MethodPost, "/v1/sound-generation", "application/json", []byte(`{"text":"rain"}`), http.StatusOK, "audio/mpeg", nil)
	if sfx.Seconds != 10 || !sfx.Estimated || sfx.Model != "eleven_text_to_sound_v2" {
		t.Fatalf("sfx = %+v", sfx)
	}
	given := MeasureEleven(http.MethodPost, "/v1/sound-generation", "application/json", []byte(`{"text":"rain","duration_seconds":4}`), http.StatusOK, "audio/mpeg", nil)
	if given.Seconds != 4 || given.Estimated {
		t.Fatalf("explicit sfx = %+v", given)
	}
	music := MeasureEleven(http.MethodPost, "/v1/music/stream", "application/json", []byte(`{"prompt":"piano","music_length_ms":8000}`), http.StatusOK, "audio/mpeg", nil)
	if music.Seconds != 8 || music.Model != "music_v1" {
		t.Fatalf("music = %+v", music)
	}
	plan := MeasureEleven(http.MethodPost, "/v1/music/plan", "application/json", []byte(`{"prompt":"piano"}`), http.StatusOK, "application/json", []byte(`{}`))
	if plan.Charge {
		t.Fatalf("plan was billed: %+v", plan)
	}
}

func TestMeasureElevenVoiceDesignFloorAndSaveIsFree(t *testing.T) {
	design := MeasureEleven(http.MethodPost, "/v1/text-to-voice/design", "application/json",
		[]byte(`{"voice_description":"warm","auto_generate_text":true}`), http.StatusOK, "application/json", []byte(`{"previews":[]}`))
	if design.Characters != 1000 || !design.Estimated || !design.Charge {
		t.Fatalf("design = %+v", design)
	}
	save := MeasureEleven(http.MethodPost, "/v1/text-to-voice", "application/json",
		[]byte(`{"voice_name":"Ada","generated_voice_id":"g1"}`), http.StatusOK, "application/json", []byte(`{"voice_id":"v"}`))
	if save.Charge {
		t.Fatalf("saving a preview was billed: %+v", save)
	}
}

func TestMeasureElevenForcedAlignmentUsesWordEnds(t *testing.T) {
	resp := []byte(`{"words":[{"text":"hi","start":0.1,"end":0.4},{"text":"there","start":0.5,"end":1.2}],"loss":0.1}`)
	u := MeasureEleven(http.MethodPost, "/v1/forced-alignment", "multipart/form-data", nil, http.StatusOK, "application/json", resp)
	if !u.Charge || u.Seconds != 1.2 || u.Model != "forced-alignment" {
		t.Fatalf("usage = %+v", u)
	}
}

func TestAudioDurationWAVAndMP3(t *testing.T) {
	wav := makeWAV(1, 8000, 1, 16) // 1 second, 8 kHz, mono, 16-bit
	secs, estimated, ok := audioDuration(bytes.NewReader(wav))
	if !ok || estimated || secs < 0.99 || secs > 1.01 {
		t.Fatalf("wav duration = %v estimated=%v ok=%v", secs, estimated, ok)
	}
	// MPEG1 layer III, 128 kbps, 44100 Hz. One second is 16000 bytes.
	mp3 := make([]byte, 16000)
	mp3[0], mp3[1], mp3[2], mp3[3] = 0xFF, 0xFB, 0x90, 0x00
	secs, estimated, ok = audioDuration(bytes.NewReader(mp3))
	if !ok || !estimated || secs < 0.99 || secs > 1.01 {
		t.Fatalf("mp3 duration = %v estimated=%v ok=%v", secs, estimated, ok)
	}
}

func TestProxyKeepsBaseURLPathPrefix(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	p := newElevenLabsProxy(srv.URL+"/vendor/elevenlabs/v1", "k")
	resp, err := p.Forward(context.Background(), http.MethodGet, "/v1/voices", "", nil, nil, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got != "/vendor/elevenlabs/v1/voices" {
		t.Fatalf("path = %q", got)
	}
}
