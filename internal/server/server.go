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
