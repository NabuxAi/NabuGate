package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// Upload, submit, poll — all three inside one synchronous Transcribe, which is
// the only shape the router knows how to drive.
func TestAssemblyAITranscribe(t *testing.T) {
	var (
		mu       sync.Mutex
		polls    int
		uploaded []byte
		job      map[string]any
		auth     string
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/upload", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		auth = r.Header.Get("Authorization")
		uploaded, _ = io.ReadAll(r.Body)
		io.WriteString(w, `{"upload_url":"https://cdn.assemblyai.com/u/1"}`)
	})
	mux.HandleFunc("POST /v2/transcript", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &job)
		io.WriteString(w, `{"id":"t-1","status":"queued"}`)
	})
	mux.HandleFunc("GET /v2/transcript/t-1", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		polls++
		// Still working on the first check: the adapter must wait rather than
		// treat an unfinished job as a failure.
		if polls == 1 {
			io.WriteString(w, `{"id":"t-1","status":"processing"}`)
			return
		}
		io.WriteString(w, `{"id":"t-1","status":"completed","text":"  hello there  ",
			"language_code":"en","audio_duration":9,
			"utterances":[{"start":500,"end":1400,"text":"hello","speaker":"A"},
			              {"start":1600,"end":2300,"text":"there","speaker":"B"}]}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := NewAssemblyAIAdapter("assemblyai", srv.URL+"/v2", "aai-secret")
	out, err := a.Transcribe(context.Background(), TranscriptionRequest{
		Model: "universal", Audio: []byte("RIFFbytes"), Filename: "a.wav",
		Granularities: []string{"segment"},
	})
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if out.Text != "hello there" || out.Language != "en" || out.Duration != 9 {
		t.Errorf("out = %+v", out)
	}

	mu.Lock()
	defer mu.Unlock()
	// A bare key: neither "Bearer" nor "Token", and the 401 for a wrong scheme
	// does not say which part it disliked.
	if auth != "aai-secret" {
		t.Errorf("authorization = %q, want the bare key", auth)
	}
	if string(uploaded) != "RIFFbytes" {
		t.Errorf("uploaded = %q", uploaded)
	}
	if job["audio_url"] != "https://cdn.assemblyai.com/u/1" {
		t.Errorf("job did not reference the upload: %v", job)
	}
	if job["speaker_labels"] != true {
		t.Errorf("speaker labels not requested despite a granularity: %v", job)
	}
	if polls < 2 {
		t.Errorf("polled %d times; a processing job must be waited on", polls)
	}

	// Utterances are already one per speaker turn, and their milliseconds must
	// arrive as the seconds every other adapter reports.
	if len(out.Segments) != 2 {
		t.Fatalf("segments = %+v", out.Segments)
	}
	if out.Segments[0].Start != 0.5 || out.Segments[1].End != 2.3 {
		t.Errorf("segments = %+v; milliseconds were not converted", out.Segments)
	}
}

// With no language given, the default is English — which on Persian audio
// returns confident nonsense rather than an error.
func TestAssemblyAIEnablesLanguageDetection(t *testing.T) {
	var job map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/upload", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"upload_url":"u"}`)
	})
	mux.HandleFunc("POST /v2/transcript", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &job)
		io.WriteString(w, `{"id":"t","status":"queued"}`)
	})
	mux.HandleFunc("GET /v2/transcript/t", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"status":"completed","text":"متن"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := NewAssemblyAIAdapter("assemblyai", srv.URL+"/v2", "k")
	if _, err := a.Transcribe(context.Background(), TranscriptionRequest{
		Model: "universal", Audio: []byte("x"), Filename: "a.mp3",
	}); err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if job["language_detection"] != true {
		t.Errorf("language detection off: %v", job)
	}
	if _, ok := job["language_code"]; ok {
		t.Errorf("a language was sent though none was asked for: %v", job)
	}
}

// A failed job must surface as an error so the router moves to the next rung
// instead of storing an empty transcript.
func TestAssemblyAIFailedJob(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/upload", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"upload_url":"u"}`)
	})
	mux.HandleFunc("POST /v2/transcript", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"id":"t","status":"queued"}`)
	})
	mux.HandleFunc("GET /v2/transcript/t", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"status":"error","error":"audio file is too short"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := NewAssemblyAIAdapter("assemblyai", srv.URL+"/v2", "k")
	_, err := a.Transcribe(context.Background(), TranscriptionRequest{
		Model: "universal", Audio: []byte("x"), Filename: "a.wav",
	})
	if err == nil || !strings.Contains(err.Error(), "too short") {
		t.Fatalf("err = %v, want the job's own reason", err)
	}
}
