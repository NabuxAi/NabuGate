package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"nabugate/internal/agent"
	"nabugate/internal/config"
	"nabugate/internal/policy"
	"nabugate/internal/provider"
	"nabugate/internal/router"
	"nabugate/internal/usage"
)

// ─────────────────────────────── fakes ─────────────────────────────────────

// toolCallReply makes the provider answer with one function call.
func toolCallReply(id, name, args string) string {
	return fmt.Sprintf(`{"choices":[{"message":{"content":"","tool_calls":[{"id":%q,"type":"function","function":{"name":%q,"arguments":%q}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`, id, name, args)
}

const proseReply = `{"choices":[{"message":{"content":"سفارش شما ارسال شد."},"finish_reason":"stop"}],"usage":{"prompt_tokens":20,"completion_tokens":8,"total_tokens":28}}`

// scriptedUpstream answers /chat/completions by inspecting the conversation:
// while no role:tool message is present it calls track_order; once the tool
// result arrives it answers in prose. It records every body it receives.
type scriptedUpstream struct {
	bodies []map[string]json.RawMessage
	calls  int
}

func (u *scriptedUpstream) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var b map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&b)
		u.bodies = append(u.bodies, b)
		u.calls++

		var msgs []struct {
			Role string `json:"role"`
		}
		_ = json.Unmarshal(b["messages"], &msgs)
		sawToolResult := false
		for _, m := range msgs {
			if m.Role == "tool" {
				sawToolResult = true
			}
		}
		if sawToolResult {
			fmt.Fprint(w, proseReply)
			return
		}
		fmt.Fprint(w, toolCallReply("call_1", "track_order", `{"order_id":"42"}`))
	}
}

// newToolTestServer builds a gateway whose nabu-fast alias resolves to the
// given OpenAI-wire upstream, with the tool executor permitted to dial
// loopback (the mock tool endpoint is an httptest server).
func newToolTestServer(t *testing.T, upstreamURL string, agents *agent.Registry) *httptest.Server {
	t.Helper()
	adapters := map[string]provider.Adapter{
		"parspack": provider.NewOpenAIAdapter("parspack", upstreamURL, "k", nil),
	}
	models := map[string]config.ModelRoute{
		"nabu-fast": {Primary: config.Target{Provider: "parspack", Model: "openai/gpt-5.5"}},
	}
	r := router.New(adapters, models, nil, nil, nil, nil, map[string][]string{"parspack": nil}, discardLogger())
	srv := New(r, policy.New(nil, nil), usage.New(nil), agents, discardLogger())
	srv.WithToolExecutor(agent.NewToolExecutorForTest(true))
	return httptest.NewServer(srv.Handler())
}

