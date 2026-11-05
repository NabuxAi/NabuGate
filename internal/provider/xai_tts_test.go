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

func TestXAISpeechSendsGrokShape(t *testing.T) {
	var gotPath, gotAuth, gotMethod string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth, gotMethod = r.URL.Path, r.Header.Get("Authorization"), r.Method
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("ID3grok"))
	}))
	defer srv.Close()

	a := NewXAITTSAdapter("xai-voice", srv.URL+"/v1", "secret")
	out, err := a.Speech(context.Background(), SpeechRequest{Model: "grok-tts", Input: " Hello there ", Voice: "Rex"})
	if err != nil {
		t.Fatalf("Speech: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/v1/tts" {
		t.Errorf("request = %s %s, want POST /v1/tts", gotMethod, gotPath)
	}
	if gotAuth != "Bearer secret" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotBody["text"] != "Hello there" || gotBody["voice_id"] != "rex" || gotBody["language"] != "auto" {
		t.Errorf("body = %v", gotBody)
	}
	// xAI has no model field; sending the route's name would be an unknown key.
	if _, ok := gotBody["model"]; ok {
		t.Errorf("body carries a model field: %v", gotBody)
	}
	of, _ := gotBody["output_format"].(map[string]any)
	if of["codec"] != "mp3" {
		t.Errorf("output_format = %v", of)
	}
	if string(out.Audio) != "ID3grok" || out.ContentType != "audio/mpeg" {
		t.Errorf("response = %q / %q", out.Audio, out.ContentType)
	}
}

func TestXAISpeechFormatsAndDefaultVoice(t *testing.T) {
	cases := map[string]struct {
		codec string
		rate  float64
		ct    string
	}{
		"wav":  {"wav", 24000, "audio/wav"},
		"pcm":  {"pcm", 24000, "audio/L16; rate=24000"},
		"ulaw": {"mulaw", 8000, "audio/basic"},
		"":     {"mp3", 24000, "audio/mpeg"},
	}
	for format, want := range cases {
		var gotBody map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &gotBody)
			w.Header().Set("Content-Type", "application/octet-stream") // not audio/*: keep our label
			_, _ = w.Write([]byte{1, 2, 3})
		}))
		a := NewXAITTSAdapter("xai-voice", srv.URL, "k")
		out, err := a.Speech(context.Background(), SpeechRequest{Input: "hi", Format: format})
		srv.Close()
		if err != nil {
			t.Fatalf("%q: %v", format, err)
		}
		of, _ := gotBody["output_format"].(map[string]any)
		if of["codec"] != want.codec || of["sample_rate"] != want.rate {
			t.Errorf("%q: output_format = %v", format, of)
		}
		if out.ContentType != want.ct {
			t.Errorf("%q: content type = %q, want %q", format, out.ContentType, want.ct)
		}
		if gotBody["voice_id"] != xaiDefaultVoice {
			t.Errorf("%q: voice_id = %v, want the default", format, gotBody["voice_id"])
		}
	}
}

func TestXAISpeechFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Authorization"), "empty") {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"Incorrect API key provided"}`))
	}))
	defer srv.Close()

	_, err := NewXAITTSAdapter("xai-voice", srv.URL, "bad").Speech(context.Background(), SpeechRequest{Input: "hi"})
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "Incorrect API key") {
		t.Errorf("401 error = %v", err)
	}
	// A 200 with no audio is a failed generation, so the router tries the next rung.
	_, err = NewXAITTSAdapter("xai-voice", srv.URL, "empty").Speech(context.Background(), SpeechRequest{Input: "hi"})
	if err == nil || !strings.Contains(err.Error(), "empty audio") {
		t.Errorf("empty 200 error = %v", err)
	}
	if _, err := NewXAITTSAdapter("xai-voice", srv.URL, "k").Speech(context.Background(), SpeechRequest{Input: "  "}); err == nil {
		t.Error("empty input was sent upstream")
	}
	if _, err := NewXAITTSAdapter("xai-voice", "", "k").Chat(context.Background(), ChatRequest{}); err == nil {
		t.Error("Chat should refuse")
	}
}
