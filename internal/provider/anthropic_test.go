package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

// A whole agent turn: tools offered, an earlier call and its result in the
// history, an image. Each reaches Anthropic in its own block type, and a
// tool_use in the answer comes back as OpenAI tool_calls.
func TestAnthropicCarriesToolsResultsAndImages(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"thinking","thinking":""},{"type":"text","text":"checking"},{"type":"tool_use","id":"toolu_2","name":"get_weather","input":{"city":"Shiraz"}}],"stop_reason":"tool_use","usage":{"input_tokens":20,"output_tokens":9}}`))
	}))
	defer srv.Close()

	raw := map[string]json.RawMessage{
		"messages": json.RawMessage(`[
			{"role":"system","content":"be brief"},
			{"role":"user","content":[{"type":"text","text":"weather?"},{"type":"image_url","image_url":{"url":"data:image/png;base64,QUJD"}}]},
			{"role":"assistant","content":null,"tool_calls":[{"id":"toolu_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Tehran\"}"}}]},
			{"role":"tool","tool_call_id":"toolu_1","content":"sunny"},
			{"role":"user","content":"and Shiraz?"}]`),
		"tools":       json.RawMessage(`[{"type":"function","function":{"name":"get_weather","description":"w","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}}]`),
		"tool_choice": json.RawMessage(`"required"`),
	}
	resp, err := NewAnthropicAdapter("anthropic", srv.URL, "k").Chat(context.Background(),
		ChatRequest{Model: "claude-haiku-5-5", Raw: raw, ParamStyle: ParamStyleReasoning})
	if err != nil {
		t.Fatal(err)
	}

	if got["system"] != "be brief" {
		t.Errorf("system = %v", got["system"])
	}
	msgs := got["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("want user, assistant, user(tool_result + text); got %d: %v", len(msgs), msgs)
	}
	first := msgs[0].(map[string]any)["content"].([]any)
	if img := first[1].(map[string]any); img["type"] != "image" || img["source"].(map[string]any)["media_type"] != "image/png" {
		t.Errorf("image block = %v", img)
	}
	use := msgs[1].(map[string]any)["content"].([]any)[0].(map[string]any)
	if use["type"] != "tool_use" || use["id"] != "toolu_1" || use["input"].(map[string]any)["city"] != "Tehran" {
		t.Errorf("tool_use = %v", use)
	}
	result := msgs[2].(map[string]any)["content"].([]any)
	if r0 := result[0].(map[string]any); r0["type"] != "tool_result" || r0["tool_use_id"] != "toolu_1" || r0["content"] != "sunny" {
		t.Errorf("tool_result = %v", r0)
	}
	if tool := got["tools"].([]any)[0].(map[string]any); tool["name"] != "get_weather" || tool["input_schema"] == nil {
		t.Errorf("tools = %v", got["tools"])
	}
	// Forced tool use is a 400 on this generation; it is offered as auto.
	if tc := got["tool_choice"].(map[string]any); tc["type"] != "auto" {
		t.Errorf("tool_choice = %v", tc)
	}

	if resp.Content != "checking" || resp.FinishReason != "tool_calls" {
		t.Errorf("content %q finish %q", resp.Content, resp.FinishReason)
	}
	var calls []struct {
		ID       string
		Function struct{ Name, Arguments string }
	}
	if err := json.Unmarshal(resp.ToolCalls, &calls); err != nil || len(calls) != 1 ||
		calls[0].ID != "toolu_2" || calls[0].Function.Arguments != `{"city":"Shiraz"}` {
		t.Fatalf("tool_calls = %s (%v)", resp.ToolCalls, err)
	}
}

// Streamed tool_use becomes OpenAI tool_calls deltas, indexed among tool
// calls, with the argument JSON in the pieces Anthropic sent.
func TestAnthropicStreamsToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, ev := range []string{
			`{"type":"message_start","message":{"usage":{"input_tokens":11}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"hmm"}}`,
			`{"type":"content_block_start","index":1,"content_block":{"type":"text"}}`,
			`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"ok "}}`,
			`{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"toolu_9","name":"get_weather"}}`,
			`{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}`,
			`{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"\"Tabriz\"}"}}`,
			`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":6}}`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", ev)
		}
	}))
	defer srv.Close()

	var text strings.Builder
	var deltas []string
	ctx := WithToolDeltas(context.Background(), func(raw json.RawMessage) error {
		deltas = append(deltas, string(raw))
		return nil
	})
	raw := map[string]json.RawMessage{"tools": json.RawMessage(`[{"type":"function","function":{"name":"get_weather"}}]`)}
	usage, err := NewAnthropicAdapter("anthropic", srv.URL, "k").ChatStream(ctx,
		ChatRequest{Model: "claude-haiku-5-5", Messages: []Message{{Role: "user", Content: "hi"}}, Raw: raw},
		func(d string) error { text.WriteString(d); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if text.String() != "ok " {
		t.Errorf("text %q (thinking must not leak into the answer)", text.String())
	}
	want := []string{
		`[{"function":{"arguments":"","name":"get_weather"},"id":"toolu_9","index":0,"type":"function"}]`,
		`[{"function":{"arguments":"{\"city\":"},"index":0}]`,
		`[{"function":{"arguments":"\"Tabriz\"}"},"index":0}]`,
	}
	if strings.Join(deltas, "\n") != strings.Join(want, "\n") {
		t.Errorf("deltas:\n%s\nwant:\n%s", strings.Join(deltas, "\n"), strings.Join(want, "\n"))
	}
	if usage.PromptTokens != 11 || usage.CompletionTokens != 6 {
		t.Errorf("usage %+v", usage)
	}
}
