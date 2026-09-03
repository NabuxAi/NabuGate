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
	"sync"
	"time"
)

// ElevenLabsAdapter speaks text through ElevenLabs and serves it as a
// SpeechAdapter, so a caller asks for audio exactly the way it would ask OpenAI.
//
// Two things about ElevenLabs do not fit the OpenAI shape, and both are handled
// here rather than pushed onto callers:
//
//   - The voice is part of the URL, not the body. OpenAI takes
//     {"voice":"alloy"}; ElevenLabs takes /v1/text-to-speech/{voice_id}. A
//     voice_id is an opaque 20-character string that nobody remembers, so this
//     adapter also accepts a voice *name* ("Rachel", "Charlotte") and resolves
//     it through /v1/voices. The lookup is cached for the process lifetime —
//     the mapping only changes when someone adds a voice in the ElevenLabs
//     console, and a stale cache costs one restart, not a wrong voice.
//
//   - Authentication is `xi-api-key`, not `Authorization: Bearer`. Sending a
//     Bearer token returns 401 with a body that does not mention the header,
//     which is a slow thing to debug.
//
// Model choice matters for Persian. `eleven_multilingual_v2` is the quality
// default but its language list is the smaller one; the v2.5 turbo and flash
// models cover more languages. The model is whatever the route names, so the
// config decides — this adapter does not guess.
type ElevenLabsAdapter struct {
	name    string
	baseURL string
	apiKey  string
	client  *http.Client

	mu     sync.RWMutex
	voices map[string]string // lower-cased name -> voice_id
}

// NewElevenLabsAdapter builds the adapter. baseURL defaults to the public API.
func NewElevenLabsAdapter(name, baseURL, apiKey string) *ElevenLabsAdapter {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.elevenlabs.io/v1"
	}
	return &ElevenLabsAdapter{
		name:    name,
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (a *ElevenLabsAdapter) Name() string { return a.name }

// Chat exists only so the adapter satisfies Adapter and can live in the shared
// adapter map; ElevenLabs speaks audio and nothing else. The router skips a
// provider that lacks a needed capability, so this fires only for a
// misconfigured alias — and fails clearly rather than panicking.
func (a *ElevenLabsAdapter) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	return ChatResponse{}, fmt.Errorf("%s: chat is not supported by the ElevenLabs provider", a.name)
}

// defaultVoiceID is ElevenLabs' "Rachel" — stable since the service launched and
// used only when neither the route nor the caller names a voice.
const defaultVoiceID = "21m00Tcm4TlvDq8ikWAM"

// outputFormat maps the container a caller asked for onto ElevenLabs' own
// format vocabulary, which encodes sample rate and bitrate in the same string.
func elevenOutputFormat(format string) (query, contentType string) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "wav", "pcm":
		// 16-bit PCM at 24 kHz. ElevenLabs returns raw PCM, not a RIFF file, so
		// it is labelled as such — a caller that asked for "wav" and got headerless
		// PCM would otherwise write an unplayable file.
		return "pcm_24000", "audio/L16; rate=24000"
	case "opus", "ogg":
		return "opus_48000_128", "audio/ogg"
	case "ulaw":
		return "ulaw_8000", "audio/basic"
	default:
		return "mp3_44100_128", "audio/mpeg"
	}
}

type elevenVoiceSettings struct {
	Stability       float64 `json:"stability"`
	SimilarityBoost float64 `json:"similarity_boost"`
	Style           float64 `json:"style"`
	UseSpeakerBoost bool    `json:"use_speaker_boost"`
}

type elevenSpeechRequest struct {
	Text          string              `json:"text"`
	ModelID       string              `json:"model_id,omitempty"`
	VoiceSettings elevenVoiceSettings `json:"voice_settings"`
}

