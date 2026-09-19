package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// parsigoUpstream is the two-call server the adapter talks to: synthesis hands
// back an id, and the audio is collected from a second address.
func parsigoUpstream(t *testing.T, wav string) (*httptest.Server, *parsigoSpeechRequest) {
	t.Helper()
	var got parsigoSpeechRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/tts" && r.Method == http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"abc123","duration":1.5}`))
		case r.URL.Path == "/api/audio/abc123" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write([]byte(wav))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func TestParsigoSpeechCollectsTheAudioItJustMade(t *testing.T) {
	srv, got := parsigoUpstream(t, "RIFFwav-bytes")
	a := NewParsigoAdapter("parsigo", srv.URL)

	resp, err := a.Speech(context.Background(), SpeechRequest{
		Input: "سلام دنیا",
		Voice: "male_news",
	})
	if err != nil {
		t.Fatalf("Speech: %v", err)
	}
	if string(resp.Audio) != "RIFFwav-bytes" {
		t.Fatalf("audio = %q", resp.Audio)
	}
	if resp.ContentType != "audio/wav" {
		t.Fatalf("content type = %q", resp.ContentType)
	}
	if got.Text != "سلام دنیا" {
		t.Fatalf("text sent = %q", got.Text)
	}
	// The suffix the server insists on is added for a voice named without one.
	if got.Voice != "male_news.wav" {
		t.Fatalf("voice sent = %q", got.Voice)
	}
}

func TestParsigoVoiceNaming(t *testing.T) {
	cases := map[string]string{
		"":                       defaultParsigoVoice,
		"female_narration":       "female_narration.wav",
		"female_narration.wav":   "female_narration.wav",
		"upload:my_own_clip.wav": "upload:my_own_clip.wav",
	}
	for in, want := range cases {
		if got := parsigoVoice(in); got != want {
			t.Errorf("parsigoVoice(%q) = %q, want %q", in, got, want)
		}
	}
}

// The server refuses text longer than its own cap with a message meant for a
// person. Losing it would turn "try a shorter text" into an unexplained 502.
func TestParsigoPassesAnUpstreamRefusalThrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":"متن طولانی است (حداکثر 800 نویسه)"}`))
	}))
	defer srv.Close()

	_, err := NewParsigoAdapter("parsigo", srv.URL).Speech(context.Background(), SpeechRequest{Input: "..."})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "متن طولانی است") {
		t.Fatalf("error lost the upstream message: %v", err)
	}
}

// A synthesis that answers 200 with no bytes has failed; handing an empty file
// to a listener is worse than saying so.
func TestParsigoRefusesEmptyAudio(t *testing.T) {
	srv, _ := parsigoUpstream(t, "")
	_, err := NewParsigoAdapter("parsigo", srv.URL).Speech(context.Background(), SpeechRequest{Input: "سلام"})
	if err == nil || !strings.Contains(err.Error(), "empty audio") {
		t.Fatalf("expected an empty-audio error, got %v", err)
	}
}

func TestParsigoDoesNotPretendToChat(t *testing.T) {
	_, err := NewParsigoAdapter("parsigo", "http://localhost:8000").Chat(context.Background(), ChatRequest{})
	if err == nil {
		t.Fatal("expected chat to be refused")
	}
}
