package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestDeepgramTranscribe(t *testing.T) {
	var (
		gotQuery url.Values
		gotAuth  string
		gotType  string
		gotBody  []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		gotAuth = r.Header.Get("Authorization")
		gotType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		io.WriteString(w, `{"metadata":{"duration":3.5},"results":{"channels":[{"detected_language":"fa",
			"alternatives":[{"transcript":"  سلام دنیا  ","words":[
				{"word":"salam","punctuated_word":"سلام","start":0.1,"end":0.5,"speaker":0},
				{"word":"donya","punctuated_word":"دنیا.","start":0.5,"end":1.2,"speaker":0}]}]}]}}`)
	}))
	defer srv.Close()

	a := NewDeepgramAdapter("deepgram", srv.URL+"/v1", "dg-secret")
	out, err := a.Transcribe(context.Background(), TranscriptionRequest{
		Model: "nova-3", Audio: []byte("OggS-bytes"), Filename: "voice.ogg",
	})
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if out.Text != "سلام دنیا" || out.Language != "fa" || out.Duration != 3.5 {
		t.Errorf("out = %+v", out)
	}
	// "Token", not "Bearer": a bearer token 401s with a body that never says so.
	if gotAuth != "Token dg-secret" {
		t.Errorf("authorization = %q", gotAuth)
	}
	// The body's MIME type is the only thing that tells Deepgram the container.
	if gotType != "audio/ogg" {
		t.Errorf("content-type = %q, want the type derived from the filename", gotType)
	}
	if string(gotBody) != "OggS-bytes" {
		t.Errorf("body = %q; the audio is the request body, not a form part", gotBody)
	}
	if gotQuery.Get("model") != "nova-3" || gotQuery.Get("smart_format") != "true" {
		t.Errorf("query = %v", gotQuery)
	}
	// With no language named, guessing English on Persian audio returns
	// confident nonsense; detection must be on instead.
	if gotQuery.Get("detect_language") != "true" || gotQuery.Get("language") != "" {
		t.Errorf("language handling = %v", gotQuery)
	}
	// punctuated_word is the readable form; the bare word is the fallback.
	if len(out.Segments) != 1 || out.Segments[0].Text != "سلام دنیا." {
		t.Errorf("segments = %+v", out.Segments)
	}
}

func TestDeepgramDiarizationSplitsSpeakers(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		io.WriteString(w, `{"metadata":{"duration":2},"results":{"channels":[{"alternatives":[{
			"transcript":"one two three","words":[
			{"punctuated_word":"One","start":0,"end":0.4,"speaker":0},
			{"punctuated_word":"two.","start":0.4,"end":0.9,"speaker":0},
			{"punctuated_word":"Three?","start":1.2,"end":1.8,"speaker":1}]}]}]}}`)
	}))
	defer srv.Close()

	a := NewDeepgramAdapter("deepgram", srv.URL+"/v1", "k")
	out, err := a.Transcribe(context.Background(), TranscriptionRequest{
		Model: "nova-3", Audio: []byte("x"), Filename: "a.wav", Language: "en",
		Granularities: []string{"word"},
	})
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if gotQuery.Get("diarize") != "true" || gotQuery.Get("language") != "en" {
		t.Errorf("query = %v", gotQuery)
	}
	if len(out.Segments) != 2 {
		t.Fatalf("segments = %d, want one per speaker: %+v", len(out.Segments), out.Segments)
	}
	if out.Segments[0].Text != "One two." || out.Segments[1].Start != 1.2 {
		t.Errorf("segments = %+v", out.Segments)
	}
}

// Deepgram answers 200 with an empty transcript when it heard no speech.
// Treating that as success would erase an archive one item at a time.
func TestDeepgramEmptyIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"results":{"channels":[{"alternatives":[{"transcript":"","words":[]}]}]}}`)
	}))
	defer srv.Close()

	a := NewDeepgramAdapter("deepgram", srv.URL+"/v1", "k")
	_, err := a.Transcribe(context.Background(), TranscriptionRequest{
		Model: "nova-3", Audio: []byte("x"), Filename: "a.wav",
	})
	if err == nil || !strings.Contains(err.Error(), "empty transcription") {
		t.Fatalf("err = %v", err)
	}
}
