package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"nabugate/internal/agent"
	"nabugate/internal/config"
	"nabugate/internal/flow"
	"nabugate/internal/policy"
	"nabugate/internal/provider"
	"nabugate/internal/router"
	"nabugate/internal/usage"
)

// echoUpstream answers each chat with the last user message it was sent,
// prefixed by a marker, and records every body it saw in order.
//
// Echoing rather than returning a constant is the whole point: a chain is only
// a chain if step two actually receives what step one produced, and a fake that
// always says "ok" cannot tell the difference.
type recorder struct {
	mu     sync.Mutex
	bodies []map[string]json.RawMessage
}

func (r *recorder) add(b map[string]json.RawMessage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bodies = append(r.bodies, b)
}

func (r *recorder) all() []map[string]json.RawMessage {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]map[string]json.RawMessage(nil), r.bodies...)
}

// failAgent names the step that should fail, matched against the system prompt
// the gateway injected. Matching on the agent rather than on the text it was
// sent is what keeps the assertion precise: after an optional step is skipped
// the next one receives the same text, so a content match would fail both.
func echoUpstream(t *testing.T, failAgent string) (*recorder, http.HandlerFunc) {
	t.Helper()
	rec := &recorder{}

	return rec, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var b map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&b)
		rec.add(b)

		var msgs []provider.Message
		_ = json.Unmarshal(b["messages"], &msgs)
		last, system := "", ""
		for i := len(msgs) - 1; i >= 0; i-- {
			if msgs[i].Role == "user" && last == "" {
				last = msgs[i].Content
			}
			if msgs[i].Role == "system" {
				system = msgs[i].Content
			}
		}

		if failAgent != "" && strings.Contains(system, failAgent) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"upstream is having a bad minute"}`))
			return
		}

		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%q},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`, "<"+last+">")
	}
}

func flowServer(t *testing.T, upstreamURL string, agents *agent.Registry, flows *flow.Registry) *httptest.Server {
	t.Helper()
	adapters := map[string]provider.Adapter{
		"parspack": provider.NewOpenAIAdapter("parspack", upstreamURL, "k", nil),
	}
	models := map[string]config.ModelRoute{
		"nabu-fast": {Primary: config.Target{Provider: "parspack", Model: "openai/gpt-5.5"}},
	}
	r := router.New(adapters, models, nil, nil, nil, nil, map[string][]string{"parspack": nil}, discardLogger())
	srv := New(r, policy.New(nil, nil), usage.New(nil), agents, discardLogger()).WithFlows(flows)

	return httptest.NewServer(srv.Handler())
}

