package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// gpt-5.x / o-series models replaced max_tokens with max_completion_tokens and
// accept only the default temperature. Sending the classic parameters is
// rejected upstream — and some proxies return an HTML error page rather than a
// JSON error, which surfaces as an empty completion and is near-impossible to
// trace. The gateway absorbs the difference so callers need not know which
// dialect sits behind an alias.

func capturedBody(t *testing.T, req ChatRequest) map[string]any {
	t.Helper()
	var got map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	adapter := NewOpenAIAdapter("test", srv.URL, "key", nil)
	if _, err := adapter.Chat(context.Background(), req); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	return got
}

func TestReasoningDialectRenamesMaxTokens(t *testing.T) {
	temp, maxTok := 0.8, 4096
	got := capturedBody(t, ChatRequest{
		ParamStyle:  ParamStyleReasoning,
		Messages:    []Message{{Role: "user", Content: "hi"}},
		Temperature: &temp,
		MaxTokens:   &maxTok,
	})

	if _, present := got["max_tokens"]; present {
		t.Error("max_tokens was sent to a reasoning model")
	}
	if got["max_completion_tokens"] != float64(4096) {
		t.Errorf("max_completion_tokens = %v, want 4096", got["max_completion_tokens"])
	}
	// Dropped rather than clamped: a caller asking for 0.8 wants variability the
	// model cannot give, and quietly sending 1 would look like compliance.
	if _, present := got["temperature"]; present {
		t.Error("a non-default temperature was sent to a reasoning model")
	}
}

func TestReasoningDialectKeepsDefaultTemperature(t *testing.T) {
	temp := 1.0
	got := capturedBody(t, ChatRequest{
		ParamStyle:  ParamStyleReasoning,
		Messages:    []Message{{Role: "user", Content: "hi"}},
		Temperature: &temp,
	})

	if got["temperature"] != float64(1) {
		t.Errorf("temperature = %v, want 1 — the one value these models accept", got["temperature"])
	}
}

func TestClassicDialectIsUntouched(t *testing.T) {
	temp, maxTok := 0.8, 4096
	got := capturedBody(t, ChatRequest{
		Messages:    []Message{{Role: "user", Content: "hi"}},
		Temperature: &temp,
		MaxTokens:   &maxTok,
	})

	if got["max_tokens"] != float64(4096) {
		t.Errorf("max_tokens = %v, want 4096 untouched", got["max_tokens"])
	}
	if got["temperature"] != 0.8 {
		t.Errorf("temperature = %v, want 0.8 untouched", got["temperature"])
	}
	if _, present := got["max_completion_tokens"]; present {
		t.Error("max_completion_tokens leaked into a classic request")
	}
}
