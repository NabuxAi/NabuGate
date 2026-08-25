package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nabugate/internal/adminstore"
	"nabugate/internal/agent"
	"nabugate/internal/config"
	"nabugate/internal/flow"
	"nabugate/internal/policy"
	"nabugate/internal/provider"
	"nabugate/internal/router"
	"nabugate/internal/usage"
)

// consoleFlowServer builds a gateway with a real admin store on a temp dir and
// a signed-in console session, so the flow editor can be exercised end to end.
func consoleFlowServer(t *testing.T, upstreamURL string) (*httptest.Server, *Server, string) {
	t.Helper()

	store, err := adminstore.Open(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateAdmin("root", "correct-horse-battery"); err != nil {
		t.Fatal(err)
	}
	session, _, err := store.Authenticate("root", "correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}

	agents := agent.NewRegistry()
	for _, name := range []string{"writer", "reviewer"} {
		_ = agents.Add(agent.Agent{Name: name, Model: "nabu-fast", System: "you are the " + name})
	}

	adapters := map[string]provider.Adapter{
		"parspack": provider.NewOpenAIAdapter("parspack", upstreamURL, "k", nil),
	}
	models := map[string]config.ModelRoute{
		"nabu-fast": {Primary: config.Target{Provider: "parspack", Model: "openai/gpt-5.5"}},
	}
	r := router.New(adapters, models, nil, nil, nil, nil, map[string][]string{"parspack": nil}, discardLogger())

	srv := New(r, policy.New(nil, nil), usage.New(nil), agents, discardLogger()).WithFlows(flow.NewRegistry())
	srv.SetAdminStore(store)

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	return ts, srv, session
}

func consoleDo(t *testing.T, ts *httptest.Server, session, method, path, body string) (*http.Response, map[string]any) {
	t.Helper()

	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, ts.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: consoleCookie, Value: session})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)

	return resp, out
}

