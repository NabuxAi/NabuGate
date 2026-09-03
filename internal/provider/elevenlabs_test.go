package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestElevenLabsSpeechUsesVoiceInPathAndXiKeyHeader(t *testing.T) {
	var gotPath, gotKey, gotAuth, gotFormat string
	var gotBody elevenSpeechRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("xi-api-key")
		gotAuth = r.Header.Get("Authorization")
		gotFormat = r.URL.Query().Get("output_format")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("ID3fake-audio"))
	}))
	defer srv.Close()

	a := NewElevenLabsAdapter("elevenlabs", srv.URL, "secret")
	out, err := a.Speech(context.Background(), SpeechRequest{
		Model: "eleven_v3", Input: "سلام", Voice: "21m00Tcm4TlvDq8ikWAM", Format: "mp3",
	})
	if err != nil {
		t.Fatalf("Speech: %v", err)
	}
	if want := "/text-to-speech/21m00Tcm4TlvDq8ikWAM"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if gotKey != "secret" {
		t.Errorf("xi-api-key = %q, want %q", gotKey, "secret")
	}
	// A Bearer header is the mistake this adapter exists to avoid.
	if gotAuth != "" {
		t.Errorf("Authorization = %q, want it unset", gotAuth)
	}
	if gotFormat != "mp3_44100_128" {
		t.Errorf("output_format = %q", gotFormat)
	}
	if gotBody.ModelID != "eleven_v3" || gotBody.Text != "سلام" {
		t.Errorf("body = %+v", gotBody)
	}
	if string(out.Audio) != "ID3fake-audio" || out.ContentType != "audio/mpeg" {
		t.Errorf("response = %q / %q", out.Audio, out.ContentType)
	}
}

func TestElevenLabsResolvesVoiceNameThroughVoicesList(t *testing.T) {
	var speechPath string
	calls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/voices" {
			calls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"voices":[{"voice_id":"abcDEF1234567890xyz","name":"Charlotte"}]}`))
			return
		}
		speechPath = r.URL.Path
		_, _ = w.Write([]byte("audio"))
	}))
	defer srv.Close()

	a := NewElevenLabsAdapter("elevenlabs", srv.URL, "k")
	for i := 0; i < 2; i++ {
		if _, err := a.Speech(context.Background(), SpeechRequest{Input: "x", Voice: "charlotte"}); err != nil {
			t.Fatalf("Speech: %v", err)
		}
	}
	if !strings.HasSuffix(speechPath, "abcDEF1234567890xyz") {
		t.Errorf("speech path = %q, name was not resolved", speechPath)
	}
	// The mapping only changes when someone edits it in the console, so the
	// second request must not pay for another lookup.
	if calls != 1 {
		t.Errorf("/voices called %d times, want 1 (cached)", calls)
	}
}

func TestElevenLabsUnknownVoiceFallsBackInsteadOfFailing(t *testing.T) {
	var speechPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/voices" {
			_, _ = w.Write([]byte(`{"voices":[]}`))
			return
		}
		speechPath = r.URL.Path
		_, _ = w.Write([]byte("audio"))
	}))
	defer srv.Close()

	a := NewElevenLabsAdapter("elevenlabs", srv.URL, "k")
	if _, err := a.Speech(context.Background(), SpeechRequest{Input: "x", Voice: "Nobody"}); err != nil {
		t.Fatalf("Speech: %v", err)
	}
	if !strings.HasSuffix(speechPath, defaultVoiceID) {
		t.Errorf("speech path = %q, want the default voice", speechPath)
	}
}

func TestElevenLabsEmptyBodyIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK) // 200, no bytes
	}))
	defer srv.Close()

	a := NewElevenLabsAdapter("elevenlabs", srv.URL, "k")
	if _, err := a.Speech(context.Background(), SpeechRequest{Input: "x"}); err == nil {
		t.Fatal("expected an error for an empty 200")
	}
}

func TestElevenLabsSurfacesUpstreamMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":{"status":"invalid_api_key","message":"Invalid API key"}}`))
	}))
	defer srv.Close()

	a := NewElevenLabsAdapter("elevenlabs", srv.URL, "k")
	_, err := a.Speech(context.Background(), SpeechRequest{Input: "x"})
	if err == nil || !strings.Contains(err.Error(), "Invalid API key") {
		t.Fatalf("err = %v, want the upstream message", err)
	}
}

func TestElevenLabsPCMIsLabelledAsPCM(t *testing.T) {
	// Callers asking for "wav" get headerless PCM from ElevenLabs; mislabelling
	// it audio/wav writes a file nothing can play.
	q, ct := elevenOutputFormat("wav")
	if q != "pcm_24000" || !strings.Contains(ct, "L16") {
		t.Errorf("wav -> %q / %q", q, ct)
	}
}
