package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// --- Speechmatics (Batch Transcription v2) ---
//
// Speechmatics is the one transcription upstream here that is not a single
// request. A job is submitted, runs on their side, and is collected afterwards:
//
//	POST /v2/jobs/            -> {"id": "..."}
//	GET  /v2/jobs/{id}        -> {"job": {"status": "running"|"done"|"rejected"}}
//	GET  /v2/jobs/{id}/transcript?format=txt
//
// TranscriptionAdapter is synchronous, so all three happen inside one
// Transcribe call and the caller sees an ordinary — if slower — transcription.
// That is deliberate: the point of putting Speechmatics in the fallback chain
// is that nothing above the router has to know a different vendor answered.

// speechmaticsPollCeiling caps the gap between status checks. The first checks
// are quick because most short clips finish in seconds; the interval then backs
// off so an hour-long file does not cost hundreds of requests.
const speechmaticsPollCeiling = 15 * time.Second

// SpeechmaticsAdapter speaks the Speechmatics batch API.
type SpeechmaticsAdapter struct {
	name    string
	baseURL string
	apiKey  string
}

// NewSpeechmaticsAdapter builds a Speechmatics adapter.
func NewSpeechmaticsAdapter(name, baseURL, apiKey string) *SpeechmaticsAdapter {
	return &SpeechmaticsAdapter{
		name:    name,
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
	}
}

func (a *SpeechmaticsAdapter) Name() string { return a.name }

// Chat exists only to satisfy Adapter. Speechmatics transcribes and does
// nothing else, so a route that points chat traffic here is a configuration
// mistake and should say so rather than fail somewhere further down.
func (a *SpeechmaticsAdapter) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	return ChatResponse{}, fmt.Errorf("%s: chat is not supported by the Speechmatics provider", a.name)
}

// speechmaticsOperatingPoint maps the model name in the route onto the accuracy
// tier. Speechmatics has no model list of its own — the choice is "enhanced"
// (accurate, slower) or "standard" (cheaper, faster) — so the alias's model
// string is what selects it, and anything unrecognised gets the accurate one,
// since a rung this far down the fallback chain exists to get the answer right.
func speechmaticsOperatingPoint(model string) string {
	if strings.Contains(strings.ToLower(model), "standard") {
		return "standard"
	}
	return "enhanced"
}

// Transcribe implements TranscriptionAdapter.
func (a *SpeechmaticsAdapter) Transcribe(ctx context.Context, req TranscriptionRequest) (TranscriptionResponse, error) {
	if len(req.Audio) == 0 {
		return TranscriptionResponse{}, fmt.Errorf("%s: empty audio", a.name)
	}

	jobID, err := a.submit(ctx, req)
	if err != nil {
		return TranscriptionResponse{}, err
	}
	if err := a.await(ctx, jobID); err != nil {
		return TranscriptionResponse{}, err
	}

	text, err := a.fetchText(ctx, jobID)
	if err != nil {
		return TranscriptionResponse{}, err
	}
	out := TranscriptionResponse{Text: text, Language: req.Language}

	// Word timings live in a different representation, so they cost a second
	// fetch. Only pay for it when the caller asked for timestamps.
	if len(req.Granularities) > 0 {
		if segs, dur, lang, err := a.fetchSegments(ctx, jobID); err == nil {
			out.Segments = segs
			out.Duration = dur
			if lang != "" {
				out.Language = lang
			}
		}
	}

	if out.Text == "" && len(out.Segments) == 0 {
		return TranscriptionResponse{}, fmt.Errorf("%s: empty transcription", a.name)
	}
	return out, nil
}

// submit posts the audio and the job config, and returns the job id.
func (a *SpeechmaticsAdapter) submit(ctx context.Context, req TranscriptionRequest) (string, error) {
	transcriptionCfg := map[string]any{
		"operating_point": speechmaticsOperatingPoint(req.Model),
	}
	cfg := map[string]any{
		"type":                 "transcription",
		"transcription_config": transcriptionCfg,
	}
	if req.Language != "" {
		transcriptionCfg["language"] = req.Language
	} else {
		// No language given: let Speechmatics identify it rather than default
		// to English, which would return confident nonsense for Persian audio.
		transcriptionCfg["language"] = "auto"
		cfg["language_identification_config"] = map[string]any{}
	}
	if req.Prompt != "" {
		// The prompt is a vocabulary hint everywhere else in this package, and
		// Speechmatics takes it the same way: words it should expect to hear.
		var words []any
		for _, w := range strings.Fields(req.Prompt) {
			words = append(words, map[string]any{"content": w})
		}
		if len(words) > 0 {
			transcriptionCfg["additional_vocab"] = words
		}
	}
	if len(req.Granularities) > 0 {
		transcriptionCfg["diarization"] = "speaker"
	}

	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("data_file", normalizeAudioFilename(req.Filename))
	if err != nil {
		return "", err
	}
	if _, err := part.Write(req.Audio); err != nil {
		return "", err
	}
	if err := form.WriteField("config", string(cfgJSON)); err != nil {
		return "", err
	}
	if err := form.Close(); err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/jobs/", &body)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", form.FormDataContentType())
	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := transcribeHTTPClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("%s: job rejected (status %d): %s", a.name, resp.StatusCode, truncate(raw))
	}

	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.ID == "" {
		return "", fmt.Errorf("%s: job submission gave no id: %s", a.name, truncate(raw))
	}
	return parsed.ID, nil
}

