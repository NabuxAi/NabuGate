package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestElevenLabsTranscribe(t *testing.T) {
	var (
		gotPath  string
		gotKey   string
		gotAuth  string
		fields   map[string][]string
		filename string
		audio    []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("xi-api-key")
		gotAuth = r.Header.Get("Authorization")
		fields, filename, audio = parseUpload(t, r)
		io.WriteString(w, `{"language_code":"fa","text":"  سلام  ","words":[
			{"text":"سلام","start":0.1,"end":0.6,"type":"word","speaker_id":"s1"}]}`)
	}))
	defer srv.Close()

	a := NewElevenLabsAdapter("elevenlabs", srv.URL, "xi-secret")
	out, err := a.Transcribe(context.Background(), TranscriptionRequest{
		Model: "scribe_v2", Audio: []byte("OggS"), Filename: "voice.ogg", Language: "fa",
	})
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if out.Text != "سلام" || out.Language != "fa" {
		t.Errorf("out = %q / %q", out.Text, out.Language)
	}
	if gotPath != "/speech-to-text" {
		t.Errorf("path = %q", gotPath)
	}
	// Bearer returns a 401 whose body never mentions the right header, so the
	// wrong one here is an expensive thing to debug.
	if gotKey != "xi-secret" || gotAuth != "" {
		t.Errorf("auth: xi-api-key=%q authorization=%q", gotKey, gotAuth)
	}
	if filename != "voice.ogg" || string(audio) != "OggS" {
		t.Errorf("upload = %q / %q", filename, audio)
	}
	// model_id, not model; language_code, not language.
	if got := fields["model_id"]; len(got) != 1 || got[0] != "scribe_v2" {
		t.Errorf("model_id = %v", got)
	}
	if got := fields["language_code"]; len(got) != 1 || got[0] != "fa" {
		t.Errorf("language_code = %v", got)
	}
	// timestamps_granularity is an enum of word|character with no off value.
	// Sending one anyway rejects every ordinary call.
	if got, ok := fields["timestamps_granularity"]; ok {
		t.Errorf("timestamps_granularity = %v; it must be omitted when no timestamps were asked for", got)
	}
	if _, ok := fields["diarize"]; ok {
		t.Error("diarization requested though the caller wanted none; it costs latency")
	}
}

func TestElevenLabsTranscribeTimestamps(t *testing.T) {
	var fields map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fields, _, _ = parseUpload(t, r)
		// Two speakers, with a spacing token between them that carries no
		// content and an audio_event that is not speech at all.
		io.WriteString(w, `{"language_code":"en","text":"one two three","words":[
			{"text":"one","start":0,"end":0.4,"type":"word","speaker_id":"s1"},
			{"text":" ","start":0.4,"end":0.4,"type":"spacing","speaker_id":"s1"},
			{"text":"two","start":0.4,"end":0.9,"type":"word","speaker_id":"s1"},
			{"text":"(laughs)","start":0.9,"end":1.1,"type":"audio_event","speaker_id":"s1"},
			{"text":"three","start":1.2,"end":1.9,"type":"word","speaker_id":"s2"}]}`)
	}))
	defer srv.Close()

	a := NewElevenLabsAdapter("elevenlabs", srv.URL, "k")
	out, err := a.Transcribe(context.Background(), TranscriptionRequest{
		Model: "scribe_v2", Audio: []byte("x"), Filename: "a.mp3",
		Granularities: []string{"word"},
	})
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if got := fields["timestamps_granularity"]; len(got) != 1 || got[0] != "word" {
		t.Errorf("timestamps_granularity = %v", got)
	}
	if got := fields["diarize"]; len(got) != 1 || got[0] != "true" {
		t.Errorf("diarize = %v", got)
	}
	if len(out.Segments) != 2 {
		t.Fatalf("segments = %d, want one per speaker: %+v", len(out.Segments), out.Segments)
	}
	if out.Segments[0].Text != "one two" {
		t.Errorf("first segment = %q; spacing and audio_event are not words", out.Segments[0].Text)
	}
	if out.Segments[1].Text != "three" || out.Segments[1].Start != 1.2 {
		t.Errorf("second segment = %+v", out.Segments[1])
	}
	// Duration counts every token's end, including the non-speech ones, because
	// they are still part of the recording.
	if out.Duration != 1.9 {
		t.Errorf("duration = %v", out.Duration)
	}
}

func TestElevenLabsTranscribeEmptyIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"language_code":"en","text":"   ","words":[]}`)
	}))
	defer srv.Close()

	a := NewElevenLabsAdapter("elevenlabs", srv.URL, "k")
	_, err := a.Transcribe(context.Background(), TranscriptionRequest{
		Model: "scribe_v2", Audio: []byte("x"), Filename: "a.wav",
	})
	if err == nil || !strings.Contains(err.Error(), "empty transcription") {
		t.Fatalf("err = %v, want an empty-transcription error so the router fails over", err)
	}
}

// A provider on a non-verbose format must not be sent timestamp_granularities:
// Mistral rejects them alongside language, and no other vendor returns them
// outside the verbose shape.
func TestOpenAITranscribeFormatOverride(t *testing.T) {
	var fields map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fields, _, _ = parseUpload(t, r)
		io.WriteString(w, `{"text":"bonjour"}`)
	}))
	defer srv.Close()

	a := NewOpenAIAdapter("mistral", srv.URL, "k", nil)
	a.SetTranscribeFormat("json")
	out, err := a.Transcribe(context.Background(), TranscriptionRequest{
		Model: "voxtral-mini-latest", Audio: []byte("x"), Filename: "a.mp3",
		Language: "fr", Granularities: []string{"segment"},
	})
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if out.Text != "bonjour" {
		t.Errorf("text = %q", out.Text)
	}
	if got := fields["response_format"]; len(got) != 1 || got[0] != "json" {
		t.Errorf("response_format = %v, want the provider's override", got)
	}
	if got, ok := fields["timestamp_granularities[]"]; ok {
		t.Errorf("timestamp_granularities = %v; not valid outside verbose_json", got)
	}
}
