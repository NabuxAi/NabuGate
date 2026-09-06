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
	"sync"
	"testing"
)

// readJobSubmission pulls the config JSON and the audio back off the wire.
func readJobSubmission(t *testing.T, r *http.Request) (cfg map[string]any, filename string, audio []byte) {
	t.Helper()
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("content type: %v", err)
	}
	mr := multipart.NewReader(r.Body, params["boundary"])
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("next part: %v", err)
		}
		data, _ := io.ReadAll(part)
		switch part.FormName() {
		case "data_file":
			filename, audio = part.FileName(), data
		case "config":
			if err := json.Unmarshal(data, &cfg); err != nil {
				t.Fatalf("config is not JSON: %v", err)
			}
		}
	}
	return cfg, filename, audio
}

// The whole job lifecycle — submit, poll past a running state, collect — has to
// fit inside one synchronous Transcribe call, because that is all the router
// knows how to drive.
func TestSpeechmaticsTranscribe(t *testing.T) {
	var (
		mu    sync.Mutex
		polls int
		cfg   map[string]any
		auth  string
		fname string
		audio []byte
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/jobs/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		auth = r.Header.Get("Authorization")
		switch {
		case r.Method == http.MethodPost:
			cfg, fname, audio = readJobSubmission(t, r)
			io.WriteString(w, `{"id":"job-7"}`)
		case strings.HasSuffix(r.URL.Path, "/transcript"):
			if r.URL.Query().Get("format") != "txt" {
				t.Errorf("transcript format = %q", r.URL.Query().Get("format"))
			}
			io.WriteString(w, "  the transcript  \n")
		default:
			polls++
			// First check is still running: the adapter must keep waiting
			// rather than treat an unfinished job as a failure.
			if polls == 1 {
				io.WriteString(w, `{"job":{"status":"running"}}`)
				return
			}
			io.WriteString(w, `{"job":{"status":"done"}}`)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := NewSpeechmaticsAdapter("speechmatics", srv.URL+"/v2", "sm-key")
	out, err := a.Transcribe(context.Background(), TranscriptionRequest{
		Model: "enhanced", Audio: []byte("WAVEfake"), Filename: "note.wav",
	})
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if out.Text != "the transcript" {
		t.Errorf("text = %q", out.Text)
	}

	mu.Lock()
	defer mu.Unlock()
	if polls < 2 {
		t.Errorf("polled %d times; a running job must be waited on", polls)
	}
	if auth != "Bearer sm-key" {
		t.Errorf("auth = %q", auth)
	}
	if fname != "note.wav" || string(audio) != "WAVEfake" {
		t.Errorf("upload = %q / %q", fname, audio)
	}
	tc := cfg["transcription_config"].(map[string]any)
	if tc["operating_point"] != "enhanced" {
		t.Errorf("operating point = %v", tc["operating_point"])
	}
	// With no language given, guessing English would return confident nonsense
	// for Persian audio — identification must be switched on instead.
	if tc["language"] != "auto" {
		t.Errorf("language = %v, want auto", tc["language"])
	}
	if _, ok := cfg["language_identification_config"]; !ok {
		t.Errorf("language identification not enabled: %#v", cfg)
	}
}

// A named language turns identification off and is passed through as-is.
func TestSpeechmaticsHonoursLanguage(t *testing.T) {
	var cfg map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/jobs/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			cfg, _, _ = readJobSubmission(t, r)
			io.WriteString(w, `{"id":"j"}`)
		case strings.HasSuffix(r.URL.Path, "/transcript"):
			io.WriteString(w, "متن")
		default:
			io.WriteString(w, `{"job":{"status":"done"}}`)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := NewSpeechmaticsAdapter("speechmatics", srv.URL+"/v2", "k")
	if _, err := a.Transcribe(context.Background(), TranscriptionRequest{
		Model: "standard", Audio: []byte("x"), Filename: "a.mp3", Language: "fa",
	}); err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	tc := cfg["transcription_config"].(map[string]any)
	if tc["language"] != "fa" {
		t.Errorf("language = %v, want fa", tc["language"])
	}
	if tc["operating_point"] != "standard" {
		t.Errorf("operating point = %v, want standard from the model name", tc["operating_point"])
	}
	if _, ok := cfg["language_identification_config"]; ok {
		t.Errorf("identification should be off when the caller named a language")
	}
}

// A rejected job must surface as an error so the router moves to the next rung
// instead of storing an empty transcript.
func TestSpeechmaticsRejectedJob(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/jobs/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			io.WriteString(w, `{"id":"j"}`)
			return
		}
		io.WriteString(w, `{"job":{"status":"rejected","errors":[{"message":"unsupported audio"}]}}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := NewSpeechmaticsAdapter("speechmatics", srv.URL+"/v2", "k")
	_, err := a.Transcribe(context.Background(), TranscriptionRequest{
		Model: "enhanced", Audio: []byte("x"), Filename: "a.wav",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported audio") {
		t.Fatalf("err = %v, want the rejection reason", err)
	}
}

// json-v2 emits one result per token. Segments group them by speaker turn, and
// punctuation joins the word before it rather than standing on its own.
func TestSpeechmaticsSegments(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/jobs/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			io.WriteString(w, `{"id":"j"}`)
		case strings.HasSuffix(r.URL.Path, "/transcript"):
			if r.URL.Query().Get("format") == "json-v2" {
				io.WriteString(w, `{"metadata":{"language":"en"},"results":[
					{"type":"word","start_time":0,"end_time":0.4,"alternatives":[{"content":"one","speaker":"S1"}]},
					{"type":"word","start_time":0.4,"end_time":0.9,"alternatives":[{"content":"two","speaker":"S1"}]},
					{"type":"punctuation","start_time":0.9,"end_time":0.9,"alternatives":[{"content":".","speaker":"S1"}]},
					{"type":"word","start_time":1.2,"end_time":1.8,"alternatives":[{"content":"three","speaker":"S2"}]}]}`)
				return
			}
			io.WriteString(w, "one two. three")
		default:
			io.WriteString(w, `{"job":{"status":"done"}}`)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := NewSpeechmaticsAdapter("speechmatics", srv.URL+"/v2", "k")
	out, err := a.Transcribe(context.Background(), TranscriptionRequest{
		Model: "enhanced", Audio: []byte("x"), Filename: "a.wav",
		Granularities: []string{"segment"},
	})
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if len(out.Segments) != 2 {
		t.Fatalf("segments = %d, want one per speaker turn: %+v", len(out.Segments), out.Segments)
	}
	if out.Segments[0].Text != "one two." {
		t.Errorf("first segment = %q, want punctuation attached to the word before it", out.Segments[0].Text)
	}
	if out.Segments[1].Text != "three" || out.Segments[1].Start != 1.2 {
		t.Errorf("second segment = %+v", out.Segments[1])
	}
	if out.Duration != 1.8 || out.Language != "en" {
		t.Errorf("duration = %v, language = %q", out.Duration, out.Language)
	}
}

// Chat is not something this vendor does; saying so beats failing later.
func TestSpeechmaticsChatUnsupported(t *testing.T) {
	a := NewSpeechmaticsAdapter("speechmatics", "https://example.invalid/v2", "k")
	if _, err := a.Chat(context.Background(), ChatRequest{}); err == nil {
		t.Fatal("chat should not be supported")
	}
}