// TestConsoleCreatesAFlowThatIsImmediatelyCallable is the whole point of the
// console half: a flow someone builds must be usable without a deploy.
func TestConsoleCreatesAFlowThatIsImmediatelyCallable(t *testing.T) {
	_, h := echoUpstream(t, "")
	up := newHTTPServer(t, h)
	ts, _, session := consoleFlowServer(t, up.URL)

	resp, out := consoleDo(t, ts, session, http.MethodPost, "/api/flows",
		`{"name":"mine","steps":[{"agent":"writer"},{"agent":"reviewer","label":"check"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save status = %d: %v", resp.StatusCode, out)
	}

	// Callable through the ordinary gateway door, right now — not after the
	// next restart.
	chat, chatOut := postFlow(t, ts, `{"model":"mine","messages":[{"role":"user","content":"brief"}]}`)
	if chat.StatusCode != http.StatusOK {
		t.Fatalf("chat status = %d: %v", chat.StatusCode, chatOut)
	}
	if got := content(t, chatOut); got != "<<brief>>" {
		t.Errorf("content = %q, want <<brief>>", got)
	}
}

// TestConsoleRefusesAStepNamingNothing — a step that names nothing is a model
// name which answers 502 to its first caller, by which time whoever wrote it
// has moved on.
func TestConsoleRefusesAStepNamingNothing(t *testing.T) {
	_, h := echoUpstream(t, "")
	up := newHTTPServer(t, h)
	ts, _, session := consoleFlowServer(t, up.URL)

	resp, out := consoleDo(t, ts, session, http.MethodPost, "/api/flows",
		`{"name":"broken","steps":[{"agent":"writer"},{"agent":"typo-here"}]}`)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if msg := flatten(out); !strings.Contains(msg, "typo-here") {
		t.Errorf("the error should name the bad step, got %v", out)
	}
}

// TestConsoleRefusesAFlowThatNamesItself — the one cycle cheap enough to catch
// before it is saved rather than when it is called.
func TestConsoleRefusesAFlowThatNamesItself(t *testing.T) {
	_, h := echoUpstream(t, "")
	up := newHTTPServer(t, h)
	ts, _, session := consoleFlowServer(t, up.URL)

	resp, _ := consoleDo(t, ts, session, http.MethodPost, "/api/flows",
		`{"name":"ouroboros","steps":[{"agent":"ouroboros"}]}`)

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// TestConsoleRefusesAnEmptyFlow — an empty chain is not a chain that does
// nothing, it is a model name that fails.
func TestConsoleRefusesAnEmptyFlow(t *testing.T) {
	_, h := echoUpstream(t, "")
	up := newHTTPServer(t, h)
	ts, _, session := consoleFlowServer(t, up.URL)

	resp, _ := consoleDo(t, ts, session, http.MethodPost, "/api/flows", `{"name":"hollow","steps":[]}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// TestConsoleFlowSurvivesARestart — the console writes to disk for exactly this
// reason; a flow that vanished on redeploy would take a business's setup with
// it.
func TestConsoleFlowSurvivesARestart(t *testing.T) {
	_, h := echoUpstream(t, "")
	up := newHTTPServer(t, h)

	dir := t.TempDir() + "/state.json"
	store, err := adminstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFlow(adminstore.FlowRecord{
		Name:  "persisted",
		Steps: []adminstore.FlowStepRecord{{Agent: "writer"}},
	}); err != nil {
		t.Fatal(err)
	}

	// A second Store over the same file is what a restart looks like.
	reopened, err := adminstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	agents := agent.NewRegistry()
	_ = agents.Add(agent.Agent{Name: "writer", Model: "nabu-fast", System: "you are the writer"})

	adapters := map[string]provider.Adapter{"parspack": provider.NewOpenAIAdapter("parspack", up.URL, "k", nil)}
	models := map[string]config.ModelRoute{"nabu-fast": {Primary: config.Target{Provider: "parspack", Model: "openai/gpt-5.5"}}}
	r := router.New(adapters, models, nil, nil, nil, nil, map[string][]string{"parspack": nil}, discardLogger())

	srv := New(r, policy.New(nil, nil), usage.New(nil), agents, discardLogger()).WithFlows(flow.NewRegistry())
	srv.SetAdminStore(reopened)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, out := postFlow(t, ts, `{"model":"persisted","messages":[{"role":"user","content":"brief"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %v", resp.StatusCode, out)
	}
}

// TestConsoleWillNotDeleteABakedFlow — deleting from the console what the repo
// declares would come back on the next deploy, which is worse than refusing.
func TestConsoleWillNotDeleteABakedFlow(t *testing.T) {
	_, h := echoUpstream(t, "")
	up := newHTTPServer(t, h)
	ts, srv, session := consoleFlowServer(t, up.URL)

	_ = srv.flows.Add(flow.Flow{Name: "baked", Steps: []flow.Step{{Agent: "writer"}}})

	resp, _ := consoleDo(t, ts, session, http.MethodDelete, "/api/flows/baked", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	if _, ok := srv.flows.Lookup("baked"); !ok {
		t.Error("the baked flow was removed anyway")
	}
}

// TestConsoleTestRunReturnsEveryStep — without the middle, a bad answer and a
// bad third step look identical.
func TestConsoleTestRunReturnsEveryStep(t *testing.T) {
	_, h := echoUpstream(t, "")
	up := newHTTPServer(t, h)
	ts, _, session := consoleFlowServer(t, up.URL)

	consoleDo(t, ts, session, http.MethodPost, "/api/flows",
		`{"name":"mine","steps":[{"agent":"writer"},{"agent":"reviewer"}]}`)

	resp, out := consoleDo(t, ts, session, http.MethodPost, "/api/flows/mine/test", `{"message":"brief"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %v", resp.StatusCode, out)
	}

	steps, _ := out["steps"].([]any)
	if len(steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(steps))
	}
	if got := steps[0].(map[string]any)["output"]; got != "<brief>" {
		t.Errorf("step 1 output = %v", got)
	}
	if got := out["output"]; got != "<<brief>>" {
		t.Errorf("output = %v", got)
	}
}

// TestConsoleFlowsNeedASession — this editor defines what models run and what
// they are told to do, so it is not a page to leave open.
func TestConsoleFlowsNeedASession(t *testing.T) {
	_, h := echoUpstream(t, "")
	up := newHTTPServer(t, h)
	ts, _, _ := consoleFlowServer(t, up.URL)

	for _, c := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/flows", ""},
		{http.MethodPost, "/api/flows", `{"name":"x","steps":[{"agent":"writer"}]}`},
		{http.MethodDelete, "/api/flows/x", ""},
		{http.MethodPost, "/api/flows/x/test", `{"message":"hi"}`},
	} {
		req, _ := http.NewRequest(c.method, ts.URL+c.path, strings.NewReader(c.body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", c.method, c.path, resp.StatusCode)
		}
	}
}

func flatten(out map[string]any) string {
	b, _ := json.Marshal(out)
	return string(b)
}
