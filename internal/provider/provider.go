// Package provider defines the common request/response shapes used across all
// upstream LLM providers and the Adapter interface each provider implements.
package provider

import (
	"context"
	"net/http"
	"time"
)

// Message is a single chat message in the unified (OpenAI-style) format.
type Message struct {
	Role    string `json:"role"`    // "system" | "user" | "assistant"
	Content string `json:"content"` // plain text content
}

// ChatRequest is the provider-agnostic chat request. Model is the *upstream*
// model name (already resolved from an alias by the router).
type ChatRequest struct {
	Model       string
	Messages    []Message
	Temperature *float64
	MaxTokens   *int
}

// Usage captures token accounting returned by the upstream provider.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatResponse is the normalized response returned by every adapter.
type ChatResponse struct {
	Content string
	Usage   Usage
}

// Adapter converts the unified request into a provider's native API call and
// normalizes the response back. One Adapter instance maps to one configured
// provider (e.g. "openai", "groq", "anthropic").
type Adapter interface {
	// Name returns the configured provider name.
	Name() string
	// Chat performs a single non-streaming chat completion.
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}

// sharedHTTPClient is reused by all adapters; upstream calls are bounded by the
// request context, so the client timeout is a generous safety net.
var sharedHTTPClient = &http.Client{Timeout: 120 * time.Second}