func threeStepFlow(t *testing.T) (*agent.Registry, *flow.Registry) {
	t.Helper()

	agents := agent.NewRegistry()
	for _, name := range []string{"writer", "reviewer", "summariser"} {
		if err := agents.Add(agent.Agent{Name: name, Model: "nabu-fast", System: "you are the " + name}); err != nil {
			t.Fatal(err)
		}
	}

	flows := flow.NewRegistry()
	if err := flows.Add(flow.Flow{
		Name: "sales-team",
		Steps: []flow.Step{
			{Agent: "writer"},
			{Agent: "reviewer"},
			{Agent: "summariser", Label: "wrap up"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	return agents, flows
}

func postFlow(t *testing.T, ts *httptest.Server, body string) (*http.Response, map[string]any) {
	t.Helper()
	resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)

	return resp, out
}

func content(t *testing.T, out map[string]any) string {
	t.Helper()
	choices, ok := out["choices"].([]any)
	if !ok || len(choices) == 0 {
		t.Fatalf("no choices in %v", out)
	}
	msg := choices[0].(map[string]any)["message"].(map[string]any)

	return msg["content"].(string)
}

// TestFlowChainsOutputForward is the whole promise: each step is handed what the
// one before it produced, and the caller gets the last one.
func TestFlowChainsOutputForward(t *testing.T) {
	rec, h := echoUpstream(t, "")
	up := newHTTPServer(t, h)
	agents, flows := threeStepFlow(t)
	ts := flowServer(t, up.URL, agents, flows)
	defer ts.Close()

	resp, out := postFlow(t, ts, `{"model":"sales-team","messages":[{"role":"user","content":"brief"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %v", resp.StatusCode, out)
	}

	// Three nested markers: writer saw "brief", reviewer saw the writer's
	// answer, the summariser saw the reviewer's.
	if got := content(t, out); got != "<<<brief>>>" {
		t.Errorf("content = %q, want <<<brief>>>", got)
	}
	if got := len(rec.all()); got != 3 {
		t.Fatalf("upstream calls = %d, want 3", got)
	}

	// Each step still gets its own agent's system prompt: a chain of
	// specialists that all shared one prompt would be one model called thrice.
	for i, want := range []string{"you are the writer", "you are the reviewer", "you are the summariser"} {
		if role, got := nthMessage(t, rec.all()[i], 0); role != "system" || got != want {
			t.Errorf("call %d message[0] = %q/%q, want system/%q", i, role, got, want)
		}
	}
}

// TestFlowTraceIsReturned — a flow that gives a good answer for the wrong
// reason looks exactly like one that does not, unless the middle is visible.
func TestFlowTraceIsReturned(t *testing.T) {
	_, h := echoUpstream(t, "")
	up := newHTTPServer(t, h)
	agents, flows := threeStepFlow(t)
	ts := flowServer(t, up.URL, agents, flows)
	defer ts.Close()

	resp, out := postFlow(t, ts, `{"model":"sales-team","messages":[{"role":"user","content":"brief"}]}`)

	if got := resp.Header.Get("X-Nabu-Flow"); got != "sales-team" {
		t.Errorf("X-Nabu-Flow = %q", got)
	}

	fl, ok := out["flow"].(map[string]any)
	if !ok {
		t.Fatalf("no flow trace in %v", out)
	}
	steps, _ := fl["steps"].([]any)
	if len(steps) != 3 {
		t.Fatalf("trace steps = %d, want 3", len(steps))
	}

	// The label names the step, not the agent, when one was given.
	if name := steps[2].(map[string]any)["name"]; name != "wrap up" {
		t.Errorf("step 3 name = %v, want 'wrap up'", name)
	}
	if agentName := steps[2].(map[string]any)["agent"]; agentName != "summariser" {
		t.Errorf("step 3 agent = %v", agentName)
	}
}

// TestFlowUsageIsTheSumOfItsSteps — four models cost four calls, and a bill
// that reported one would make the choice to run a chain unreviewable.
func TestFlowUsageIsTheSumOfItsSteps(t *testing.T) {
	_, h := echoUpstream(t, "")
	up := newHTTPServer(t, h)
	agents, flows := threeStepFlow(t)
	ts := flowServer(t, up.URL, agents, flows)
	defer ts.Close()

	_, out := postFlow(t, ts, `{"model":"sales-team","messages":[{"role":"user","content":"brief"}]}`)

	u := out["usage"].(map[string]any)
	if got := u["total_tokens"].(float64); got != 6 {
		t.Errorf("total_tokens = %v, want 6 (3 steps × 2)", got)
	}
}

// TestFlowStepFailureEndsTheChain — a reviewer handed nothing writes a review
// of nothing, and it reads exactly as confidently as a real one.
func TestFlowStepFailureEndsTheChain(t *testing.T) {
	rec, h := echoUpstream(t, "reviewer") // step two dies
	up := newHTTPServer(t, h)
	agents, flows := threeStepFlow(t)
	ts := flowServer(t, up.URL, agents, flows)
	defer ts.Close()

	resp, out := postFlow(t, ts, `{"model":"sales-team","messages":[{"role":"user","content":"brief"}]}`)

	if resp.StatusCode == http.StatusOK {
		t.Fatalf("a dead step must not answer 200: %v", out)
	}
	// The router retries a 500, so the call count is not the assertion — the
	// assertion is that the chain stopped: nothing downstream of the dead step
	// was ever asked anything.
	for _, call := range rec.all() {
		if role, content := nthMessage(t, call, 0); role == "system" && strings.Contains(content, "summariser") {
			t.Fatal("the summariser ran after the step before it died")
		}
	}
}

// TestOptionalStepIsSkipped — the opt-in the default exists to make deliberate.
func TestOptionalStepIsSkipped(t *testing.T) {
	_, h := echoUpstream(t, "reviewer")
	up := newHTTPServer(t, h)

	agents, _ := threeStepFlow(t)
	flows := flow.NewRegistry()
	if err := flows.Add(flow.Flow{
		Name: "tolerant",
		Steps: []flow.Step{
			{Agent: "writer"},
			{Agent: "reviewer", Optional: true},
			{Agent: "summariser"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	ts := flowServer(t, up.URL, agents, flows)
	defer ts.Close()

	resp, out := postFlow(t, ts, `{"model":"tolerant","messages":[{"role":"user","content":"brief"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %v", resp.StatusCode, out)
	}

	// The summariser received the writer's output, the failed step having been
	// stepped over rather than passing an empty string down the chain.
	if got := content(t, out); got != "<<brief>>" {
		t.Errorf("content = %q, want <<brief>>", got)
	}

	steps := out["flow"].(map[string]any)["steps"].([]any)
	if skipped := steps[1].(map[string]any)["skipped"]; skipped != true {
		t.Errorf("step 2 skipped = %v, want true", skipped)
	}
	if steps[1].(map[string]any)["error"] == nil {
		t.Error("a skipped step should say why")
	}
}

// TestFlowTemplateReachesBothTheDraftAndTheBrief — a late reviewer needs the
// original ask, not only what the step before it made of it.
func TestFlowTemplateReachesBothTheDraftAndTheBrief(t *testing.T) {
	rec, h := echoUpstream(t, "")
	up := newHTTPServer(t, h)

	agents, _ := threeStepFlow(t)
	flows := flow.NewRegistry()
	if err := flows.Add(flow.Flow{
		Name: "checked",
		Steps: []flow.Step{
			{Agent: "writer"},
			{Agent: "reviewer", Input: "BRIEF: {{input}}\nDRAFT: {{previous}}"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	ts := flowServer(t, up.URL, agents, flows)
	defer ts.Close()

	postFlow(t, ts, `{"model":"checked","messages":[{"role":"user","content":"sell it"}]}`)

	calls := rec.all()
	if len(calls) != 2 {
		t.Fatalf("upstream calls = %d, want 2", len(calls))
	}
	_, sent := nthMessage(t, calls[1], 1)
	if sent != "BRIEF: sell it\nDRAFT: <sell it>" {
		t.Errorf("step 2 input = %q", sent)
	}
}

// TestFlowCannotStream — the first token of a chain's answer does not exist
// until its last step starts, and streaming a middle step's draft as if it were
// the answer is worse than saying so.
func TestFlowCannotStream(t *testing.T) {
	_, h := echoUpstream(t, "")
	up := newHTTPServer(t, h)
	agents, flows := threeStepFlow(t)
	ts := flowServer(t, up.URL, agents, flows)
	defer ts.Close()

	resp, _ := postFlow(t, ts, `{"model":"sales-team","messages":[{"role":"user","content":"x"}],"stream":true}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// TestFlowCycleIsRefusedByName — "flow x is part of a cycle" is a definition
// the operator can go and fix; "too deep" is a symptom they would have to
// diagnose.
func TestFlowCycleIsRefusedByName(t *testing.T) {
	_, h := echoUpstream(t, "")
	up := newHTTPServer(t, h)

	agents, _ := threeStepFlow(t)
	flows := flow.NewRegistry()
	_ = flows.Add(flow.Flow{Name: "a", Steps: []flow.Step{{Agent: "b"}}})
	_ = flows.Add(flow.Flow{Name: "b", Steps: []flow.Step{{Agent: "a"}}})

	ts := flowServer(t, up.URL, agents, flows)
	defer ts.Close()

	resp, out := postFlow(t, ts, `{"model":"a","messages":[{"role":"user","content":"x"}]}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if msg := fmt.Sprint(out); !strings.Contains(msg, "cycle") {
		t.Errorf("error should name the cycle, got %v", out)
	}
}

// TestFlowOfFlowsRuns — "a team built from teams" is the case nesting exists
// for, and it must work up to the cap rather than only be guarded against.
func TestFlowOfFlowsRuns(t *testing.T) {
	_, h := echoUpstream(t, "")
	up := newHTTPServer(t, h)

	agents, _ := threeStepFlow(t)
	flows := flow.NewRegistry()
	_ = flows.Add(flow.Flow{Name: "inner", Steps: []flow.Step{{Agent: "writer"}, {Agent: "reviewer"}}})
	_ = flows.Add(flow.Flow{Name: "outer", Steps: []flow.Step{{Agent: "inner"}, {Agent: "summariser"}}})

	ts := flowServer(t, up.URL, agents, flows)
	defer ts.Close()

	resp, out := postFlow(t, ts, `{"model":"outer","messages":[{"role":"user","content":"brief"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %v", resp.StatusCode, out)
	}
	if got := content(t, out); got != "<<<brief>>>" {
		t.Errorf("content = %q, want <<<brief>>>", got)
	}
	// The nested flow's tokens are counted once, not dropped and not doubled.
	if got := out["usage"].(map[string]any)["total_tokens"].(float64); got != 6 {
		t.Errorf("total_tokens = %v, want 6", got)
	}
}

// TestFlowsAppearOnModels — a chain nobody can discover is one nobody will use.
func TestFlowsAppearOnModels(t *testing.T) {
	_, h := echoUpstream(t, "")
	up := newHTTPServer(t, h)
	agents, flows := threeStepFlow(t)
	ts := flowServer(t, up.URL, agents, flows)
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

	for _, m := range out.Data {
		if m.ID == "sales-team" {
			if m.OwnedBy != "flow" {
				t.Errorf("sales-team owned_by = %q, want flow", m.OwnedBy)
			}
			return
		}
	}
	t.Error("sales-team missing from /v1/models")
}

// TestFlowIsRefusedForAKeyThatMayNotUseIt — a flow is reached through the same
// door as any other model, so the key allow-list has to cover it.
func TestFlowIsRefusedForAKeyThatMayNotUseIt(t *testing.T) {
	_, h := echoUpstream(t, "")
	up := newHTTPServer(t, h)

	agents, flows := threeStepFlow(t)
	adapters := map[string]provider.Adapter{
		"parspack": provider.NewOpenAIAdapter("parspack", up.URL, "k", nil),
	}
	models := map[string]config.ModelRoute{
		"nabu-fast": {Primary: config.Target{Provider: "parspack", Model: "openai/gpt-5.5"}},
	}
	r := router.New(adapters, models, nil, nil, nil, nil, map[string][]string{"parspack": nil}, discardLogger())
	enforcer := policy.New(nil, []policy.KeyConfig{
		{Key: "secret", Project: "walled-in", Allow: []string{"nabu-fast"}},
	})
	srv := New(r, enforcer, usage.New(nil), agents, discardLogger()).WithFlows(flows)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"sales-team","messages":[{"role":"user","content":"x"}]}`))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", resp.StatusCode)
	}
}