// await polls the job until it leaves "running". The request context is the
// real deadline; this loop only decides how often to ask.
func (a *SpeechmaticsAdapter) await(ctx context.Context, jobID string) error {
	delay := 500 * time.Millisecond
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s: job %s still running when the request expired: %w", a.name, jobID, ctx.Err())
		case <-time.After(delay):
		}
		if delay < speechmaticsPollCeiling {
			delay *= 2
			if delay > speechmaticsPollCeiling {
				delay = speechmaticsPollCeiling
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/jobs/"+jobID, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+a.apiKey)
		resp, err := transcribeHTTPClient.Do(req)
		if err != nil {
			return err
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			return fmt.Errorf("%s: cannot read job %s (status %d): %s", a.name, jobID, resp.StatusCode, truncate(raw))
		}

		var parsed struct {
			Job struct {
				Status string `json:"status"`
				Errors []struct {
					Message string `json:"message"`
				} `json:"errors"`
				Duration int `json:"duration"`
			} `json:"job"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return fmt.Errorf("%s: unreadable job status: %s", a.name, truncate(raw))
		}
		switch parsed.Job.Status {
		case "done":
			return nil
		case "running":
			continue
		default:
			msg := parsed.Job.Status
			if len(parsed.Job.Errors) > 0 {
				msg += ": " + parsed.Job.Errors[0].Message
			}
			return fmt.Errorf("%s: job %s %s", a.name, jobID, msg)
		}
	}
}

func (a *SpeechmaticsAdapter) transcript(ctx context.Context, jobID, format string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/jobs/"+jobID+"/transcript?format="+format, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	resp, err := transcribeHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%s: cannot fetch transcript for %s (status %d): %s", a.name, jobID, resp.StatusCode, truncate(raw))
	}
	return raw, nil
}

func (a *SpeechmaticsAdapter) fetchText(ctx context.Context, jobID string) (string, error) {
	raw, err := a.transcript(ctx, jobID, "txt")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

// fetchSegments reads the json-v2 transcript for word timings. Speechmatics
// emits one result per token, punctuation included, so words are grouped into
// one segment per speaker turn — closer to what a caller expects from
// "segments" than a list of bare words would be.
func (a *SpeechmaticsAdapter) fetchSegments(ctx context.Context, jobID string) ([]TranscriptionSegment, float64, string, error) {
	raw, err := a.transcript(ctx, jobID, "json-v2")
	if err != nil {
		return nil, 0, "", err
	}
	var parsed struct {
		Metadata struct {
			Language string `json:"language"`
		} `json:"metadata"`
		Results []struct {
			Type         string  `json:"type"`
			StartTime    float64 `json:"start_time"`
			EndTime      float64 `json:"end_time"`
			AttachesTo   string  `json:"attaches_to"`
			Alternatives []struct {
				Content string `json:"content"`
				Speaker string `json:"speaker"`
			} `json:"alternatives"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, 0, "", err
	}

	var (
		segs    []TranscriptionSegment
		cur     *TranscriptionSegment
		speaker string
		end     float64
	)
	for _, r := range parsed.Results {
		if len(r.Alternatives) == 0 {
			continue
		}
		alt := r.Alternatives[0]
		if r.EndTime > end {
			end = r.EndTime
		}
		if r.Type == "punctuation" && cur != nil {
			// Punctuation attaches to the previous word rather than standing
			// alone, so it gets no space in front of it.
			cur.Text += alt.Content
			cur.End = r.EndTime
			continue
		}
		if cur == nil || alt.Speaker != speaker {
			segs = append(segs, TranscriptionSegment{
				ID: len(segs), Start: r.StartTime, End: r.EndTime, Text: alt.Content,
			})
			cur = &segs[len(segs)-1]
			speaker = alt.Speaker
			continue
		}
		cur.Text += " " + alt.Content
		cur.End = r.EndTime
	}
	return segs, end, parsed.Metadata.Language, nil
}