// shopAgent registers one agent whose track_order tool points at toolURL.
func shopAgent(t *testing.T, toolURL string, maxSteps int) *agent.Registry {
	t.Helper()
	reg := agent.NewRegistry()
	err := reg.Add(agent.Agent{
		Name:         "shop-agent",
		Model:        "nabu-fast",
		System:       "You are a shop assistant.",
		MaxToolSteps: maxSteps,
		Tools: []agent.Tool{{
			Name:        "track_order",
			Type:        agent.ToolTypeHTTP,
			Description: "fetch an order",
			Method:      "GET",
			URL:         toolURL + "/orders/{order_id}",
			PathParams:  []string{"order_id"},
			Parameters:  json.RawMessage(`{"type":"object","properties":{"order_id":{"type":"string"}},"required":["order_id"]}`),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

// messagesOf decodes the messages array of a captured upstream body.
func messagesOf(t *testing.T, body map[string]json.RawMessage) []map[string]any {
	t.Helper()
	var msgs []map[string]any
	if err := json.Unmarshal(body["messages"], &msgs); err != nil {
		t.Fatalf("messages unmarshal: %v", err)
	}
	return msgs
}

// ─────────────────────────────── tests ─────────────────────────────────────

// TestAgentToolLoopEndToEnd is the whole story in one request: the model asks
// for track_order, the gateway calls the shop, hands the result back, and the
// caller receives only the final prose answer.
func TestAgentToolLoopEndToEnd(t *testing.T) {
	var toolSawPath string
	var toolCalls int32
	toolSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		toolSawPath = r.URL.Path
		atomic.AddInt32(&toolCalls, 1)
		fmt.Fprint(w, `{"id":42,"status":"shipped"}`)
	}))
	defer toolSrv.Close()

	up := &scriptedUpstream{}
	upSrv := newHTTPServer(t, up.handler(t))
	ts := newToolTestServer(t, upSrv.URL, shopAgent(t, toolSrv.URL, 0))
	defer ts.Close()

	body := `{"model":"shop-agent","messages":[{"role":"user","content":"سفارش ۴۲ کجاست؟"}]}`
	resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d: %s", resp.StatusCode, b)
	}

	var out struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)

	if out.Choices[0].Message.Content != "سفارش شما ارسال شد." {
		t.Errorf("content = %q", out.Choices[0].Message.Content)
	}
	if out.Choices[0].FinishReason != "stop" {
		t.Errorf("finish = %q", out.Choices[0].FinishReason)
	}
	if out.Model != "shop-agent" {
		t.Errorf("model = %q, want the agent name echoed", out.Model)
	}
	// Usage is summed over both provider round-trips (15 + 28).
	if out.Usage.TotalTokens != 43 {
		t.Errorf("total_tokens = %d, want 43", out.Usage.TotalTokens)
	}
	if got := resp.Header.Get("X-Nabu-Tool-Calls"); got != "1" {
		t.Errorf("X-Nabu-Tool-Calls = %q, want 1", got)
	}
	if atomic.LoadInt32(&toolCalls) != 1 || toolSawPath != "/orders/42" {
		t.Errorf("tool endpoint hit %d times at %q", toolCalls, toolSawPath)
	}

	// The provider saw two requests: the first offered the agent's tools, the
	// second carried the assistant tool_calls and the role:tool result.
	if up.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", up.calls)
	}
	var toolsOffered []map[string]any
	if err := json.Unmarshal(up.bodies[0]["tools"], &toolsOffered); err != nil || len(toolsOffered) != 1 {
		t.Fatalf("tools offered upstream: %v (%s)", err, up.bodies[0]["tools"])
	}
	fn, _ := toolsOffered[0]["function"].(map[string]any)
	if fn["name"] != "track_order" {
		t.Errorf("offered tool = %v", fn["name"])
	}
	second := messagesOf(t, up.bodies[1])
	var assistantCalls, toolResult map[string]any
	for _, m := range second {
		if m["role"] == "assistant" && m["tool_calls"] != nil {
			assistantCalls = m
		}
		if m["role"] == "tool" {
			toolResult = m
		}
	}
	if assistantCalls == nil {
		t.Error("second provider call missing the assistant tool_calls message")
	}
	if toolResult == nil || toolResult["tool_call_id"] != "call_1" ||
		!strings.Contains(toolResult["content"].(string), `"status":"shipped"`) {
		t.Errorf("tool result message = %v", toolResult)
	}
}

// TestAgentToolLoopStepCap forces the model to always want another tool: after
// the step budget the gateway strips the tools and makes it answer.
func TestAgentToolLoopStepCap(t *testing.T) {
	var toolCalls int32
	toolSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&toolCalls, 1)
		fmt.Fprint(w, `{}`)
	}))
	defer toolSrv.Close()

	var bodies []map[string]json.RawMessage
	upSrv := newHTTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		var b map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&b)
		bodies = append(bodies, b)
		if len(b["tools"]) == 0 {
			// Tools were taken away: the model must answer with what it has.
			fmt.Fprint(w, proseReply)
			return
		}
		fmt.Fprint(w, toolCallReply(fmt.Sprint("call_", len(bodies)), "track_order", `{"order_id":"42"}`))
	})

	ts := newToolTestServer(t, upSrv.URL, shopAgent(t, toolSrv.URL, 2))
	defer ts.Close()

	body := `{"model":"shop-agent","messages":[{"role":"user","content":"پیگیری کن"}]}`
	resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d: %s", resp.StatusCode, b)
	}

	if atomic.LoadInt32(&toolCalls) != 2 {
		t.Errorf("tool executions = %d, want exactly max_tool_steps (2)", toolCalls)
	}
	if len(bodies) != 3 {
		t.Fatalf("provider calls = %d, want 2 tool steps + 1 forced answer", len(bodies))
	}
	if len(bodies[2]["tools"]) != 0 {
		t.Error("final forced call still carried tools")
	}
	if got := resp.Header.Get("X-Nabu-Tool-Calls"); got != "2" {
		t.Errorf("X-Nabu-Tool-Calls = %q, want 2", got)
	}
}

