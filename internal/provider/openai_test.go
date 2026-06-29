package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// TestOpenAIChatPassthrough verifies that arbitrary OpenAI params (here: tools)
// are forwarded to the upstream verbatim, that the alias model is overridden
// with the resolved upstream model, that "stream" is absent on the non-stream
// path, and that tool_calls + finish_reason come back in the normalized response.
func TestOpenAIChatPassthrough(t *testing.T) {
	var gotBody map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"f","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`))
	}))
	defer srv.Close()

	a := NewOpenAIAdapter("openai", srv.URL, "k", nil)
	raw := map[string]json.RawMessage{
		"model":       json.RawMessage(`"nabu-fast"`),
		"messages":    json.RawMessage(`[{"role":"user","content":"hi"}]`),
		"tools":       json.RawMessage(`[{"type":"function","function":{"name":"f"}}]`),
		"temperature": json.RawMessage(`0.5`),
	}
	resp, err := a.Chat(context.Background(), ChatRequest{Model: "gpt-4o-mini", Raw: raw})
	if err != nil {
		t.Fatal(err)
	}

	var gotModel string
	_ = json.Unmarshal(gotBody["model"], &gotModel)
	if gotModel != "gpt-4o-mini" {
		t.Fatalf("model not overridden to upstream: %q", gotModel)
	}
	if _, ok := gotBody["tools"]; !ok {
		t.Fatal("tools were not forwarded upstream")
	}
	if _, ok := gotBody["stream"]; ok {
		t.Fatal("stream must be absent on the non-stream path")
	}
	if len(resp.ToolCalls) == 0 {
		t.Fatal("tool_calls missing from response")
	}
	if resp.FinishReason != "tool_calls" {
		t.Fatalf("finish_reason = %q, want tool_calls", resp.FinishReason)
	}
	if resp.Usage.TotalTokens != 7 {
		t.Fatalf("usage = %+v", resp.Usage)
	}
}

// TestOpenAIChatRetry verifies a transient 5xx is retried and then succeeds.
func TestOpenAIChatRetry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"message":"busy"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"total_tokens":1}}`))
	}))
	defer srv.Close()

	a := NewOpenAIAdapter("openai", srv.URL, "k", nil)
	resp, err := a.Chat(context.Background(), ChatRequest{
		Model: "m",
		Raw:   map[string]json.RawMessage{"messages": json.RawMessage(`[{"role":"user","content":"x"}]`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "ok" {
		t.Fatalf("content = %q", resp.Content)
	}
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Fatalf("expected 2 upstream calls (1 retry), got %d", n)
	}
}

// TestOpenAIChatFallbackBody verifies the typed-field fallback when no Raw is set.
func TestOpenAIChatFallbackBody(t *testing.T) {
	var gotBody map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hi"},"finish_reason":"stop"}],"usage":{"total_tokens":1}}`))
	}))
	defer srv.Close()

	a := NewOpenAIAdapter("openai", srv.URL, "k", nil)
	temp := 0.2
	_, err := a.Chat(context.Background(), ChatRequest{
		Model:       "m",
		Messages:    []Message{{Role: "user", Content: "hi"}},
		Temperature: &temp,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := gotBody["messages"]; !ok {
		t.Fatal("messages not built from typed fields")
	}
	if _, ok := gotBody["temperature"]; !ok {
		t.Fatal("temperature not built from typed fields")
	}
}
