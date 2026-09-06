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

// --- AssemblyAI (/v2) ---
//
// Three calls to one transcript: the audio is uploaded and comes back as a URL,
// the URL is submitted as a job, and the job is polled. Same asynchronous shape
// as Speechmatics, different enough in the details to need its own adapter —
// the upload is a separate endpoint rather than a part of the submission, auth
// is a bare key in Authorization with no scheme, and the transcript arrives as
// a field on the job rather than from a separate fetch.
//
// TranscriptionAdapter is synchronous, so all three happen inside one call and
// the caller cannot tell a different vendor answered.

// assemblyPollCeiling caps the gap between status checks, so an hour of audio
// does not cost hundreds of requests while a short clip still returns fast.
const assemblyPollCeiling = 10 * time.Second

// AssemblyAIAdapter speaks the AssemblyAI v2 API.
type AssemblyAIAdapter struct {
	name    string
	baseURL string
	apiKey  string
}

// NewAssemblyAIAdapter builds the adapter. baseURL defaults to the public API.
func NewAssemblyAIAdapter(name, baseURL, apiKey string) *AssemblyAIAdapter {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.assemblyai.com/v2"
	}
	return &AssemblyAIAdapter{
		name:    name,
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
	}
}

func (a *AssemblyAIAdapter) Name() string { return a.name }

// Chat exists only to satisfy Adapter; AssemblyAI transcribes and nothing else.
func (a *AssemblyAIAdapter) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	return ChatResponse{}, fmt.Errorf("%s: chat is not supported by the AssemblyAI provider", a.name)
}

// auth sets the credential. AssemblyAI takes the key bare — no "Bearer", no
// "Token" — and answers a scheme it did not expect with a 401 that does not say
// which part it disliked.
func (a *AssemblyAIAdapter) auth(r *http.Request) { r.Header.Set("Authorization", a.apiKey) }

type assemblyTranscript struct {
	ID       string `json:"id"`
	Status   string `json:"status"` // queued | processing | completed | error
	Text     string `json:"text"`
	Language string `json:"language_code"`
	// AudioDuration is whole seconds — AssemblyAI does not report fractions.
	AudioDuration float64 `json:"audio_duration"`
	Error         string  `json:"error"`
	Utterances    []struct {
		Start   int64  `json:"start"` // milliseconds
		End     int64  `json:"end"`
		Text    string `json:"text"`
		Speaker string `json:"speaker"`
	} `json:"utterances"`
	Words []struct {
		Start int64  `json:"start"`
		End   int64  `json:"end"`
		Text  string `json:"text"`
	} `json:"words"`
}

// Transcribe implements TranscriptionAdapter.
func (a *AssemblyAIAdapter) Transcribe(ctx context.Context, req TranscriptionRequest) (TranscriptionResponse, error) {
	if len(req.Audio) == 0 {
		return TranscriptionResponse{}, fmt.Errorf("%s: empty audio", a.name)
	}

	audioURL, err := a.upload(ctx, req.Audio)
	if err != nil {
		return TranscriptionResponse{}, err
	}

	body := map[string]any{"audio_url": audioURL}
	if req.Model != "" {
		body["speech_model"] = req.Model
	}
	if req.Language != "" {
		body["language_code"] = req.Language
	} else {
		// Without this the default is English, which on Persian audio returns
		// confident nonsense rather than an error.
		body["language_detection"] = true
	}
	if len(req.Granularities) > 0 {
		body["speaker_labels"] = true
	}
	if req.Prompt != "" {
		// Word boost is the vocabulary hint every other adapter here calls
		// prompt: terms it should expect to hear.
		body["word_boost"] = strings.Fields(req.Prompt)
	}

	job, err := a.submit(ctx, body)
	if err != nil {
		return TranscriptionResponse{}, err
	}
	final, err := a.await(ctx, job.ID)
	if err != nil {
		return TranscriptionResponse{}, err
	}

	out := TranscriptionResponse{
		Text:     strings.TrimSpace(final.Text),
		Language: final.Language,
		Duration: final.AudioDuration,
	}
	if out.Language == "" {
		out.Language = req.Language
	}
	// Utterances are already grouped by speaker turn, which is what a segment
	// should be. Fall back to words only when diarization was not asked for.
	for _, u := range final.Utterances {
		out.Segments = append(out.Segments, TranscriptionSegment{
			ID: len(out.Segments), Start: msToSeconds(u.Start), End: msToSeconds(u.End),
			Text: strings.TrimSpace(u.Text),
		})
	}
	if len(out.Segments) == 0 && len(req.Granularities) > 0 {
		for _, w := range final.Words {
			out.Segments = append(out.Segments, TranscriptionSegment{
				ID: len(out.Segments), Start: msToSeconds(w.Start), End: msToSeconds(w.End), Text: w.Text,
			})
		}
	}

	if out.Text == "" && len(out.Segments) == 0 {
		return TranscriptionResponse{}, fmt.Errorf("%s: empty transcription", a.name)
	}
	return out, nil
}