// Speech synthesizes req.Input and returns the encoded audio.
func (a *ElevenLabsAdapter) Speech(ctx context.Context, req SpeechRequest) (SpeechResponse, error) {
	text := strings.TrimSpace(req.Input)
	if text == "" {
		return SpeechResponse{}, fmt.Errorf("%s: empty input", a.name)
	}

	voiceID, err := a.resolveVoice(ctx, req.Voice)
	if err != nil {
		return SpeechResponse{}, err
	}
	format, contentType := elevenOutputFormat(req.Format)

	body, _ := json.Marshal(elevenSpeechRequest{
		Text:    text,
		ModelID: strings.TrimSpace(req.Model),
		VoiceSettings: elevenVoiceSettings{
			Stability:       0.45,
			SimilarityBoost: 0.8,
			Style:           0.0,
			UseSpeakerBoost: true,
		},
	})

	endpoint := fmt.Sprintf("%s/text-to-speech/%s?output_format=%s", a.baseURL, url.PathEscape(voiceID), format)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return SpeechResponse{}, fmt.Errorf("%s: %w", a.name, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "audio/*")
	httpReq.Header.Set("xi-api-key", a.apiKey)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return SpeechResponse{}, fmt.Errorf("%s: %w", a.name, err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return SpeechResponse{}, fmt.Errorf("%s: reading audio: %w", a.name, err)
	}
	if resp.StatusCode != http.StatusOK {
		return SpeechResponse{}, fmt.Errorf("%s: upstream %d: %s", a.name, resp.StatusCode, elevenError(payload))
	}
	// A 200 with no bytes is a failed generation, not silence. Returning it as
	// success would put an empty file in front of a listener.
	if len(payload) == 0 {
		return SpeechResponse{}, fmt.Errorf("%s: empty audio", a.name)
	}
	if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "audio/") {
		contentType = ct
	}
	return SpeechResponse{Audio: payload, ContentType: contentType}, nil
}

// resolveVoice turns whatever the route named into a voice_id.
//
// An id is passed straight through; a name is looked up once and cached. When
// the lookup cannot run (no network, revoked key) the request still proceeds
// with the default voice rather than failing — a caller who asked for audio is
// better served by audio in the wrong voice than by an error, and the wrong
// voice is obvious the moment anyone listens.
func (a *ElevenLabsAdapter) resolveVoice(ctx context.Context, voice string) (string, error) {
	voice = strings.TrimSpace(voice)
	if voice == "" {
		return defaultVoiceID, nil
	}
	// Voice ids are opaque alphanumerics with no spaces; anything with a space
	// or a non-ASCII rune is certainly a name.
	if isVoiceID(voice) {
		return voice, nil
	}

	key := strings.ToLower(voice)
	a.mu.RLock()
	id, ok := a.voices[key]
	a.mu.RUnlock()
	if ok {
		return id, nil
	}

	if err := a.loadVoices(ctx); err != nil {
		return defaultVoiceID, nil
	}
	a.mu.RLock()
	id, ok = a.voices[key]
	a.mu.RUnlock()
	if ok {
		return id, nil
	}
	return defaultVoiceID, nil
}

func isVoiceID(s string) bool {
	if len(s) < 16 {
		return false
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func (a *ElevenLabsAdapter) loadVoices(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/voices", nil)
	if err != nil {
		return err
	}
	req.Header.Set("xi-api-key", a.apiKey)
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("voices: upstream %d", resp.StatusCode)
	}
	var out struct {
		Voices []struct {
			VoiceID string `json:"voice_id"`
			Name    string `json:"name"`
		} `json:"voices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	m := make(map[string]string, len(out.Voices))
	for _, v := range out.Voices {
		m[strings.ToLower(v.Name)] = v.VoiceID
	}
	a.mu.Lock()
	a.voices = m
	a.mu.Unlock()
	return nil
}

// elevenError pulls the human part out of an error body, which nests the
// message under detail and sometimes makes detail a plain string.
func elevenError(payload []byte) string {
	var withObject struct {
		Detail struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"detail"`
	}
	if err := json.Unmarshal(payload, &withObject); err == nil && withObject.Detail.Message != "" {
		return withObject.Detail.Message
	}
	var withString struct {
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(payload, &withString); err == nil && withString.Detail != "" {
		return withString.Detail
	}
	if len(payload) > 300 {
		payload = payload[:300]
	}
	return string(payload)
}
