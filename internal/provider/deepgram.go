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
)

// --- Deepgram (pre-recorded /v1/listen) ---
//
// The simplest transcription upstream here: no multipart, no job queue. The
// audio is the request body, the options are query parameters, and the answer
// comes back on the same call. Everything unusual about it is in three places:
//
//   - The auth scheme is "Token", not "Bearer". A bearer token returns 401 with
//     a body that does not mention the header.
//   - The MIME type of the body is how it learns the container. Getting it
//     wrong is a 400 about "corrupt or unsupported data", not about the header.
//   - The transcript is buried three levels deep in a results structure built
//     for multi-channel audio, and reads empty rather than erroring when the
//     model heard nothing.

// deepgramResponse is the pre-recorded shape, trimmed to what a transcript
// needs. Deepgram nests by channel then by alternative because it can be asked
// for several of each; we ask for neither, so the first of each is the answer.
type deepgramResponse struct {
	Metadata struct {
		Duration float64 `json:"duration"`
	} `json:"metadata"`
	Results struct {
		Channels []struct {
			Alternatives []struct {
				Transcript string `json:"transcript"`
				Words      []struct {
					Word           string  `json:"word"`
					PunctuatedWord string  `json:"punctuated_word"`
					Start          float64 `json:"start"`
					End            float64 `json:"end"`
					Speaker        *int    `json:"speaker"`
				} `json:"words"`
			} `json:"alternatives"`
			DetectedLanguage string `json:"detected_language"`
		} `json:"channels"`
	} `json:"results"`
	Err string `json:"err_msg"`
}

// DeepgramAdapter speaks Deepgram's pre-recorded API.
type DeepgramAdapter struct {
	name    string
	baseURL string
	apiKey  string
}

// NewDeepgramAdapter builds the adapter. baseURL defaults to the public API.
func NewDeepgramAdapter(name, baseURL, apiKey string) *DeepgramAdapter {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.deepgram.com/v1"
	}
	return &DeepgramAdapter{
		name:    name,
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
	}
}

func (a *DeepgramAdapter) Name() string { return a.name }

// Chat exists only to satisfy Adapter. Deepgram transcribes and nothing else,
// so a route pointing chat traffic here is a configuration mistake that should
// say so rather than fail further down.
func (a *DeepgramAdapter) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	return ChatResponse{}, fmt.Errorf("%s: chat is not supported by the Deepgram provider", a.name)
}

// Transcribe implements TranscriptionAdapter.
func (a *DeepgramAdapter) Transcribe(ctx context.Context, req TranscriptionRequest) (TranscriptionResponse, error) {
	if len(req.Audio) == 0 {
		return TranscriptionResponse{}, fmt.Errorf("%s: empty audio", a.name)
	}

	q := url.Values{}
	if req.Model != "" {
		q.Set("model", req.Model)
	}
	// smart_format is punctuation, casing and number formatting. Without it the
	// transcript is one unbroken lowercase run, which is technically the words
	// and practically unusable.
	q.Set("smart_format", "true")
	if req.Language != "" {
		q.Set("language", req.Language)
	} else {
		// Guessing English on Persian audio returns confident nonsense, so let
		// Deepgram identify the language when the caller did not name one.
		q.Set("detect_language", "true")
	}
	if len(req.Granularities) > 0 {
		q.Set("diarize", "true")
	}
	if req.Prompt != "" {
		// Keyterms bias the model toward names and jargon it would otherwise
		// spell phonetically — the same job `prompt` does on the other adapters.
		for _, term := range strings.Fields(req.Prompt) {
			q.Add("keyterm", term)
		}
	}

	endpoint := a.baseURL + "/listen?" + q.Encode()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(req.Audio))
	if err != nil {
		return TranscriptionResponse{}, err
	}
	// "Token", not "Bearer".
	httpReq.Header.Set("Authorization", "Token "+a.apiKey)
	// The body's MIME type is how Deepgram learns the container; it reads no
	// filename. audioMIMEFromFilename is shared with the Gemini adapter.
	httpReq.Header.Set("Content-Type", audioMIMEFromFilename(req.Filename))

	resp, err := transcribeHTTPClient.Do(httpReq)
	if err != nil {
		return TranscriptionResponse{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return TranscriptionResponse{}, fmt.Errorf("%s: upstream error (status %d): %s", a.name, resp.StatusCode, truncate(raw))
	}

	var parsed deepgramResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return TranscriptionResponse{}, fmt.Errorf("%s: unreadable response: %s", a.name, truncate(raw))
	}
	if parsed.Err != "" {
		return TranscriptionResponse{}, fmt.Errorf("%s: %s", a.name, parsed.Err)
	}
	if len(parsed.Results.Channels) == 0 || len(parsed.Results.Channels[0].Alternatives) == 0 {
		return TranscriptionResponse{}, fmt.Errorf("%s: empty transcription", a.name)
	}

	channel := parsed.Results.Channels[0]
	alt := channel.Alternatives[0]
	out := TranscriptionResponse{
		Text:     strings.TrimSpace(alt.Transcript),
		Language: channel.DetectedLanguage,
		Duration: parsed.Metadata.Duration,
	}
	if out.Language == "" {
		out.Language = req.Language
	}

	// One segment per speaker turn. Without diarization every word carries no
	// speaker, so they collapse into a single segment — which is the right
	// answer for a caller who asked for timestamps but not for speakers.
	var cur *TranscriptionSegment
	speaker := -1
	for _, w := range alt.Words {
		text := w.PunctuatedWord
		if text == "" {
			text = w.Word
		}
		who := -1
		if w.Speaker != nil {
			who = *w.Speaker
		}
		if cur == nil || who != speaker {
			out.Segments = append(out.Segments, TranscriptionSegment{
				ID: len(out.Segments), Start: w.Start, End: w.End, Text: text,
			})
			cur = &out.Segments[len(out.Segments)-1]
			speaker = who
			continue
		}
		cur.Text += " " + text
		cur.End = w.End
	}

	// Deepgram answers 200 with an empty transcript when it heard no speech.
	// Treating that as success lets a broken upstream erase an archive one item
	// at a time, and the router can only fail over if this says so.
	if out.Text == "" && len(out.Segments) == 0 {
		return TranscriptionResponse{}, fmt.Errorf("%s: empty transcription", a.name)
	}
	return out, nil
}