// msToSeconds converts AssemblyAI's milliseconds to the seconds every other
// adapter here reports, so a caller never has to ask which vendor answered.
func msToSeconds(ms int64) float64 { return float64(ms) / 1000 }

// upload posts the raw bytes and returns the URL to reference them by. The URL
// is private to the account and expires on its own, so there is nothing to
// clean up afterwards.
func (a *AssemblyAIAdapter) upload(ctx context.Context, audio []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/upload", bytes.NewReader(audio))
	if err != nil {
		return "", err
	}
	a.auth(req)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := transcribeHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("%s: upload refused (status %d): %s", a.name, resp.StatusCode, truncate(raw))
	}
	var parsed struct {
		UploadURL string `json:"upload_url"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.UploadURL == "" {
		return "", fmt.Errorf("%s: upload gave no URL: %s", a.name, truncate(raw))
	}
	return parsed.UploadURL, nil
}

func (a *AssemblyAIAdapter) submit(ctx context.Context, body map[string]any) (assemblyTranscript, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return assemblyTranscript{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/transcript", bytes.NewReader(payload))
	if err != nil {
		return assemblyTranscript{}, err
	}
	a.auth(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := transcribeHTTPClient.Do(req)
	if err != nil {
		return assemblyTranscript{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return assemblyTranscript{}, fmt.Errorf("%s: job refused (status %d): %s", a.name, resp.StatusCode, truncate(raw))
	}
	var parsed assemblyTranscript
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.ID == "" {
		return assemblyTranscript{}, fmt.Errorf("%s: job submission gave no id: %s", a.name, truncate(raw))
	}
	return parsed, nil
}

// await polls until the job leaves queued/processing. The request context is
// the real deadline; this only decides how often to ask.
func (a *AssemblyAIAdapter) await(ctx context.Context, id string) (assemblyTranscript, error) {
	delay := 500 * time.Millisecond
	for {
		select {
		case <-ctx.Done():
			return assemblyTranscript{}, fmt.Errorf("%s: job %s still running when the request expired: %w", a.name, id, ctx.Err())
		case <-time.After(delay):
		}
		if delay < assemblyPollCeiling {
			delay *= 2
			if delay > assemblyPollCeiling {
				delay = assemblyPollCeiling
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/transcript/"+id, nil)
		if err != nil {
			return assemblyTranscript{}, err
		}
		a.auth(req)
		resp, err := transcribeHTTPClient.Do(req)
		if err != nil {
			return assemblyTranscript{}, err
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			return assemblyTranscript{}, fmt.Errorf("%s: cannot read job %s (status %d): %s", a.name, id, resp.StatusCode, truncate(raw))
		}
		var parsed assemblyTranscript
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return assemblyTranscript{}, fmt.Errorf("%s: unreadable job status: %s", a.name, truncate(raw))
		}
		switch parsed.Status {
		case "completed":
			return parsed, nil
		case "queued", "processing":
			continue
		default:
			msg := parsed.Status
			if parsed.Error != "" {
				msg += ": " + parsed.Error
			}
			return assemblyTranscript{}, fmt.Errorf("%s: job %s %s", a.name, id, msg)
		}
	}
}