// TestAgentToolLoopClientToolsWin: a caller that sends its own tools gets the
// old pass-through behaviour — the agent's tools are not injected, no loop
// runs, and the tool endpoint is never touched.
func TestAgentToolLoopClientToolsWin(t *testing.T) {
	var toolCalls int32
	toolSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&toolCalls, 1)
		fmt.Fprint(w, `{}`)
	}))
	defer toolSrv.Close()

	var gotTools string
	upSrv := newHTTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		var b map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&b)
		gotTools = string(b["tools"])
		// The upstream answers with a call to the CLIENT's tool; the gateway
		// must hand it straight back instead of executing anything.
		fmt.Fprint(w, toolCallReply("call_9", "client_tool", `{}`))
	})

	ts := newToolTestServer(t, upSrv.URL, shopAgent(t, toolSrv.URL, 0))
	defer ts.Close()

	body := `{"model":"shop-agent","tools":[{"type":"function","function":{"name":"client_tool","parameters":{"type":"object"}}}],"messages":[{"role":"user","content":"hi"}]}`
	resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d: %s", resp.StatusCode, b)
	}

	if !strings.Contains(gotTools, "client_tool") || strings.Contains(gotTools, "track_order") {
		t.Errorf("upstream tools = %s, want the client's tools only", gotTools)
	}
	if atomic.LoadInt32(&toolCalls) != 0 {
		t.Error("agent tool endpoint was called despite client tools winning")
	}
	// The tool_calls response passes through to the caller unexecuted.
	var out struct {
		Choices []struct {
			Message map[string]any `json:"message"`
		} `json:"choices"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Choices[0].Message["tool_calls"] == nil {
		t.Error("client tool_calls were not passed back to the caller")
	}
	if got := resp.Header.Get("X-Nabu-Tool-Calls"); got != "" {
		t.Errorf("X-Nabu-Tool-Calls = %q, want unset on pass-through", got)
	}
}

// TestAgentToolLoopUnknownToolFedBack: a call to an undeclared function is an
// answerable error, fed back as the tool result — not a failed request.
func TestAgentToolLoopUnknownToolFedBack(t *testing.T) {
	up := &scriptedUpstream{}
	// Rewrite the script: call a tool that does not exist first.
	var once int32
	upSrv := newHTTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		var b map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&b)
		up.bodies = append(up.bodies, b)
		if atomic.AddInt32(&once, 1) == 1 {
			fmt.Fprint(w, toolCallReply("call_x", "delete_everything", `{}`))
			return
		}
		fmt.Fprint(w, proseReply)
	})

	ts := newToolTestServer(t, upSrv.URL, shopAgent(t, "http://127.0.0.1:1", 0))
	defer ts.Close()

	body := `{"model":"shop-agent","messages":[{"role":"user","content":"hi"}]}`
	resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d: %s", resp.StatusCode, b)
	}
	second := messagesOf(t, up.bodies[1])
	found := false
	for _, m := range second {
		if m["role"] == "tool" && strings.Contains(m["content"].(string), "unknown tool") {
			found = true
		}
	}
	if !found {
		t.Error("unknown-tool error was not fed back as a tool message")
	}
}

// TestAgentToolLoopStreamShaped: stream:true against a tool agent still gets
// an SSE response — the finished answer re-shaped as one delta.
func TestAgentToolLoopStreamShaped(t *testing.T) {
	toolSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":"shipped"}`)
	}))
	defer toolSrv.Close()

	up := &scriptedUpstream{}
	upSrv := newHTTPServer(t, up.handler(t))
	ts := newToolTestServer(t, upSrv.URL, shopAgent(t, toolSrv.URL, 0))
	defer ts.Close()

	body := `{"model":"shop-agent","stream":true,"messages":[{"role":"user","content":"سفارشم کجاست؟"}]}`
	resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q, want SSE", ct)
	}
	raw, _ := io.ReadAll(resp.Body)
	s := string(raw)
	if !strings.Contains(s, "سفارش شما ارسال شد.") {
		t.Errorf("stream missing the final answer: %s", s)
	}
	if !strings.Contains(s, "data: [DONE]") {
		t.Errorf("stream missing terminator: %s", s)
	}
}

