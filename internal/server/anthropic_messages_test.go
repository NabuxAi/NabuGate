package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nabugate/internal/policy"
)

func anthTestBody(t *testing.T, in map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestAnthropicMessagesWithAuthAndMeteredChat(t *testing.T) {
	up := fakeUpstream(t)
	defer up.Close()
	ts := newTestServer(t, up.URL, policy.New([]string{"test-owner-key"}, nil), nil)
	defer ts.Close()

	data := anthTestBody(t, map[string]any{
		"model": "nabu-fast", "max_tokens": 128,
		"messages": []map[string]any{{"role": "user", "content": "Hello"}},
	})
	unauthorized, err := http.Post(ts.URL+"/v1/messages", "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing credentials status=%d, want 401", unauthorized.StatusCode)
	}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/messages", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", "test-owner-key") // Anthropic SDK credential style
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status=%d, body=%s", response.StatusCode, body)
	}
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result["type"] != "message" || result["model"] != "nabu-fast" {
		t.Errorf("unexpected Anthropic message envelope: %v", result)
	}
	blocks, ok := result["content"].([]any)
	if !ok || len(blocks) == 0 || blocks[0].(map[string]any)["type"] != "text" {
		t.Errorf("missing text content blocks: %v", result["content"])
	}
}

func TestAnthropicMessagesConvertTools(t *testing.T) {
	var incoming nabuAnthropicRequest
	raw := []byte("{\"model\":\"nabu-fast\",\"max_tokens\":128,\"messages\":[{\"role\":\"user\",\"content\":\"calculate\"},{\"role\":\"assistant\",\"content\":[{\"type\":\"tool_use\",\"id\":\"call_1\",\"name\":\"calc\",\"input\":{\"a\":2}}]},{\"role\":\"user\",\"content\":[{\"type\":\"tool_result\",\"tool_use_id\":\"call_1\",\"content\":\"4\"}]}],\"tools\":[{\"name\":\"calc\",\"input_schema\":{\"type\":\"object\"}}]}")
	if err := json.Unmarshal(raw, &incoming); err != nil {
		t.Fatal(err)
	}
	converted, err := nabuAnthConvert(incoming)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(converted.Raw["messages"], []byte("\"tool_calls\"")) {
		t.Errorf("tool_use missing from converted request: %s", converted.Raw["messages"])
	}
	if !bytes.Contains(converted.Raw["messages"], []byte("\"tool_call_id\":\"call_1\"")) {
		t.Errorf("tool_result missing from converted request: %s", converted.Raw["messages"])
	}
	if !bytes.Contains(converted.Raw["tools"], []byte("\"parameters\"")) {
		t.Errorf("tool input_schema not mapped: %s", converted.Raw["tools"])
	}
}

func TestAnthropicMessagesStreamEvents(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello \"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"world\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":2,\"total_tokens\":14}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer up.Close()
	ts := newTestServer(t, up.URL, policy.New(nil, nil), nil)
	defer ts.Close()
	body := anthTestBody(t, map[string]any{
		"model": "nabu-fast", "max_tokens": 128, "stream": true,
		"messages": []map[string]any{{"role": "user", "content": "Hi"}},
	})
	resp, err := http.Post(ts.URL+"/v1/messages", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d %s", resp.StatusCode, raw)
	}
	for _, event := range []string{"message_start", "content_block_start", "text_delta", "message_delta", "message_stop"} {
		if !strings.Contains(string(raw), event) {
			t.Errorf("missing event %s in %s", event, raw)
		}
	}
	if strings.Contains(string(raw), "data: [DONE]") {
		t.Error("Anthropic stream must not use OpenAI [DONE] sentinel")
	}
}

func TestAnthropicMessagesRejectUnsupportedBlocks(t *testing.T) {
	up := fakeUpstream(t)
	defer up.Close()
	ts := newTestServer(t, up.URL, policy.New(nil, nil), nil)
	defer ts.Close()
	body := anthTestBody(t, map[string]any{
		"model": "nabu-fast", "max_tokens": 128,
		"messages": []map[string]any{{
			"role": "user", "content": []map[string]any{{"type": "document"}},
		}},
	})
	resp, err := http.Post(ts.URL+"/v1/messages", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d, body=%s", resp.StatusCode, raw)
	}
}
