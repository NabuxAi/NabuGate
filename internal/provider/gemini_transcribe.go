package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

// --- Transcription (Google Gemini) ---
//
// Gemini offers two different ways to turn speech into text, and which one
// applies depends on the model:
//
//   - The dedicated transcription models (gemini-3.5-transcribe) answer on the
//     Interactions API. They take the audio by reference, so the bytes must go
//     through the Files API first, and they return word-level annotations with
//     speaker labels.
//   - Every general multimodal model (gemini-3-flash and friends) will happily
//     transcribe from generateContent with the audio inlined as base64 and an
//     instruction telling it to write down what it hears. No upload, one round
//     trip, no word timings.
//
// Both are worth having. The dedicated model is the better transcriber; the
// general one is the rung that still works when the transcription model is
// unavailable in a region or the Files API is refusing uploads. Transcribe
// picks between them from the model name, so the router's fallback list can
// hold both and the caller never learns which answered.

// geminiInlineAudioLimit is the point past which inline base64 stops being
// safe. The documented request ceiling is 20MB for the whole payload, and
// base64 inflates by a third, so we hand anything larger to the Files API even
// on the generateContent path rather than send a request the API will reject.
const geminiInlineAudioLimit = 14 << 20 // 14 MiB of raw audio ≈ 19MB encoded

// geminiFileActiveTimeout bounds the wait for an uploaded file to finish
// processing. Google reports PROCESSING until the audio is decoded; referencing
// a file in that state fails, so we poll. The request context still governs the
// real deadline — this only stops an indefinite poll against a file that is
// stuck.
const geminiFileActiveTimeout = 3 * time.Minute

// audioMIMEFromFilename maps a filename onto the MIME types Gemini accepts for
// audio. An unknown extension gets audio/mpeg rather than an error: the API
// sniffs the container itself, and a wrong-but-plausible label costs less than
// refusing a file we could have transcribed.
func audioMIMEFromFilename(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".wav":
		return "audio/wav"
	case ".mp3":
		return "audio/mp3"
	case ".aiff", ".aif":
		return "audio/aiff"
	case ".aac", ".m4a", ".mp4":
		return "audio/aac"
	case ".ogg", ".oga":
		return "audio/ogg"
	case ".opus":
		return "audio/opus"
	case ".flac":
		return "audio/flac"
	case ".webm":
		return "audio/webm"
	case ".amr":
		return "audio/amr"
	default:
		return "audio/mpeg"
	}
}

// isTranscriptionModel reports whether the model answers on the Interactions
// API rather than generateContent.
func isTranscriptionModel(model string) bool {
	return strings.Contains(strings.ToLower(model), "transcribe")
}

// Transcribe implements TranscriptionAdapter.
func (a *GeminiAdapter) Transcribe(ctx context.Context, req TranscriptionRequest) (TranscriptionResponse, error) {
	if len(req.Audio) == 0 {
		return TranscriptionResponse{}, fmt.Errorf("%s: empty audio", a.name)
	}
	mime := audioMIMEFromFilename(req.Filename)

	if isTranscriptionModel(req.Model) {
		return a.transcribeInteraction(ctx, req, mime)
	}
	return a.transcribeGenerate(ctx, req, mime)
}

// --- the dedicated transcription models ---

