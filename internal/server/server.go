// Package server exposes the OpenAI-compatible HTTP API that projects call.
package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"nabugate/internal/provider"
	"nabugate/internal/router"
)

// Server wires the router and auth into an http.Handler.
type Server struct {
	router  *router.Router
	apiKeys map[string]bool
	log     *slog.Logger
}

// New builds a Server. If apiKeys is empty, authentication is disabled (dev
// mode) and a warning is logged by the caller.
func New(r *router.Router, apiKeys []string, log *slog.Logger) *Server {
	keys := make(map[string]bool, len(apiKeys))
	for _, k := range apiKeys {
		if k != "" {
			keys[k] = true
		}
	}
	return &Server{router: r, apiKeys: keys, log: log}
}

// Handler returns the root http.Handler with routes and middleware applied.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /v1/models", s.auth(s.handleModels))
	mux.HandleFunc("POST /v1/chat/completions", s.auth(s.handleChat))
	mux.HandleFunc("POST /v1/images/generations", s.auth(s.handleImages))
	mux.HandleFunc("POST /v1/audio/speech", s.auth(s.handleSpeech))
	mux.HandleFunc("POST /v1/embeddings", s.auth(s.handleEmbeddings))
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleModels(w http.ResponseWriter, _ *http.Request) {
	raw, err := s.router.MarshalAliases()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

// chatRequestBody is the OpenAI-compatible request projects send. "model" is a
// NabuGate alias (e.g. "nabu-fast"), not a real upstream model.
type chatRequestBody struct {
	Model       string             `json:"model"`
	Messages    []provider.Message `json:"messages"`
	Temperature *float64           `json:"temperature"`
	MaxTokens   *int               `json:"max_tokens"`
	Stream      bool               `json:"stream"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var body chatRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Model == "" {
		writeError(w, http.StatusBadRequest, "field 'model' (alias) is required")
		return
	}
	if len(body.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "field 'messages' must not be empty")
		return
	}

	chatReq := provider.ChatRequest{
		Messages:    body.Messages,
		Temperature: body.Temperature,
		MaxTokens:   body.MaxTokens,
	}
	if body.Stream {
		s.streamChat(w, r, body.Model, chatReq)
		return
	}

	result, err := s.router.Chat(r.Context(), body.Model, provider.ChatRequest{
		Messages:    body.Messages,
		Temperature: body.Temperature,
		MaxTokens:   body.MaxTokens,
	})
	if err != nil {
		// Unknown alias is a client error; everything else is upstream/bad gateway.
		status := http.StatusBadGateway
		if strings.HasPrefix(err.Error(), "unknown model alias") {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	w.Header().Set("X-Nabu-Provider", result.Provider)
	w.Header().Set("X-Nabu-Model", result.Model)

	resp := map[string]any{
		"id":             "nabu-" + fmt.Sprint(time.Now().UnixNano()),
		"object":         "chat.completion",
		"created":        time.Now().Unix(),
		"model":          result.Alias,
		"provider":       result.Provider,
		"upstream_model": result.Model,
		"choices": []map[string]any{{
			"index":         0,
			"finish_reason": "stop",
			"message": map[string]string{
				"role":    "assistant",
				"content": result.Response.Content,
			},
		}},
		"usage": map[string]int{
			"prompt_tokens":     result.Response.Usage.PromptTokens,
			"completion_tokens": result.Response.Usage.CompletionTokens,
			"total_tokens":      result.Response.Usage.TotalTokens,
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

// streamChat streams a chat completion as OpenAI-style SSE chunks. Response
// headers (including the chosen provider) are written lazily on the first delta
// so that, if every target fails before producing output, we can still return a
// normal JSON error with the right status code.
func (s *Server) streamChat(w http.ResponseWriter, r *http.Request, alias string, req provider.ChatRequest) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	id := "nabu-" + fmt.Sprint(time.Now().UnixNano())
	created := time.Now().Unix()
	var metaProvider, metaModel string
	headersWritten := false

	writeSSE := func(v any) {
		payload, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", payload)
		flusher.Flush()
	}
	startHeaders := func() {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.Header().Set("X-Nabu-Provider", metaProvider)
		w.Header().Set("X-Nabu-Model", metaModel)
		w.WriteHeader(http.StatusOK)
		headersWritten = true
		writeSSE(streamChunk(id, created, alias, metaProvider, metaModel, map[string]any{"role": "assistant"}, nil))
	}

	result, err := s.router.ChatStream(r.Context(), alias, req,
		func(p, m string) { metaProvider, metaModel = p, m },
		func(delta string) error {
			if !headersWritten {
				startHeaders()
			}
			writeSSE(streamChunk(id, created, alias, metaProvider, metaModel, map[string]any{"content": delta}, nil))
			return nil
		},
	)

	if !headersWritten {
		if err != nil {
			writeError(w, aliasErrStatus(err, "unknown model alias"), err.Error())
			return
		}
		startHeaders() // succeeded but produced no text; emit an empty stream
	}

	finish := "stop"
	if err != nil {
		finish = "error"
	}
	writeSSE(streamChunk(id, created, alias, result.Provider, result.Model, map[string]any{}, &finish))
	fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// streamChunk builds one OpenAI-style chat.completion.chunk object.
func streamChunk(id string, created int64, alias, prov, model string, delta map[string]any, finish *string) map[string]any {
	return map[string]any{
		"id":             id,
		"object":         "chat.completion.chunk",
		"created":        created,
		"model":          alias,
		"provider":       prov,
		"upstream_model": model,
		"choices": []map[string]any{{
			"index":         0,
			"delta":         delta,
			"finish_reason": finish,
		}},
	}
}

// imageRequestBody is the OpenAI-compatible image request. "model" is an alias.
type imageRequestBody struct {
	Model       string `json:"model"`
	Prompt      string `json:"prompt"`
	N           int    `json:"n"`
	Size        string `json:"size"`
	AspectRatio string `json:"aspect_ratio"`
}

func (s *Server) handleImages(w http.ResponseWriter, r *http.Request) {
	var body imageRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Model == "" || body.Prompt == "" {
		writeError(w, http.StatusBadRequest, "fields 'model' (alias) and 'prompt' are required")
		return
	}

	result, err := s.router.Image(r.Context(), body.Model, provider.ImageRequest{
		Prompt:      body.Prompt,
		N:           body.N,
		Size:        body.Size,
		AspectRatio: body.AspectRatio,
	})
	if err != nil {
		writeError(w, aliasErrStatus(err, "unknown image alias"), err.Error())
		return
	}

	w.Header().Set("X-Nabu-Provider", result.Provider)
	w.Header().Set("X-Nabu-Model", result.Model)

	data := make([]map[string]string, 0, len(result.Images))
	for _, b64 := range result.Images {
		data = append(data, map[string]string{"b64_json": b64})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"created":        time.Now().Unix(),
		"model":          result.Alias,
		"provider":       result.Provider,
		"upstream_model": result.Model,
		"data":           data,
	})
}

// speechRequestBody is the OpenAI-compatible speech request. "model" is an alias.
type speechRequestBody struct {
	Model          string `json:"model"`
	Input          string `json:"input"`
	Voice          string `json:"voice"`
	ResponseFormat string `json:"response_format"`
}

func (s *Server) handleSpeech(w http.ResponseWriter, r *http.Request) {
	var body speechRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Model == "" || body.Input == "" {
		writeError(w, http.StatusBadRequest, "fields 'model' (alias) and 'input' are required")
		return
	}

	result, err := s.router.Speech(r.Context(), body.Model, provider.SpeechRequest{
		Input:  body.Input,
		Voice:  body.Voice,
		Format: body.ResponseFormat,
	})
	if err != nil {
		writeError(w, aliasErrStatus(err, "unknown audio alias"), err.Error())
		return
	}

	// OpenAI's /v1/audio/speech returns raw audio bytes, so we do too.
	w.Header().Set("X-Nabu-Provider", result.Provider)
	w.Header().Set("X-Nabu-Model", result.Model)
	w.Header().Set("Content-Type", result.ContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(result.Audio)
}

// embeddingRequestBody is the OpenAI-compatible embeddings request. "input"
// may be a single string or an array of strings.
type embeddingRequestBody struct {
	Model string          `json:"model"`
	Input json.RawMessage `json:"input"`
}

func (s *Server) handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	var body embeddingRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Model == "" {
		writeError(w, http.StatusBadRequest, "field 'model' (alias) is required")
		return
	}

	// Accept both string and []string for "input".
	var inputs []string
	if err := json.Unmarshal(body.Input, &inputs); err != nil {
		var single string
		if err2 := json.Unmarshal(body.Input, &single); err2 != nil {
			writeError(w, http.StatusBadRequest, "field 'input' must be a string or array of strings")
			return
		}
		inputs = []string{single}
	}
	if len(inputs) == 0 {
		writeError(w, http.StatusBadRequest, "field 'input' must not be empty")
		return
	}

	result, err := s.router.Embed(r.Context(), body.Model, provider.EmbeddingRequest{Input: inputs})
	if err != nil {
		writeError(w, aliasErrStatus(err, "unknown embedding alias"), err.Error())
		return
	}

	w.Header().Set("X-Nabu-Provider", result.Provider)
	w.Header().Set("X-Nabu-Model", result.Model)

	data := make([]map[string]any, 0, len(result.Embeddings))
	for i, vec := range result.Embeddings {
		data = append(data, map[string]any{"object": "embedding", "index": i, "embedding": vec})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"object":         "list",
		"model":          result.Alias,
		"provider":       result.Provider,
		"upstream_model": result.Model,
		"data":           data,
		"usage": map[string]int{
			"prompt_tokens": result.Usage.PromptTokens,
			"total_tokens":  result.Usage.TotalTokens,
		},
	})
}

// aliasErrStatus maps an unknown-alias error to 400 and everything else to 502.
func aliasErrStatus(err error, unknownPrefix string) int {
	if strings.HasPrefix(err.Error(), unknownPrefix) {
		return http.StatusBadRequest
	}
	return http.StatusBadGateway
}

// auth wraps a handler with bearer-token checking against the configured
// internal API keys. When no keys are configured, requests pass through.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if len(s.apiKeys) > 0 {
			token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !s.apiKeys[strings.TrimSpace(token)] {
				writeError(w, http.StatusUnauthorized, "invalid or missing API key")
				return
			}
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"message": msg},
	})
}
