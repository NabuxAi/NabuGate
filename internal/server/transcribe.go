package server

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"nabugate/internal/provider"
)

// maxAudioUpload caps a single transcription upload.
//
// 100 MB is above OpenAI's own 25 MB limit on purpose: a caller that sends a
// larger file should hear it from the upstream that actually enforces it,
// naming the real limit, rather than from a gateway guess. What this cap is
// for is refusing something absurd before it is read into memory.
const maxAudioUpload = 100 << 20

// handleTranscription implements POST /v1/audio/transcriptions.
//
// Multipart, because that is what the OpenAI wire format specifies and what
// every client library already sends: `file`, `model`, and the usual optional
// fields. `model` is a gateway alias, exactly as it is everywhere else here.
func (s *Server) handleTranscription(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "expected multipart/form-data with a 'file' part")
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	alias := strings.TrimSpace(r.FormValue("model"))
	if alias == "" {
		writeError(w, http.StatusBadRequest, "field 'model' (a transcription alias) is required")
		return
	}
	if !s.aliasAllowed(w, r, alias) {
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "field 'file' is required")
		return
	}
	defer file.Close()

	if header.Size > maxAudioUpload {
		writeError(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("audio is %d bytes; the limit is %d", header.Size, maxAudioUpload))
		return
	}
	audio, err := io.ReadAll(io.LimitReader(file, maxAudioUpload+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read the uploaded file")
		return
	}
	if len(audio) == 0 {
		writeError(w, http.StatusBadRequest, "the uploaded file is empty")
		return
	}
	if len(audio) > maxAudioUpload {
		writeError(w, http.StatusRequestEntityTooLarge, "audio exceeds the upload limit")
		return
	}

	req := provider.TranscriptionRequest{
		Audio:    audio,
		Filename: header.Filename,
		Language: strings.TrimSpace(r.FormValue("language")),
		Prompt:   r.FormValue("prompt"),
	}
	if t := strings.TrimSpace(r.FormValue("temperature")); t != "" {
		if v, err := strconv.ParseFloat(t, 64); err == nil {
			req.Temperature = &v
		}
	}
	// Both spellings: the API documents the bracketed form, and enough clients
	// send the bare one that rejecting it would be a papercut with no upside.
	if r.MultipartForm != nil {
		req.Granularities = append(req.Granularities, r.MultipartForm.Value["timestamp_granularities[]"]...)
		req.Granularities = append(req.Granularities, r.MultipartForm.Value["timestamp_granularities"]...)
	}

	result, err := s.router.Transcribe(r.Context(), alias, req)
	if err != nil {
		writeError(w, aliasErrStatus(err, "unknown transcription alias"), err.Error())
		return
	}

	// Audio is billed by duration upstream, and a transcript's token count says
	// nothing about what it cost. Record the seconds so the console's numbers
	// mean something for this capability too.
	usage := result.Usage
	if usage.TotalTokens == 0 && result.Duration > 0 {
		usage.TotalTokens = int(result.Duration)
	}
	s.record(r, result.Provider, result.Model, usage)

	w.Header().Set("X-Nabu-Provider", result.Provider)
	w.Header().Set("X-Nabu-Model", result.Model)

	format := strings.TrimSpace(r.FormValue("response_format"))
	if format == "text" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, result.Text)
		return
	}

	// The OpenAI verbose_json shape, so an existing client needs no changes.
	out := map[string]any{"text": result.Text}
	if format == "verbose_json" || len(result.Segments) > 0 {
		out["language"] = result.Language
		out["duration"] = result.Duration
		segments := make([]map[string]any, 0, len(result.Segments))
		for _, seg := range result.Segments {
			segments = append(segments, map[string]any{
				"id": seg.ID, "start": seg.Start, "end": seg.End, "text": seg.Text,
			})
		}
		out["segments"] = segments
	}
	writeJSON(w, http.StatusOK, out)
}
