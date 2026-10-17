package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Claude Opus 5.5 / Fable 5.1 answer temperature and top_p with a 400, so a
// reasoning-style target must not forward them; a classic target still must.
func TestAnthropicReasoningStyleDropsSampling(t *testing.T) {
	for _, tc := range []struct {
		style string
		want  bool
	}{{"", true}, {ParamStyleReasoning, false}} {
		var got map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&got)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
		}))
		temp, topP := 0.2, 0.9
		req := ChatRequest{
			Model:       "claude-opus-5-5",
			Messages:    []Message{{Role: "user", Content: "hello"}},
			Temperature: &temp,
			TopP:        &topP,
			ParamStyle:  tc.style,
		}
		if _, err := NewAnthropicAdapter("anthropic", srv.URL, "k").Chat(context.Background(), req); err != nil {
			t.Fatal(err)
		}
		srv.Close()
		_, hasT := got["temperature"]
		_, hasP := got["top_p"]
		if hasT != tc.want || hasP != tc.want {
			t.Errorf("style %q: temperature=%v top_p=%v, want both %v", tc.style, hasT, hasP, tc.want)
		}
	}
}
