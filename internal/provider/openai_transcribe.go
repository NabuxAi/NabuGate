package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strconv"
	"strings"
)

// --- Transcription (OpenAI Audio Transcriptions API) ---
//
// Unlike every other capability here the request is multipart, not JSON: the
// audio is a file part and the parameters are form fields. Providers that proxy
// OpenAI (Parspack, AvalAI, GapGPT) and providers that merely speak its wire
// format (Groq) accept the same shape, which is what makes one adapter enough.

// openAITranscription is the verbose_json response. A provider asked for a
// plain-text format returns only Text; the rest stay zero.
type openAITranscription struct {
	Text     string  `json:"text"`
	Language string  `json:"language"`
	Duration float64 `json:"duration"`
	Segments []struct {
		ID    int     `json:"id"`
		Start float64 `json:"start"`
		End   float64 `json:"end"`
		Text  string  `json:"text"`
	} `json:"segments"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
		TotalTokens  int `json:"total_tokens"`
		// gpt-4o-transcribe bills audio by the second, not the token.
		Seconds float64 `json:"seconds"`
	} `json:"usage"`
}

// defaultAudioFilename is used when the caller sent no name. The extension is
// load-bearing: upstreams sniff the container from it and reject a file they
// cannot classify, so "audio" alone fails where "audio.wav" succeeds.
const defaultAudioFilename = "audio.wav"

func normalizeAudioFilename(name string) string {
	name = path.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == "/" {
		return defaultAudioFilename
	}
	if path.Ext(name) == "" {
		return name + ".wav"
	}
	return name
}

// Transcribe implements TranscriptionAdapter using POST /audio/transcriptions.
func (a *OpenAIAdapter) Transcribe(ctx context.Context, req TranscriptionRequest) (TranscriptionResponse, error) {
	if len(req.Audio) == 0 {
		return TranscriptionResponse{}, fmt.Errorf("%s: empty audio", a.name)
	}

	var body bytes.Buffer
	form := multipart.NewWriter(&body)

	part, err := form.CreateFormFile("file", normalizeAudioFilename(req.Filename))
	if err != nil {
		return TranscriptionResponse{}, err
	}
	if _, err := part.Write(req.Audio); err != nil {
		return TranscriptionResponse{}, err
	}

	fields := map[string]string{"model": req.Model}
	if req.Language != "" {
		fields["language"] = req.Language
	}
	if req.Prompt != "" {
		fields["prompt"] = req.Prompt
	}
	if req.Temperature != nil {
		fields["temperature"] = strconv.FormatFloat(*req.Temperature, 'f', -1, 64)
	}
	// verbose_json is the only format that carries language, duration and
	// segments. Ask for it whenever timestamps were requested; a caller that
	// wants nothing but the words still gets them from this shape.
	fields["response_format"] = "verbose_json"
	for k, v := range fields {
		if err := form.WriteField(k, v); err != nil {
			return TranscriptionResponse{}, err
		}
	}
	for _, g := range req.Granularities {
		// Repeated field, PHP-style brackets — what the OpenAI API expects.
		if err := form.WriteField("timestamp_granularities[]", g); err != nil {
			return TranscriptionResponse{}, err
		}
	}
	if err := form.Close(); err != nil {
		return TranscriptionResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/audio/transcriptions", &body)
	if err != nil {
		return TranscriptionResponse{}, err
	}
	httpReq.Header.Set("Content-Type", form.FormDataContentType())
	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	for k, v := range a.extraHeaders {
		httpReq.Header.Set(k, v)
	}

	resp, err := transcribeHTTPClient.Do(httpReq)
	if err != nil {
		return TranscriptionResponse{}, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return TranscriptionResponse{}, fmt.Errorf("%s: upstream error (status %d): %s", a.name, resp.StatusCode, truncate(data))
	}

	var parsed openAITranscription
	if err := json.Unmarshal(data, &parsed); err != nil {
		// Some proxies ignore response_format and answer with bare text. That is
		// still a usable transcript, so take it rather than fail the call.
		text := strings.TrimSpace(string(data))
		if text == "" || strings.HasPrefix(text, "{") {
			return TranscriptionResponse{}, fmt.Errorf("%s: unreadable response: %s", a.name, truncate(data))
		}
		return TranscriptionResponse{Text: text}, nil
	}

	out := TranscriptionResponse{
		Text:     strings.TrimSpace(parsed.Text),
		Language: parsed.Language,
		Duration: parsed.Duration,
		Usage: Usage{
			PromptTokens:     parsed.Usage.InputTokens,
			CompletionTokens: parsed.Usage.OutputTokens,
			TotalTokens:      parsed.Usage.TotalTokens,
		},
	}
	for _, s := range parsed.Segments {
		out.Segments = append(out.Segments, TranscriptionSegment{
			ID: s.ID, Start: s.Start, End: s.End, Text: strings.TrimSpace(s.Text),
		})
	}

	// An empty transcript is a failure, not a silent file. Treating it as
	// success lets a broken upstream quietly erase a caller's archive one item
	// at a time, and the router can only fail over if this says so.
	if out.Text == "" && len(out.Segments) == 0 {
		return TranscriptionResponse{}, fmt.Errorf("%s: empty transcription", a.name)
	}
	return out, nil
}
