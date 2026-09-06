package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// A general Gemini model transcribes through generateContent with the audio
// inlined, which is the rung that needs no Files API at all.
func TestGeminiTranscribeInline(t *testing.T) {
	var gotPath, gotKey string
	var body map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotKey = r.URL.Path, r.Header.Get("x-goog-api-key")
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &body)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"  سلام دنیا  "}]}}]}`)
	}))
	defer srv.Close()

	a := NewGeminiAdapter("gemini", srv.URL+"/v1beta", "k-123")
	out, err := a.Transcribe(context.Background(), TranscriptionRequest{
		Model: "gemini-3-flash", Audio: []byte("RIFFfake"), Filename: "voice.ogg",
	})
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if out.Text != "سلام دنیا" {
		t.Errorf("text = %q, want the trimmed transcript", out.Text)
	}
	if gotPath != "/v1beta/models/gemini-3-flash:generateContent" {
		t.Errorf("path = %q", gotPath)
	}
	if gotKey != "k-123" {
		t.Errorf("api key header = %q; Gemini transcription authenticates by header, not query", gotKey)
	}

	// The audio must arrive as base64 inline data with a MIME type derived from
	// the filename — Gemini rejects an audio part it cannot classify.
	parts := body["contents"].([]any)[0].(map[string]any)["parts"].([]any)
	inline, ok := parts[1].(map[string]any)["inlineData"].(map[string]any)
	if !ok {
		t.Fatalf("second part is not inlineData: %#v", parts[1])
	}
	if inline["mimeType"] != "audio/ogg" {
		t.Errorf("mimeType = %v, want audio/ogg for a .ogg file", inline["mimeType"])
	}
	if got, _ := base64.StdEncoding.DecodeString(inline["data"].(string)); string(got) != "RIFFfake" {
		t.Errorf("inline audio = %q", got)
	}
	// Temperature must default to 0: a warm model invents words.
	if temp := body["generationConfig"].(map[string]any)["temperature"]; temp != float64(0) {
		t.Errorf("temperature = %v, want 0", temp)
	}
}

// The dedicated transcription model takes the audio by reference, so the
// adapter has to upload it, wait for it, read the interaction, and clean up.
func TestGeminiTranscribeInteraction(t *testing.T) {
	var (
		mu      sync.Mutex
		deleted string
		uploadN int
	)
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/upload/v1beta/files", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Goog-Upload-Command") != "start" {
			t.Errorf("upload command = %q, want start", r.Header.Get("X-Goog-Upload-Command"))
		}
		if r.Header.Get("X-Goog-Upload-Header-Content-Type") != "audio/mp3" {
			t.Errorf("declared content type = %q", r.Header.Get("X-Goog-Upload-Header-Content-Type"))
		}
		w.Header().Set("X-Goog-Upload-Url", srv.URL+"/bytes")
		io.WriteString(w, `{}`)
	})
	mux.HandleFunc("/bytes", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		uploadN++
		mu.Unlock()
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != "ID3audio" {
			t.Errorf("uploaded bytes = %q", raw)
		}
		if r.Header.Get("X-Goog-Upload-Command") != "upload, finalize" {
			t.Errorf("finalize command = %q", r.Header.Get("X-Goog-Upload-Command"))
		}
		io.WriteString(w, `{"file":{"name":"files/abc","uri":"https://files/abc","state":"ACTIVE"}}`)
	})
	mux.HandleFunc("/v1beta/interactions", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &body)
		in := body["input"].([]any)[0].(map[string]any)
		if in["uri"] != "https://files/abc" {
			t.Errorf("interaction referenced %v, not the uploaded file", in["uri"])
		}
		cfg := body["generation_config"].(map[string]any)["transcription_config"].(map[string]any)
		if cfg["enable_word_timestamps"] != true {
			t.Errorf("word timestamps not requested despite a granularity: %#v", cfg)
		}
		io.WriteString(w, `{"output_text":"hello there","steps":[{"content":[{"annotations":[
			{"type":"word_info","text":"hello","start_offset":"0.5s","end_offset":"1s"},
			{"type":"word_info","text":"there","start_offset":"1s","end_offset":"2.25s"},
			{"type":"other","text":"ignored"}]}]}]}`)
	})
	mux.HandleFunc("/v1beta/files/abc", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			mu.Lock()
			deleted = "files/abc"
			mu.Unlock()
		}
		io.WriteString(w, `{}`)
	})

	a := NewGeminiAdapter("gemini", srv.URL+"/v1beta", "k")
	out, err := a.Transcribe(context.Background(), TranscriptionRequest{
		Model: "gemini-3.5-transcribe", Audio: []byte("ID3audio"), Filename: "clip.mp3",
		Granularities: []string{"word"},
	})
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if out.Text != "hello there" {
		t.Errorf("text = %q", out.Text)
	}
	if len(out.Segments) != 2 {
		t.Fatalf("segments = %d, want 2 word_info annotations (the other type is not a word)", len(out.Segments))
	}
	if out.Segments[0].Start != 0.5 || out.Segments[1].End != 2.25 {
		t.Errorf("offsets = %+v; protobuf durations should parse as seconds", out.Segments)
	}
	if out.Duration != 2.25 {
		t.Errorf("duration = %v, want the last word's end", out.Duration)
	}

	mu.Lock()
	defer mu.Unlock()
	if uploadN != 1 {
		t.Errorf("uploaded %d times, want 1", uploadN)
	}
	// The 48-hour retention is charged whether or not we read the file again.
	if deleted != "files/abc" {
		t.Errorf("uploaded file was not deleted; it would sit in the quota until it expired")
	}
}

// A model that answers with nothing is a failure, not a silent recording: the
// router can only fail over to the next vendor if this says so.
func TestGeminiTranscribeEmptyIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"   "}]}}]}`)
	}))
	defer srv.Close()

	a := NewGeminiAdapter("gemini", srv.URL+"/v1beta", "k")
	_, err := a.Transcribe(context.Background(), TranscriptionRequest{
		Model: "gemini-3-flash", Audio: []byte("x"), Filename: "a.wav",
	})
	if err == nil || !strings.Contains(err.Error(), "empty transcription") {
		t.Fatalf("err = %v, want an empty-transcription error", err)
	}
}

func TestGeminiUploadEndpoint(t *testing.T) {
	a := NewGeminiAdapter("g", "https://generativelanguage.googleapis.com/v1beta", "k")
	got, err := a.uploadEndpoint()
	if err != nil {
		t.Fatalf("uploadEndpoint: %v", err)
	}
	want := "https://generativelanguage.googleapis.com/upload/v1beta/files"
	if got != want {
		t.Errorf("uploadEndpoint = %q, want %q", got, want)
	}
}