// TestAgentToolLoopRefusesNonOpenAIRoute: an agent with tools whose model
// routes only to a non-OpenAI-wire provider fails loudly at request time.
func TestAgentToolLoopRefusesNonOpenAIRoute(t *testing.T) {
	adapters := map[string]provider.Adapter{
		"anth": provider.NewAnthropicAdapter("anth", "http://127.0.0.1:1", "k"),
	}
	models := map[string]config.ModelRoute{
		"nabu-claude": {Primary: config.Target{Provider: "anth", Model: "claude-sonnet"}},
	}
	r := router.New(adapters, models, nil, nil, nil, nil, nil, discardLogger())

	reg := agent.NewRegistry()
	if err := reg.Add(agent.Agent{
		Name: "claude-tools-agent", Model: "nabu-claude",
		Tools: []agent.Tool{{
			Name: "track_order", Type: agent.ToolTypeHTTP, Method: "GET",
			URL:        "https://shop.example.com/orders/{order_id}",
			PathParams: []string{"order_id"},
			Parameters: json.RawMessage(`{"type":"object"}`),
		}},
	}); err != nil {
		t.Fatal(err)
	}

	srv := New(r, policy.New(nil, nil), usage.New(nil), reg, discardLogger())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := `{"model":"claude-tools-agent","messages":[{"role":"user","content":"hi"}]}`
	resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 400: %s", resp.StatusCode, b)
	}
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), "OpenAI tool wire format") {
		t.Errorf("error should explain the wire-format limitation: %s", b)
	}
}

// TestAgentsEndpointListsTools verifies /v1/agents shows each agent's tools
// and honours the calling key's allow-list.
func TestAgentsEndpointListsTools(t *testing.T) {
	upSrv := newHTTPServer(t, func(w http.ResponseWriter, r *http.Request) {})
	ts := newToolTestServer(t, upSrv.URL, shopAgent(t, "https://shop.example.com", 3))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/agents")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Data []struct {
			Name         string   `json:"name"`
			Model        string   `json:"model"`
			Tools        []string `json:"tools"`
			MaxToolSteps int      `json:"max_tool_steps"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if len(out.Data) != 1 {
		t.Fatalf("data = %+v", out.Data)
	}
	got := out.Data[0]
	if got.Name != "shop-agent" || got.Model != "nabu-fast" {
		t.Errorf("entry = %+v", got)
	}
	if len(got.Tools) != 1 || got.Tools[0] != "track_order" {
		t.Errorf("tools = %v", got.Tools)
	}
	if got.MaxToolSteps != 3 {
		t.Errorf("max_tool_steps = %d", got.MaxToolSteps)
	}
}

// TestAgentsEndpointScopedByAllowList: a project key sees only its own agents.
func TestAgentsEndpointScopedByAllowList(t *testing.T) {
	upSrv := newHTTPServer(t, func(w http.ResponseWriter, r *http.Request) {})

	adapters := map[string]provider.Adapter{
		"parspack": provider.NewOpenAIAdapter("parspack", upSrv.URL, "k", nil),
	}
	models := map[string]config.ModelRoute{
		"nabu-fast": {Primary: config.Target{Provider: "parspack", Model: "openai/gpt-5.5"}},
	}
	r := router.New(adapters, models, nil, nil, nil, nil, nil, discardLogger())

	reg := agent.NewRegistry()
	_ = reg.Add(agent.Agent{Name: "allowed-agent", Model: "nabu-fast"})
	_ = reg.Add(agent.Agent{Name: "other-agent", Model: "nabu-fast"})

	enforcer := policy.New(nil, []policy.KeyConfig{{
		Key: "k1", Project: "p1", Allow: []string{"allowed-agent"},
	}})
	srv := New(r, enforcer, usage.New(nil), reg, discardLogger())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer k1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Data []struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if len(out.Data) != 1 || out.Data[0].Name != "allowed-agent" {
		t.Errorf("scoped list = %+v", out.Data)
	}
}
