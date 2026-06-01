// Package server exposes the OpenAI-compatible HTTP API that projects call.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"nabugate/internal/policy"
	"nabugate/internal/provider"
	"nabugate/internal/router"
)

type policyCtxKey struct{}

// Server wires the router, auth and policy into an http.Handler.
type Server struct {
	router *router.Router
	policy *policy.Enforcer
	log    *slog.Logger
}

// New builds a Server. If the enforcer has no keys, authentication is disabled
// (dev mode) and a warning is logged by the caller.
func New(r *router.Router, enforcer *policy.Enforcer, log *slog.Logger) *Server {
	return &Server{router: r, policy: enforcer, log: log}
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

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	pol, hasPol := r.Context().Value(policyCtxKey{}).(policy.Policy)
	data := make([]map[string]string, 0)
	for _, a := range s.router.AliasInfos() {
		if s.policy.Enabled() && hasPol && !pol.Allows(a.ID) {
			continue // hide aliases this key may not use
		}
		data = append(data, map[string]string{"id": a.ID, "object": "model", "owned_by": a.Owner})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
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
	if !s.aliasAllowed(w, r, body.Model) {
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
	if !s.aliasAllowed(w, r, body.Model) {
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
	if !s.aliasAllowed(w, r, body.Model) {
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
	if !s.aliasAllowed(w, r, body.Model) {
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

// auth validates the bearer token, enforces the per-key rate limit, and stores
// the resolved policy in the request context for later alias checks. When no
// keys are configured, requests pass through (dev mode).
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.policy.Enabled() {
			next(w, r)
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		pol, ok := s.policy.Lookup(token)
		if !ok {
			writeError(w, http.StatusUnauthorized, "invalid or missing API key")
			return
		}
		if !s.policy.RateOK(token) {
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), policyCtxKey{}, pol)))
	}
}

// aliasAllowed reports whether the request's key may use the given alias, and
// writes a 403 if not. Returns true when policy is disabled.
func (s *Server) aliasAllowed(w http.ResponseWriter, r *http.Request, alias string) bool {
	if !s.policy.Enabled() {
		return true
	}
	pol, ok := r.Context().Value(policyCtxKey{}).(policy.Policy)
	if ok && pol.Allows(alias) {
		return true
	}
	writeError(w, http.StatusForbidden, fmt.Sprintf("alias %q is not permitted for this key", alias))
	return false
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
