package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"nabugate/internal/agent"
	"nabugate/internal/policy"
)

// newHTTPServer starts an httptest server for h and closes it at test end.
func newHTTPServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	return s
}

// capture records the last chat body an upstream received, so agent tests can
// assert what NabuGate actually forwarded (injected system prompt, params,
// rewritten model).
type capture struct {
	mu   sync.Mutex
	body map[string]json.RawMessage
}

func (c *capture) set(b map[string]json.RawMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.body = b
}

func (c *capture) get() map[string]json.RawMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.body
}

// capturingUpstream is an OpenAI-wire fake that records each /chat/completions
// body and answers with either JSON or SSE, depending on the "stream" flag.
func capturingUpstream(t *testing.T) (*capture, http.HandlerFunc) {
	t.Helper()
	cap := &capture{}
	return cap, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var b map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&b)
		cap.set(b)
		if len(b["stream"]) > 0 && string(b["stream"]) == "true" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"total_tokens\":2}}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"total_tokens":2}}`))
	}
}

// writerAgent is a small registry with one agent used across the tests below.
func writerAgent(t *testing.T) *agent.Registry {
	t.Helper()
	reg := agent.NewRegistry()
	temp := 0.3
	if err := reg.Add(agent.Agent{
		Name:        "cine-writer",
		Description: "UX writer",
		Model:       "nabu-fast",
		System:      "You are a UX writer.",
		Temperature: &temp,
	}); err != nil {
		t.Fatal(err)
	}
	return reg
}

// firstMessage returns the role+content of the message at index i in a captured
// chat body.
func nthMessage(t *testing.T, body map[string]json.RawMessage, i int) (string, string) {
	t.Helper()
	var msgs []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(body["messages"], &msgs); err != nil {
		t.Fatalf("messages unmarshal: %v", err)
	}
	if i >= len(msgs) {
		t.Fatalf("message index %d out of range (have %d)", i, len(msgs))
	}
	return msgs[i].Role, msgs[i].Content
}

// TestAgentChatInjectsSystemAndRoutes verifies an agent request prepends the
// agent's system prompt, applies its default temperature, routes to the agent's
// underlying model, and echoes the agent name back to the caller.
func TestAgentChatInjectsSystemAndRoutes(t *testing.T) {
	cap, h := capturingUpstream(t)
	up := newHTTPServer(t, h)
	ts := newTestServer(t, up.URL, policy.New(nil, nil), writerAgent(t))
	defer ts.Close()

	body := `{"model":"cine-writer","messages":[{"role":"user","content":"hi"}]}`
	resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Nabu-Agent"); got != "cine-writer" {
		t.Errorf("X-Nabu-Agent = %q, want cine-writer", got)
	}

	got := cap.get()
	// System prompt prepended ahead of the caller's user message.
	if role, content := nthMessage(t, got, 0); role != "system" || content != "You are a UX writer." {
		t.Errorf("message[0] = %q/%q, want system/'You are a UX writer.'", role, content)
	}
	if role, content := nthMessage(t, got, 1); role != "user" || content != "hi" {
		t.Errorf("message[1] = %q/%q, want user/hi", role, content)
	}
	// Underlying model resolved from the agent's "nabu-fast" alias.
	var fwdModel string
	_ = json.Unmarshal(got["model"], &fwdModel)
	if fwdModel != "openai/gpt-5.5" {
		t.Errorf("forwarded model = %q, want openai/gpt-5.5", fwdModel)
	}
	// Default temperature applied.
	if string(got["temperature"]) != "0.3" {
		t.Errorf("temperature = %s, want 0.3", got["temperature"])
	}

	// Response echoes the agent name as the model, not the underlying alias.
	var out struct {
		Model    string `json:"model"`
		Provider string `json:"provider"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Model != "cine-writer" {
		t.Errorf("response model = %q, want cine-writer", out.Model)
	}
}

// TestAgentDefaultsDoNotOverrideCaller verifies a caller-supplied parameter wins
// over the agent's default.
func TestAgentDefaultsDoNotOverrideCaller(t *testing.T) {
	cap, h := capturingUpstream(t)
	up := newHTTPServer(t, h)
	ts := newTestServer(t, up.URL, policy.New(nil, nil), writerAgent(t))
	defer ts.Close()

	body := `{"model":"cine-writer","temperature":0.9,"messages":[{"role":"user","content":"hi"}]}`
	resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if string(cap.get()["temperature"]) != "0.9" {
		t.Errorf("temperature = %s, want caller's 0.9", cap.get()["temperature"])
	}
}

// TestAgentListedInModels verifies configured agents appear on /v1/models.
func TestAgentListedInModels(t *testing.T) {
	cap, h := capturingUpstream(t)
	_ = cap
	up := newHTTPServer(t, h)
	ts := newTestServer(t, up.URL, policy.New(nil, nil), writerAgent(t))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	found := false
	for _, m := range out.Data {
		if m.ID == "cine-writer" {
			found = true
			if m.OwnedBy != "agent" {
				t.Errorf("owned_by = %q, want agent", m.OwnedBy)
			}
		}
	}
	if !found {
		t.Errorf("cine-writer missing from /v1/models: %+v", out.Data)
	}
}

// TestAgentStreamInjectsSystem verifies the streaming path also injects the
// agent's system prompt and labels chunks with the agent name.
func TestAgentStreamInjectsSystem(t *testing.T) {
	cap, h := capturingUpstream(t)
	up := newHTTPServer(t, h)
	ts := newTestServer(t, up.URL, policy.New(nil, nil), writerAgent(t))
	defer ts.Close()

	body := `{"model":"cine-writer","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q, want SSE", ct)
	}
	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	if !strings.Contains(string(buf[:n]), `"model":"cine-writer"`) {
		t.Errorf("stream chunk should carry agent name, got %s", buf[:n])
	}
	if role, content := nthMessage(t, cap.get(), 0); role != "system" || content != "You are a UX writer." {
		t.Errorf("streamed system message = %q/%q", role, content)
	}
}
