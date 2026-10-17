package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// An agent's streamed turn is often nothing but a tool call. The stream used
// to forward text only, so such a turn read as empty, every fallback was
// tried, and the caller — LangGraph, the Vercel AI SDK, the Agents SDK — got
// a 502 instead of the call it was waiting to run.
func TestAStreamedToolCallReachesTheCaller(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"role":"assistant"}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":\"Tehran\"}"}}]}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13}}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer up.Close()
	ts, _ := fidelityServer(t, up.URL, nil, nil)
	defer ts.Close()

	raw := sseChatStream(t, ts, `{"model":"nabu-fast","stream":true,"messages":[{"role":"user","content":"weather?"}],
		"tools":[{"type":"function","function":{"name":"get_weather","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}}]}`)

	var args strings.Builder
	var name, finish string
	for _, line := range strings.Split(raw, "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok || data == "[DONE]" {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					ToolCalls []struct {
						Function struct{ Name, Arguments string } `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		for _, c := range chunk.Choices {
			for _, tc := range c.Delta.ToolCalls {
				if tc.Function.Name != "" {
					name = tc.Function.Name
				}
				args.WriteString(tc.Function.Arguments)
			}
			if c.FinishReason != nil {
				finish = *c.FinishReason
			}
		}
	}
	if name != "get_weather" || args.String() != `{"city":"Tehran"}` || finish != "tool_calls" {
		t.Fatalf("name %q args %q finish %q\n%s", name, args.String(), finish, raw)
	}
}

// Pre-tokenized embedding input (LangChain's default) is refused with a code
// and the setting that fixes it, not a bare format error.
func TestTokenizedEmbeddingInputIsRefusedWithTheFix(t *testing.T) {
	ts, _ := fidelityServer(t, "http://127.0.0.1:1", nil, nil)
	defer ts.Close()
	for _, input := range []string{`[1,2,3]`, `[[1,2],[3]]`} {
		resp, err := http.Post(ts.URL+"/v1/embeddings", "application/json",
			strings.NewReader(`{"model":"nabu-fast","input":`+input+`}`))
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]map[string]any
		json.NewDecoder(resp.Body).Decode(&got)
		resp.Body.Close()
		msg, _ := got["error"]["message"].(string)
		if resp.StatusCode != http.StatusBadRequest || got["error"]["code"] != "tokenized_input" || !strings.Contains(msg, "check_embedding_ctx_length") {
			t.Errorf("input %s: %d %v", input, resp.StatusCode, got)
		}
	}
}