type geminiInteractionResponse struct {
	OutputText string `json:"output_text"`
	Steps      []struct {
		Content []struct {
			Annotations []struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				Speaker     string `json:"speaker"`
				StartOffset string `json:"start_offset"`
				EndOffset   string `json:"end_offset"`
			} `json:"annotations"`
		} `json:"content"`
	} `json:"steps"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (a *GeminiAdapter) transcribeInteraction(ctx context.Context, req TranscriptionRequest, mime string) (TranscriptionResponse, error) {
	fileURI, fileName, err := a.uploadFile(ctx, req.Audio, req.Filename, mime)
	if err != nil {
		return TranscriptionResponse{}, err
	}
	// The upload is billed against a 48-hour retention window whether or not we
	// ever read it again. One transcript is all we want, so drop it instead of
	// leaving a file per call to expire on its own.
	defer a.deleteFile(context.WithoutCancel(ctx), fileName)

	transcriptionCfg := map[string]any{"mode": "smart"}
	if req.Language != "" {
		transcriptionCfg["language_codes"] = []string{req.Language}
	}
	// Word timings and speaker labels cost latency and halve the accepted input
	// length, so ask for them only when the caller wants timestamps back.
	if len(req.Granularities) > 0 {
		transcriptionCfg["enable_word_timestamps"] = true
		transcriptionCfg["enable_speaker_diarization"] = true
	}

	input := []any{map[string]any{"type": "audio", "uri": fileURI, "mime_type": mime}}
	if req.Prompt != "" {
		input = append(input, map[string]any{"type": "text", "text": req.Prompt})
	}

	payload, err := json.Marshal(map[string]any{
		"model":             req.Model,
		"input":             input,
		"generation_config": map[string]any{"transcription_config": transcriptionCfg},
	})
	if err != nil {
		return TranscriptionResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/interactions", bytes.NewReader(payload))
	if err != nil {
		return TranscriptionResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", a.apiKey)

	resp, err := transcribeHTTPClient.Do(httpReq)
	if err != nil {
		return TranscriptionResponse{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return TranscriptionResponse{}, fmt.Errorf("%s: upstream error (status %d): %s", a.name, resp.StatusCode, truncate(raw))
	}

	var parsed geminiInteractionResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return TranscriptionResponse{}, fmt.Errorf("%s: invalid response (status %d): %s", a.name, resp.StatusCode, truncate(raw))
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return TranscriptionResponse{}, fmt.Errorf("%s: %s", a.name, parsed.Error.Message)
	}

	out := TranscriptionResponse{Text: strings.TrimSpace(parsed.OutputText), Language: req.Language}
	for _, step := range parsed.Steps {
		for _, c := range step.Content {
			for _, ann := range c.Annotations {
				if ann.Type != "word_info" {
					continue
				}
				out.Segments = append(out.Segments, TranscriptionSegment{
					ID:    len(out.Segments),
					Start: parseGeminiOffset(ann.StartOffset),
					End:   parseGeminiOffset(ann.EndOffset),
					Text:  ann.Text,
				})
			}
		}
	}
	if len(out.Segments) > 0 {
		out.Duration = out.Segments[len(out.Segments)-1].End
	}
	if out.Text == "" && len(out.Segments) == 0 {
		return TranscriptionResponse{}, fmt.Errorf("%s: empty transcription", a.name)
	}
	return out, nil
}

// parseGeminiOffset reads a protobuf Duration ("12.500s") as seconds. An
// unparseable offset becomes 0 rather than an error: a missing timestamp is
// worth less than the word it belongs to, and the transcript is still correct
// without it.
func parseGeminiOffset(s string) float64 {
	s = strings.TrimSuffix(strings.TrimSpace(s), "s")
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// --- the general multimodal models ---

// geminiTranscribeInstruction is deliberately narrow. Left to itself a chat
// model summarises, translates, or prefaces the transcript with "Here is the
// transcription:" — all three corrupt an archive that stores the answer
// verbatim.
const geminiTranscribeInstruction = "Transcribe this audio verbatim. Output only the transcript text, " +
	"in the language actually spoken, with no preamble, no translation, no summary and no commentary. " +
	"If the audio contains no speech, output nothing."

func (a *GeminiAdapter) transcribeGenerate(ctx context.Context, req TranscriptionRequest, mime string) (TranscriptionResponse, error) {
	instruction := geminiTranscribeInstruction
	if req.Prompt != "" {
		instruction += "\n\nContext for names and spelling: " + req.Prompt
	}
	if req.Language != "" {
		instruction += "\n\nThe expected language is " + req.Language + "."
	}

	parts := []geminiMediaPart{{Text: instruction}}
	if len(req.Audio) > geminiInlineAudioLimit {
		// Too large to inline. The Files API takes it by reference instead, and
		// generateContent accepts a fileData part in place of inlineData.
		fileURI, fileName, err := a.uploadFile(ctx, req.Audio, req.Filename, mime)
		if err != nil {
			return TranscriptionResponse{}, err
		}
		defer a.deleteFile(context.WithoutCancel(ctx), fileName)
		return a.transcribeGenerateWithFile(ctx, req, instruction, fileURI, mime)
	}
	parts = append(parts, geminiMediaPart{InlineData: &geminiInlineData{
		MimeType: mime,
		Data:     base64.StdEncoding.EncodeToString(req.Audio),
	}})

	body := map[string]any{
		"contents":         []any{map[string]any{"role": "user", "parts": parts}},
		"generationConfig": map[string]any{"temperature": geminiTranscribeTemperature(req.Temperature)},
	}
	return a.readGenerateTranscript(ctx, req, body)
}

func (a *GeminiAdapter) transcribeGenerateWithFile(ctx context.Context, req TranscriptionRequest, instruction, fileURI, mime string) (TranscriptionResponse, error) {
	body := map[string]any{
		"contents": []any{map[string]any{"role": "user", "parts": []any{
			map[string]any{"text": instruction},
			map[string]any{"fileData": map[string]any{"mimeType": mime, "fileUri": fileURI}},
		}}},
		"generationConfig": map[string]any{"temperature": geminiTranscribeTemperature(req.Temperature)},
	}
	return a.readGenerateTranscript(ctx, req, body)
}

// geminiTranscribeTemperature defaults to 0. Transcription is not a creative
// task, and a warm model invents words the speaker never said.
func geminiTranscribeTemperature(t *float64) float64 {
	if t == nil {
		return 0
	}
	return *t
}

// readGenerateTranscript posts a generateContent body and reads the answer as a
// transcript. It does not reuse GeminiAdapter.generate because that one shares
// the two-minute HTTP client, and an hour of audio outlives it.
func (a *GeminiAdapter) readGenerateTranscript(ctx context.Context, req TranscriptionRequest, body any) (TranscriptionResponse, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return TranscriptionResponse{}, err
	}
	endpoint := fmt.Sprintf("%s/models/%s:generateContent", a.baseURL, url.PathEscape(req.Model))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return TranscriptionResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", a.apiKey)

	resp, err := transcribeHTTPClient.Do(httpReq)
	if err != nil {
		return TranscriptionResponse{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return TranscriptionResponse{}, fmt.Errorf("%s: upstream error (status %d): %s", a.name, resp.StatusCode, truncate(raw))
	}

	var parsed geminiMediaResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return TranscriptionResponse{}, fmt.Errorf("%s: invalid response (status %d): %s", a.name, resp.StatusCode, truncate(raw))
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return TranscriptionResponse{}, fmt.Errorf("%s: %s", a.name, parsed.Error.Message)
	}

	var sb strings.Builder
	for _, cand := range parsed.Candidates {
		for _, p := range cand.Content.Parts {
			sb.WriteString(p.Text)
		}
	}
	text := strings.TrimSpace(sb.String())
	if text == "" {
		return TranscriptionResponse{}, fmt.Errorf("%s: empty transcription", a.name)
	}
	return TranscriptionResponse{Text: text, Language: req.Language}, nil
}

// --- Files API ---

type geminiFile struct {
	File struct {
		Name  string `json:"name"`
		URI   string `json:"uri"`
		State string `json:"state"`
	} `json:"file"`
}

// uploadEndpoint rewrites the configured base URL into its upload counterpart:
// https://host/v1beta -> https://host/upload/v1beta/files. Deriving it keeps a
// self-hosted or proxied Gemini endpoint working, which a hard-coded Google URL
// would not.
func (a *GeminiAdapter) uploadEndpoint() (string, error) {
	u, err := url.Parse(a.baseURL)
	if err != nil {
		return "", fmt.Errorf("%s: unusable base_url %q: %w", a.name, a.baseURL, err)
	}
	u.Path = "/upload" + u.Path + "/files"
	return u.String(), nil
}

// uploadFile puts the audio through the resumable Files API and returns the URI
// to reference it by and the resource name to delete it by.
func (a *GeminiAdapter) uploadFile(ctx context.Context, audio []byte, filename, mime string) (string, string, error) {
	endpoint, err := a.uploadEndpoint()
	if err != nil {
		return "", "", err
	}
	display := normalizeAudioFilename(filename)

	// Step 1: announce the upload and collect the one-shot URL to send it to.
	start, err := json.Marshal(map[string]any{"file": map[string]any{"display_name": display}})
	if err != nil {
		return "", "", err
	}
	startReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(start))
	if err != nil {
		return "", "", err
	}
	startReq.Header.Set("x-goog-api-key", a.apiKey)
	startReq.Header.Set("X-Goog-Upload-Protocol", "resumable")
	startReq.Header.Set("X-Goog-Upload-Command", "start")
	startReq.Header.Set("X-Goog-Upload-Header-Content-Length", strconv.Itoa(len(audio)))
	startReq.Header.Set("X-Goog-Upload-Header-Content-Type", mime)
	startReq.Header.Set("Content-Type", "application/json")

	startResp, err := transcribeHTTPClient.Do(startReq)
	if err != nil {
		return "", "", err
	}
	startBody, _ := io.ReadAll(startResp.Body)
	startResp.Body.Close()
	if startResp.StatusCode >= 400 {
		return "", "", fmt.Errorf("%s: file upload refused (status %d): %s", a.name, startResp.StatusCode, truncate(startBody))
	}
	uploadURL := startResp.Header.Get("X-Goog-Upload-Url")
	if uploadURL == "" {
		return "", "", fmt.Errorf("%s: file upload returned no upload URL", a.name)
	}

	// Step 2: send the bytes and finalize in the same request.
	putReq, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, bytes.NewReader(audio))
	if err != nil {
		return "", "", err
	}
	putReq.Header.Set("Content-Length", strconv.Itoa(len(audio)))
	putReq.Header.Set("X-Goog-Upload-Offset", "0")
	putReq.Header.Set("X-Goog-Upload-Command", "upload, finalize")

	putResp, err := transcribeHTTPClient.Do(putReq)
	if err != nil {
		return "", "", err
	}
	putBody, _ := io.ReadAll(putResp.Body)
	putResp.Body.Close()
	if putResp.StatusCode >= 400 {
		return "", "", fmt.Errorf("%s: file upload failed (status %d): %s", a.name, putResp.StatusCode, truncate(putBody))
	}

	var file geminiFile
	if err := json.Unmarshal(putBody, &file); err != nil || file.File.URI == "" {
		return "", "", fmt.Errorf("%s: file upload gave no URI: %s", a.name, truncate(putBody))
	}

	if err := a.awaitFileActive(ctx, file.File.Name, file.File.State); err != nil {
		a.deleteFile(context.WithoutCancel(ctx), file.File.Name)
		return "", "", err
	}
	return file.File.URI, file.File.Name, nil
}

// awaitFileActive polls until the upload leaves PROCESSING. Referencing a file
// mid-decode is an error, so this is the difference between a working call and
// an intermittent one on longer audio.
func (a *GeminiAdapter) awaitFileActive(ctx context.Context, name, state string) error {
	if name == "" || state == "ACTIVE" {
		return nil
	}
	deadline := time.Now().Add(geminiFileActiveTimeout)
	delay := 500 * time.Millisecond
	for {
		if state == "ACTIVE" {
			return nil
		}
		if state == "FAILED" {
			return fmt.Errorf("%s: uploaded file %s failed processing", a.name, name)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: uploaded file %s still %s after %s", a.name, name, state, geminiFileActiveTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		if delay < 4*time.Second {
			delay *= 2
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/"+name, nil)
		if err != nil {
			return err
		}
		req.Header.Set("x-goog-api-key", a.apiKey)
		resp, err := transcribeHTTPClient.Do(req)
		if err != nil {
			return err
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			return fmt.Errorf("%s: cannot read file %s (status %d): %s", a.name, name, resp.StatusCode, truncate(raw))
		}
		// GET files/{id} answers with the bare file, not the {file:{…}} wrapper
		// the upload returns.
		var bare struct {
			Name  string `json:"name"`
			State string `json:"state"`
		}
		if err := json.Unmarshal(raw, &bare); err != nil {
			return fmt.Errorf("%s: unreadable file state: %s", a.name, truncate(raw))
		}
		state = bare.State
	}
}

// deleteFile is best-effort cleanup. A failure here costs a file that expires
// on its own in 48 hours, which is not worth failing a good transcript over.
func (a *GeminiAdapter) deleteFile(ctx context.Context, name string) {
	if name == "" {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, a.baseURL+"/"+name, nil)
	if err != nil {
		return
	}
	req.Header.Set("x-goog-api-key", a.apiKey)
	resp, err := transcribeHTTPClient.Do(req)
	if err != nil {
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}
