package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
)

// --- Transcription (ElevenLabs Scribe) ---
//
// Scribe is multipart like OpenAI's endpoint but agrees with it on almost
// nothing else: the path is /speech-to-text, the model field is model_id, the
// language field is language_code, authentication is xi-api-key, and the
// response carries a flat list of words rather than segments. Close enough to
// look reusable, far enough that the openai adapter cannot serve it.
//
// It earns its place in the chain by covering languages the whisper family
// handles poorly, which for this deployment is the whole point.

// elevenTranscription is Scribe's response shape.
type elevenTranscription struct {
	Text         string `json:"text"`
	LanguageCode string `json:"language_code"`
	Words        []struct {
		Text      string  `json:"text"`
		Start     float64 `json:"start"`
		End       float64 `json:"end"`
		Type      string  `json:"type"` // word | spacing | audio_event
		SpeakerID string  `json:"speaker_id"`
	} `json:"words"`
	Detail any `json:"detail"` // error payload; shape varies by failure
}

// Transcribe implements TranscriptionAdapter using POST /speech-to-text.
func (a *ElevenLabsAdapter) Transcribe(ctx context.Context, req TranscriptionRequest) (TranscriptionResponse, error) {
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

	fields := map[string]string{"model_id": req.Model}
	if req.Language != "" {
		// ISO-639-1 or -3. Naming it skips detection and improves accuracy;
		// leaving it empty lets Scribe identify the language itself, which is
		// what a mixed archive needs.
		fields["language_code"] = req.Language
	}
	if len(req.Granularities) > 0 {
		fields["timestamps_granularity"] = "word"
		fields["diarize"] = "true"
	}
	// No else: timestamps_granularity is an enum of "word" and "character" with
	// no off value, so a caller that wants only the words gets the field
	// omitted. Sending "none" is a 422 on every ordinary call.
	if req.Temperature != nil {
		fields["temperature"] = strconv.FormatFloat(*req.Temperature, 'f', -1, 64)
	}
	for k, v := range fields {
		if err := form.WriteField(k, v); err != nil {
			return TranscriptionResponse{}, err
		}
	}
	if err := form.Close(); err != nil {
		return TranscriptionResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/speech-to-text", &body)
	if err != nil {
		return TranscriptionResponse{}, err
	}
	httpReq.Header.Set("Content-Type", form.FormDataContentType())
	// Bearer returns a 401 whose body never mentions the right header.
	httpReq.Header.Set("xi-api-key", a.apiKey)

	// Not a.client: its two-minute cap is sized for a sentence of speech, and a
	// long recording runs past it.
	resp, err := transcribeHTTPClient.Do(httpReq)
	if err != nil {
		return TranscriptionResponse{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return TranscriptionResponse{}, fmt.Errorf("%s: upstream error (status %d): %s", a.name, resp.StatusCode, truncate(raw))
	}

	var parsed elevenTranscription
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return TranscriptionResponse{}, fmt.Errorf("%s: unreadable response: %s", a.name, truncate(raw))
	}

	out := TranscriptionResponse{
		Text:     strings.TrimSpace(parsed.Text),
		Language: parsed.LanguageCode,
	}
	// Words are grouped into one segment per speaker turn. Spacing entries carry
	// no content and audio_event entries describe noises rather than speech, so
	// neither becomes a segment of its own.
	var cur *TranscriptionSegment
	speaker := ""
	for _, w := range parsed.Words {
		if w.End > out.Duration {
			out.Duration = w.End
		}
		if w.Type != "word" {
			continue
		}
		if cur == nil || w.SpeakerID != speaker {
			out.Segments = append(out.Segments, TranscriptionSegment{
				ID: len(out.Segments), Start: w.Start, End: w.End, Text: w.Text,
			})
			cur = &out.Segments[len(out.Segments)-1]
			speaker = w.SpeakerID
			continue
		}
		cur.Text += " " + w.Text
		cur.End = w.End
	}

	if out.Text == "" && len(out.Segments) == 0 {
		return TranscriptionResponse{}, fmt.Errorf("%s: empty transcription", a.name)
	}
	return out, nil
}
