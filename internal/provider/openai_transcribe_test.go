package provider

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// parseUpload reads what the adapter actually put on the wire.
func parseUpload(t *testing.T, r *http.Request) (fields map[string][]string, filename string, audio []byte) {
	t.Helper()
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("content type: %v", err)
	}
	mr := multipart.NewReader(r.Body, params["boundary"])
	fields = map[string][]string{}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("next part: %v", err)
		}
		data, _ := io.ReadAll(part)
		if part.FormName() == "file" {
			filename, audio = part.FileName(), data
			continue
		}
		fields[part.FormName()] = append(fields[part.FormName()], string(data))
	}
	return fields, filename, audio
}

func newTranscribeServer(t *testing.T, handler http.HandlerFunc) (*OpenAIAdapter, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	a := NewOpenAIAdapter("testprov", srv.URL, "test-key", nil)
	return a, srv.Close
}

func TestTranscribeSendsMultipartWithFileAndFields(t *testing.T) {
	var gotFields map[string][]string
	var gotName string
	var gotAudio []byte
	var gotAuth string

	a, done := newTranscribeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/transcriptions" {
			t.Errorf("path = %q", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		gotFields, gotName, gotAudio = parseUpload(t, r)
		_ = json.NewEncoder(w).Encode(map[string]any{"text": "hello there"})
	})
	defer done()

	temp := 0.25
	resp, err := a.Transcribe(context.Background(), TranscriptionRequest{
		Model: "whisper-1", Audio: []byte("RIFFfake"), Filename: "reel.wav",
		Language: "fa", Prompt: "ReelMind", Temperature: &temp,
		Granularities: []string{"segment"},
	})
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if resp.Text != "hello there" {
		t.Errorf("text = %q", resp.Text)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotName != "reel.wav" || string(gotAudio) != "RIFFfake" {
		t.Errorf("file = %q / %q", gotName, gotAudio)
	}
	for k, want := range map[string]string{
		"model": "whisper-1", "language": "fa", "prompt": "ReelMind",
		"temperature": "0.25", "response_format": "verbose_json",
	} {
		if len(gotFields[k]) == 0 || gotFields[k][0] != want {
			t.Errorf("field %s = %v, want %q", k, gotFields[k], want)
		}
	}
	if got := gotFields["timestamp_granularities[]"]; len(got) != 1 || got[0] != "segment" {
		t.Errorf("granularities = %v", got)
	}
}

func TestTranscribeParsesSegments(t *testing.T) {
	a, done := newTranscribeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _, _ = parseUpload(t, r)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"text": " full text ", "language": "persian", "duration": 12.5,
			"segments": []map[string]any{
				{"id": 0, "start": 0.0, "end": 4.2, "text": " first "},
				{"id": 1, "start": 4.2, "end": 12.5, "text": "second"},
			},
		})
	})
	defer done()

	resp, err := a.Transcribe(context.Background(), TranscriptionRequest{
		Model: "whisper-1", Audio: []byte("x"), Filename: "a.wav",
	})
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if resp.Text != "full text" || resp.Language != "persian" || resp.Duration != 12.5 {
		t.Errorf("got %+v", resp)
	}
	if len(resp.Segments) != 2 {
		t.Fatalf("segments = %d", len(resp.Segments))
	}
	// Timestamps are the whole point: they turn a transcript into a citation.
	if resp.Segments[1].Start != 4.2 || resp.Segments[1].End != 12.5 {
		t.Errorf("segment bounds = %+v", resp.Segments[1])
	}
	if resp.Segments[0].Text != "first" {
		t.Errorf("segment text not trimmed: %q", resp.Segments[0].Text)
	}
}

func TestTranscribeAcceptsPlainTextFromProxiesThatIgnoreFormat(t *testing.T) {
	a, done := newTranscribeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _, _ = parseUpload(t, r)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "  bare transcript  ")
	})
	defer done()

	resp, err := a.Transcribe(context.Background(), TranscriptionRequest{
		Model: "whisper-1", Audio: []byte("x"), Filename: "a.wav",
	})
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if resp.Text != "bare transcript" {
		t.Errorf("text = %q", resp.Text)
	}
}

func TestTranscribeTreatsEmptyResultAsFailure(t *testing.T) {
	// An empty transcript must fail so the router can try the next vendor.
	// Accepting it would let a broken upstream erase an archive item by item.
	a, done := newTranscribeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _, _ = parseUpload(t, r)
		_ = json.NewEncoder(w).Encode(map[string]any{"text": "   "})
	})
	defer done()

	if _, err := a.Transcribe(context.Background(), TranscriptionRequest{
		Model: "whisper-1", Audio: []byte("x"), Filename: "a.wav",
	}); err == nil || !strings.Contains(err.Error(), "empty transcription") {
		t.Fatalf("err = %v", err)
	}
}

func TestTranscribeSurfacesUpstreamError(t *testing.T) {
	a, done := newTranscribeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _, _ = parseUpload(t, r)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"slow down"}}`)
	})
	defer done()

	_, err := a.Transcribe(context.Background(), TranscriptionRequest{
		Model: "whisper-1", Audio: []byte("x"), Filename: "a.wav",
	})
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("err = %v", err)
	}
}

func TestTranscribeRejectsEmptyAudioBeforeCallingUpstream(t *testing.T) {
	called := false
	a, done := newTranscribeServer(t, func(w http.ResponseWriter, r *http.Request) { called = true })
	defer done()

	if _, err := a.Transcribe(context.Background(), TranscriptionRequest{Model: "m"}); err == nil {
		t.Fatal("expected an error for empty audio")
	}
	if called {
		t.Error("upstream was called with no audio")
	}
}

func TestNormalizeAudioFilename(t *testing.T) {
	// The extension is load-bearing: upstreams classify the container from it.
	cases := map[string]string{
		"":               defaultAudioFilename,
		"audio":          "audio.wav",
		"reel.mp3":       "reel.mp3",
		"/etc/passwd":    "passwd.wav",
		"a/b/clip.m4a":   "clip.m4a",
		"  spaced.ogg  ": "spaced.ogg",
	}
	for in, want := range cases {
		if got := normalizeAudioFilename(in); got != want {
			t.Errorf("normalizeAudioFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTranscribeUsesTheLongTimeoutClient(t *testing.T) {
	// The one capability whose work is measured in minutes. On the shared
	// two-minute client a ten-minute recording failed as "context deadline
	// exceeded while awaiting headers" — which reads as an upstream fault
	// rather than our own clock running out while the engine still worked.
	if transcribeHTTPClient.Timeout <= sharedHTTPClient.Timeout {
		t.Fatalf("transcribe timeout %v must exceed the shared %v",
			transcribeHTTPClient.Timeout, sharedHTTPClient.Timeout)
	}
	if transcribeHTTPClient.Timeout < 30*time.Minute {
		t.Errorf("timeout %v is too short for a long recording on a CPU engine",
			transcribeHTTPClient.Timeout)
	}
}
