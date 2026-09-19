package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ParsigoAdapter speaks Persian through پارسی‌گو (github.com/nimaone/persian_tts),
// a self-hosted text-to-speech server, and serves it as a SpeechAdapter so a
// caller asks for audio exactly the way it would ask OpenAI.
//
// It is the only speech provider here that runs on the deployment's own
// machine: no key, no vendor, no per-character price, and nothing about the
// text leaves the network it is hosted on. That is the whole reason to route to
// it — Persian synthesis that would otherwise be billed abroad, for a business
// whose text may not travel.
//
// Three things about it do not fit the OpenAI shape:
//
//   - Synthesis and download are two calls. POST /api/tts returns an id, and
//     the WAV is fetched from GET /api/audio/{id}. A caller of this gateway
//     asked for one file, so both happen here.
//
//   - There is no authentication at all, so the base_url is the whole
//     credential. It belongs on a private address, never on the open internet.
//
//   - It produces WAV and nothing else. A caller asking for mp3 gets WAV,
//     correctly labelled, because the interface calls the format best-effort
//     and audio in the wrong container beats no audio.
//
// The voice is a reference recording on the server ("female_narration.wav"), or
// "upload:<name>.wav" for one somebody cloned through its own web UI.
type ParsigoAdapter struct {
	name    string
	baseURL string
	client  *http.Client
}

// NewParsigoAdapter builds the adapter. baseURL is required — there is no
// public instance to fall back to, which is the point of a self-hosted voice.
func NewParsigoAdapter(name, baseURL string) *ParsigoAdapter {
	return &ParsigoAdapter{
		name:    name,
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		// Synthesis runs on CPU and is roughly real time, so a long paragraph
		// takes tens of seconds. The ceiling is generous for that reason; the
		// caller's context still cuts it short when they give up first.
		client: &http.Client{Timeout: 300 * time.Second},
	}
}

func (a *ParsigoAdapter) Name() string { return a.name }

// Chat exists only so the adapter satisfies Adapter and can live in the shared
// adapter map. The router skips a provider that lacks a needed capability, so
// this fires only for a misconfigured alias — and says so rather than panicking.
func (a *ParsigoAdapter) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	return ChatResponse{}, fmt.Errorf("%s: chat is not supported by the parsigo provider", a.name)
}

// defaultParsigoVoice is one of the three recordings the server ships with.
// Used only when neither the route nor the caller names a voice.
const defaultParsigoVoice = "female_narration.wav"

// parsigoVoice accepts a voice with or without its .wav suffix, because the
// alias in a config file reads better without one and the server insists on it.
// An "upload:" id is passed through untouched — the suffix is already part of
// the name somebody uploaded.
func parsigoVoice(voice string) string {
	voice = strings.TrimSpace(voice)
	if voice == "" {
		return defaultParsigoVoice
	}
	if strings.HasPrefix(voice, "upload:") || strings.HasSuffix(voice, ".wav") {
		return voice
	}
	return voice + ".wav"
}

type parsigoSpeechRequest struct {
	Text  string `json:"text"`
	Voice string `json:"voice"`
}

// Speech synthesizes req.Input and returns the WAV the server produced.
func (a *ParsigoAdapter) Speech(ctx context.Context, req SpeechRequest) (SpeechResponse, error) {
	text := strings.TrimSpace(req.Input)
	if text == "" {
		return SpeechResponse{}, fmt.Errorf("%s: empty input", a.name)
	}
	if a.baseURL == "" {
		return SpeechResponse{}, fmt.Errorf("%s: no base_url configured", a.name)
	}

	body, _ := json.Marshal(parsigoSpeechRequest{Text: text, Voice: parsigoVoice(req.Voice)})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/api/tts", bytes.NewReader(body))
	if err != nil {
		return SpeechResponse{}, fmt.Errorf("%s: %w", a.name, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return SpeechResponse{}, fmt.Errorf("%s: %w", a.name, err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return SpeechResponse{}, fmt.Errorf("%s: reading response: %w", a.name, err)
	}
	if resp.StatusCode != http.StatusOK {
		// The server refuses text over its own character cap, and text it can
		// find no Persian in, with a message written for a person. Passing it
		// through is the difference between "try a shorter text" and "502".
		return SpeechResponse{}, fmt.Errorf("%s: upstream %d: %s", a.name, resp.StatusCode, parsigoError(payload))
	}

	var made struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(payload, &made); err != nil {
		return SpeechResponse{}, fmt.Errorf("%s: invalid response: %w", a.name, err)
	}
	if made.ID == "" {
		return SpeechResponse{}, fmt.Errorf("%s: no audio id returned", a.name)
	}

	return a.fetchAudio(ctx, made.ID)
}

// fetchAudio collects the WAV the synthesis call left behind. The server holds
// it in memory under that id, so this is the second half of one operation
// rather than a call a caller could be asked to make itself.
func (a *ParsigoAdapter) fetchAudio(ctx context.Context, id string) (SpeechResponse, error) {
	endpoint := a.baseURL + "/api/audio/" + url.PathEscape(id)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return SpeechResponse{}, fmt.Errorf("%s: %w", a.name, err)
	}
	httpReq.Header.Set("Accept", "audio/*")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return SpeechResponse{}, fmt.Errorf("%s: %w", a.name, err)
	}
	defer resp.Body.Close()

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return SpeechResponse{}, fmt.Errorf("%s: reading audio: %w", a.name, err)
	}
	if resp.StatusCode != http.StatusOK {
		return SpeechResponse{}, fmt.Errorf("%s: upstream %d: %s", a.name, resp.StatusCode, parsigoError(audio))
	}
	// A 200 with no bytes is a failed generation, not silence. Returning it as
	// success would put an empty file in front of a listener.
	if len(audio) == 0 {
		return SpeechResponse{}, fmt.Errorf("%s: empty audio", a.name)
	}

	contentType := "audio/wav"
	if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "audio/") {
		contentType = ct
	}
	return SpeechResponse{Audio: audio, ContentType: contentType}, nil
}

// parsigoError pulls the human part out of a FastAPI refusal, which carries it
// under "detail" — a plain string for the messages this server writes, an
// object for the ones its framework writes about a malformed body.
func parsigoError(payload []byte) string {
	var withString struct {
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(payload, &withString); err == nil && withString.Detail != "" {
		return withString.Detail
	}
	trimmed := strings.TrimSpace(string(payload))
	if len(trimmed) > 300 {
		trimmed = trimmed[:300] + "…"
	}
	if trimmed == "" {
		return "(empty response)"
	}
	return trimmed
}
