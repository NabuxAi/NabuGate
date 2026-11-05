package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// XAITTSAdapter speaks text through xAI's Grok voice (POST /v1/tts) and serves
// it as a SpeechAdapter, so a caller asks for it the way it asks OpenAI.
//
// It is its own adapter, not a mode of the openai one that already serves
// xAI's chat, because nothing past the Bearer header matches /audio/speech:
//
//   - The path is /tts and the text field is `text`, not `input`.
//   - The voice is `voice_id` ("eve", "ara", "rex", …), case-insensitive.
//   - `language` is required. A speech request here carries no language, so
//     the adapter sends "auto" and lets the model read the script.
//   - The container is an object — {"codec","sample_rate"} — not a string.
//
// There is no model field: the route's model name ("grok-tts") exists for the
// router and the price list, and is not sent.
//
// Persian is not on Grok's language list. An alias that can carry Persian text
// must not route here; that is the config's job, and the config tests check it.
type XAITTSAdapter struct {
	name    string
	baseURL string
	apiKey  string
	client  *http.Client
}

// NewXAITTSAdapter builds the adapter. baseURL defaults to the public API.
func NewXAITTSAdapter(name, baseURL, apiKey string) *XAITTSAdapter {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.x.ai/v1"
	}
	return &XAITTSAdapter{
		name:    name,
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (a *XAITTSAdapter) Name() string { return a.name }

// Chat exists only so the adapter fits the shared adapter map. Grok's chat is
// served by the "xai" provider through the openai adapter; this one speaks.
func (a *XAITTSAdapter) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	return ChatResponse{}, fmt.Errorf("%s: chat is not supported by the xAI voice provider", a.name)
}

// xaiDefaultVoice is the voice xAI itself defaults to.
const xaiDefaultVoice = "eve"

type xaiOutputFormat struct {
	Codec      string `json:"codec"`
	SampleRate int    `json:"sample_rate,omitempty"`
}

type xaiSpeechRequest struct {
	Text         string          `json:"text"`
	VoiceID      string          `json:"voice_id"`
	Language     string          `json:"language"`
	OutputFormat xaiOutputFormat `json:"output_format"`
}

// xaiFormat maps the container a caller asked for onto xAI's codec and
// sample rate. Raw PCM and G.711 come back without a header, so they are
// labelled by what they are rather than by what was asked for.
func xaiFormat(format string) (xaiOutputFormat, string) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "wav":
		return xaiOutputFormat{Codec: "wav", SampleRate: 24000}, "audio/wav"
	case "pcm":
		return xaiOutputFormat{Codec: "pcm", SampleRate: 24000}, "audio/L16; rate=24000"
	case "ulaw", "mulaw":
		return xaiOutputFormat{Codec: "mulaw", SampleRate: 8000}, "audio/basic"
	case "alaw":
		return xaiOutputFormat{Codec: "alaw", SampleRate: 8000}, "audio/x-alaw-basic"
	default:
		return xaiOutputFormat{Codec: "mp3", SampleRate: 24000}, "audio/mpeg"
	}
}

// Speech synthesizes req.Input and returns the encoded audio.
func (a *XAITTSAdapter) Speech(ctx context.Context, req SpeechRequest) (SpeechResponse, error) {
	text := strings.TrimSpace(req.Input)
	if text == "" {
		return SpeechResponse{}, fmt.Errorf("%s: empty input", a.name)
	}
	voice := strings.ToLower(strings.TrimSpace(req.Voice))
	if voice == "" {
		voice = xaiDefaultVoice
	}
	format, contentType := xaiFormat(req.Format)

	body, _ := json.Marshal(xaiSpeechRequest{
		Text: text, VoiceID: voice, Language: "auto", OutputFormat: format,
	})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/tts", bytes.NewReader(body))
	if err != nil {
		return SpeechResponse{}, fmt.Errorf("%s: %w", a.name, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)

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
		return SpeechResponse{}, fmt.Errorf("%s: upstream %d: %s", a.name, resp.StatusCode, xaiError(payload))
	}
	// A 200 with no bytes is a failed generation, not silence.
	if len(payload) == 0 {
		return SpeechResponse{}, fmt.Errorf("%s: empty audio", a.name)
	}
	if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "audio/") {
		contentType = ct
	}
	return SpeechResponse{Audio: payload, ContentType: contentType}, nil
}

// xaiError pulls the message out of an error body ({"error": "…"} or
// {"error": {"message": "…"}}), else a bounded slice of the raw body.
func xaiError(payload []byte) string {
	var withString struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(payload, &withString); err == nil && withString.Error != "" {
		return withString.Error
	}
	var withObject struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(payload, &withObject); err == nil && withObject.Error.Message != "" {
		return withObject.Error.Message
	}
	if len(payload) > 300 {
		payload = payload[:300]
	}
	return string(payload)
}
