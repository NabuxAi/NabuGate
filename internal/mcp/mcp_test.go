// The MCP endpoint's contract, pinned.
//
// NabuGate holds every provider key in the estate, mints the tokens the other
// products call it with, and knows what each of them spent. The MCP endpoint
// reads the same data the admin console reads. So the properties worth a test
// are: no token is 401, a wrong token is 401, a token with the right prefix is
// still 401, GET is 405, a notification gets 202 and no body, the four methods
// answer in the exact envelope a client expects, the two failure kinds land in
// the right envelope, and no response — success or failure — ever contains a
// secret.
//
// Per-tool payload assertions live next door in tools_test.go.
package mcp

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nabugate/internal/adminstore"
	"nabugate/internal/agent"
	"nabugate/internal/config"
	"nabugate/internal/policy"
	"nabugate/internal/provider"
	"nabugate/internal/router"
	"nabugate/internal/usage"
)

// The needles. Every one of these is a real NabuGate secret shape, seeded into
// the fixture so the leak test is checking this repo's own exposure rather than
// a sibling service's. Copying another repo's needle list here would leave the
// test green while the gateway published its provider keys.
const (
	// theProviderKey is what a provider adapter holds in memory. cfg.Providers
	// below is fed through the real BuildAdapters, so the router in this
	// fixture is backed by live adapters carrying this value.
	theProviderKey = "sk-live-provider-key-must-never-appear"

	// theKeyEnvName is the NAME of the variable that key arrives in. Withheld
	// as well as the value: the names are a map of where to look.
	theKeyEnvName = "NABUGATE_TEST_PROVIDER_KEY"

	// theBaseURLSecret is embedded in a provider base_url, which is the other
	// place a credential hides in this config — some gateways carry the key in
	// the query string.
	theBaseURLSecret = "sk-key-embedded-in-a-base-url"

	// theConsoleToken is a console-minted project token: the value a project
	// puts in its Authorization header to call this gateway.
	theConsoleToken = "ngk-console-minted-token-must-never-appear"

	// theAgentToolKey is the credential an agent's HTTP tool sends downstream.
	// It lives in agent.Tool.Headers, which is why nabugate_agents_list returns
	// three hand-picked fields rather than the agent.
	theAgentToolKey = "sk-agent-tool-header-must-never-appear"

	// theAgentPrompt is a system prompt. Not a credential, but not the MCP
	// endpoint's to publish either.
	theAgentPrompt = "you are a private system prompt that must never appear"
)

const testToken = "mcp-test-token"

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func newTestServer(t *testing.T) http.Handler {
	t.Helper()

	return newTestServerWith(t, testFixture(t))
}

// fixture is everything Register takes, built once so tools_test.go can reach
// the same values it asserts against.
type fixture struct {
	router   *router.Router
	tracker  *usage.Tracker
	agents   *agent.Registry
	requests *adminstore.RequestLog
}

func testFixture(t *testing.T) fixture {
	t.Helper()

	// The key is in the process environment exactly as it is in production, so
	// BuildAdapters below produces adapters that really are holding it.
	t.Setenv(theKeyEnvName, theProviderKey)

	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"acme": {
				Enabled:   true,
				Type:      "openai",
				BaseURL:   "https://acme.example.invalid/v1?key=" + theBaseURLSecret,
				APIKeyEnv: theKeyEnvName,
			},
		},
		Models: map[string]config.ModelRoute{
			// One rung, so this alias is reported degraded — the fixture needs
			// a warning in it for the health assertions to mean anything.
			"nabu-fast": {Primary: config.Target{Provider: "acme", Model: "gpt-fast"}},
		},
		Embeddings: map[string]config.ModelRoute{
			"nabu-embed": {Primary: config.Target{Provider: "acme", Model: "embed-1"}},
		},
		Server: config.ServerConfig{
			Keys: []policy.KeyConfig{{Key: theConsoleToken, Project: "nabuwrite", Allow: []string{"nabu-*"}}},
		},
		MCP: config.MCP{Enabled: true, Path: "/mcp", TokenEnv: "NABUGATE_MCP_TOKEN"},
	}

	adapters, warnings := cfg.BuildAdapters()
	if len(adapters) == 0 {
		t.Fatalf("fixture built no adapters, so nothing is holding the needle: %v", warnings)
	}

	r := router.New(adapters, cfg.Models, nil, nil, cfg.Embeddings, nil, nil, testLogger())

	tracker := usage.New(map[string]usage.Price{"acme/gpt-fast": {Input: 1, Output: 2}})
	tracker.Record("nabuwrite", "acme", "gpt-fast",
		provider.Usage{PromptTokens: 1000, CompletionTokens: 500, TotalTokens: 1500})

	agents := agent.NewRegistry()
	if err := agents.Add(agent.Agent{
		Name:        "write-editor",
		Description: "Edits a draft.",
		Model:       "nabu-fast",
		System:      theAgentPrompt,
		Tools: []agent.Tool{{
			Name:        "lookup",
			Type:        agent.ToolTypeHTTP,
			Description: "Looks something up.",
			URL:         "https://internal.example.invalid/lookup",
			// This is the shape the secret rule exists for: a downstream
			// credential sitting in a struct that marshals perfectly well.
			Headers:    map[string]string{"Authorization": "Bearer " + theAgentToolKey},
			Parameters: json.RawMessage(`{"type":"object","properties":{}}`),
		}},
	}); err != nil {
		t.Fatalf("fixture agent rejected: %v", err)
	}

	requests := adminstore.NewRequestLog(10)
	requests.Add(adminstore.RequestEntry{
		At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		// Mixed case on purpose: the ring lower-cases what it compares, and a
		// filter that forgot to would silently return nothing.
		Project: "NabuWrite", Provider: "acme", Model: "gpt-fast",
		Tokens: 1500, CostUSD: 0.002,
	})
	requests.Add(adminstore.RequestEntry{
		At:      time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC),
		Project: "cinematic", Model: "nabu-smart",
		Denied: true, Reason: "alias not permitted for this key",
	})

	return fixture{router: r, tracker: tracker, agents: agents, requests: requests}
}

func newTestServerWith(t *testing.T, f fixture) http.Handler {
	t.Helper()

	srv := New("nabugate", Version, "/mcp", testToken, testLogger())
	Register(srv, f.router, f.tracker, f.agents, f.requests)

	return srv.Handler()
}

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(discard{}, nil)) }

// post sends one JSON-RPC body and returns the recorder.
func post(t *testing.T, h http.Handler, token, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	return rec
}

// rpc is the shape every answer is decoded into.
type rpc struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) rpc {
	t.Helper()

	var out rpc
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (body %s)", err, rec.Body.String())
	}

	return out
}

// toolResult is the tools/call envelope as a client sees it.
type toolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

// callTool runs one tool and returns its envelope, failing the test on any
// JSON-RPC-level error.
func callTool(t *testing.T, h http.Handler, name, arguments string) toolResult {
	t.Helper()

	if arguments == "" {
		arguments = "{}"
	}

	rec := post(t, h, testToken,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"`+name+`","arguments":`+arguments+`}}`)

	got := decode(t, rec)
	if got.Error != nil {
		t.Fatalf("%s: JSON-RPC error %d: %s", name, got.Error.Code, got.Error.Message)
	}

	var out toolResult
	if err := json.Unmarshal(got.Result, &out); err != nil {
		t.Fatalf("%s: decode result: %v", name, err)
	}

	if len(out.Content) != 1 || out.Content[0].Type != "text" {
		t.Fatalf("%s: content = %+v, want exactly one text block", name, out.Content)
	}

	return out
}

// payload unmarshals a successful tool's text block into v.
func payload(t *testing.T, res toolResult, v any) {
	t.Helper()

	if res.IsError {
		t.Fatalf("isError set: %s", res.Content[0].Text)
	}

	if err := json.Unmarshal([]byte(res.Content[0].Text), v); err != nil {
		t.Fatalf("the text block must be the JSON payload: %v (%q)", err, res.Content[0].Text)
	}
}

// The gate. Without this the endpoint hands an anonymous caller every project's
// spend and every alias this deployment can reach.
func TestTheEndpointRefusesAnythingButTheConfiguredToken(t *testing.T) {
	h := newTestServer(t)

	cases := []struct {
		name  string
		token string
		want  int
	}{
		{"no token", "", http.StatusUnauthorized},
		{"wrong token", "not-the-token", http.StatusUnauthorized},
		// The row that proves the compare is constant-time-shaped rather than a
		// prefix match.
		{"token with the right prefix", testToken + "-extra", http.StatusUnauthorized},
		{"the configured token", testToken, http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := post(t, h, tc.token, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

// An unset token must leave the route unmounted rather than open. There is no
// dev-mode-open path here, which is the one rule that differs from the
// surrounding code: policy.Enforcer.Enabled() being false makes /v1/* open, and
// that idiom must never be carried onto /mcp.
func TestAnUnsetTokenDisablesTheEndpointRatherThanOpeningIt(t *testing.T) {
	f := testFixture(t)

	srv := New("nabugate", Version, "/mcp", "", testLogger())
	Register(srv, f.router, f.tracker, f.agents, f.requests)

	if srv.Enabled() {
		t.Fatal("a server with no token reports Enabled: Handler() would be mounted and the endpoint would be open")
	}

	// And if it were mounted anyway, it still refuses everything.
	rec := post(t, srv.Handler(), "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("an empty configured token accepted an empty presented token: status = %d", rec.Code)
	}
}

// A client that opens the SSE stream or tears a session down must be told this
// endpoint is POST-only rather than that the path does not exist.
func TestNonPostIsRefusedWith405(t *testing.T) {
	h := newTestServer(t)

	for _, method := range []string{http.MethodGet, http.MethodDelete, http.MethodPut} {
		req := httptest.NewRequest(method, "/mcp", nil)
		req.Header.Set("Authorization", "Bearer "+testToken)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /mcp = %d, want 405", method, rec.Code)
		}
		if got := rec.Header().Get("Allow"); got != http.MethodPost {
			t.Errorf("%s /mcp Allow = %q, want POST", method, got)
		}
	}
}

// notifications/initialized arrives immediately after initialize and carries no
// id. Answering it with a JSON-RPC result and a null id is the classic way a
// hand-rolled server breaks a client, so the 202 is pinned.
func TestNotificationsGetAcceptedWithNoBody(t *testing.T) {
	rec := post(t, newTestServer(t), testToken, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("notification = %d, want 202", rec.Code)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "" {
		t.Errorf("notification answered with a body: %s", body)
	}
}

func TestInitializeAnswersTheExactHandshake(t *testing.T) {
	rec := post(t, newTestServer(t), testToken,
		`{"jsonrpc":"2.0","id":"abc","method":"initialize","params":{"protocolVersion":"2024-11-05"}}`)

	got := decode(t, rec)

	if got.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want 2.0", got.JSONRPC)
	}
	// A string id must come back as the same string, not as a number or null.
	// An implementer who typed `ID int` fails here and nowhere else.
	if string(got.ID) != `"abc"` {
		t.Errorf("id = %s, want \"abc\"", got.ID)
	}

	var result struct {
		ProtocolVersion string         `json:"protocolVersion"`
		Capabilities    map[string]any `json:"capabilities"`
		ServerInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(got.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}

	if result.ProtocolVersion != ProtocolVersion {
		t.Errorf("protocolVersion = %q, want %q", result.ProtocolVersion, ProtocolVersion)
	}
	if _, ok := result.Capabilities["tools"]; !ok {
		t.Errorf("capabilities = %v, want a tools entry", result.Capabilities)
	}
	if result.ServerInfo.Name != "nabugate" {
		t.Errorf("serverInfo.name = %q, want nabugate", result.ServerInfo.Name)
	}
}

// ping's result is present, not omitted: a client that reads result as a
// liveness signal sees nothing if it is dropped by omitempty.
func TestPingAnswersWithAPresentResult(t *testing.T) {
	rec := post(t, newTestServer(t), testToken, `{"jsonrpc":"2.0","id":7,"method":"ping"}`)

	got := decode(t, rec)
	if got.Error != nil {
		t.Fatalf("ping errored: %d %s", got.Error.Code, got.Error.Message)
	}
	if string(got.ID) != "7" {
		t.Errorf("id = %s, want 7 (a numeric id must survive as a number)", got.ID)
	}
	if string(got.Result) != "{}" {
		t.Errorf("result = %s, want {}", got.Result)
	}
}

// Every tool this service exposes is named here. The test fails when a tool is
// added or renamed, which is the point: the name is the contract four services
// share, and a drifted one is only noticed by the client that breaks.
func TestToolsListIsExactlyTheDeclaredSet(t *testing.T) {
	rec := post(t, newTestServer(t), testToken, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)

	var result struct {
		Tools []struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(decode(t, rec).Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}

	want := []string{
		"nabugate_agents_list",
		"nabugate_health_get",
		"nabugate_models_list",
		"nabugate_requests_list",
		"nabugate_usage_get",
	}

	if len(result.Tools) != len(want) {
		t.Fatalf("tools = %d, want %d", len(result.Tools), len(want))
	}

	for i, name := range want {
		if result.Tools[i].Name != name {
			t.Errorf("tool %d = %q, want %q (the list must be sorted)", i, result.Tools[i].Name, name)
		}
		if result.Tools[i].Description == "" {
			t.Errorf("tool %q has no description", name)
		}
		if result.Tools[i].InputSchema["type"] != "object" {
			t.Errorf("tool %q inputSchema type = %v, want object", name, result.Tools[i].InputSchema["type"])
		}
		if _, ok := result.Tools[i].InputSchema["properties"]; !ok {
			t.Errorf("tool %q inputSchema has no properties map (empty is {} , not absent)", name)
		}
	}
}

// The two failure kinds, kept apart. A model can act on "no usage for that
// project"; only whoever wrote the client can act on "no such tool".
func TestFailuresAreSortedIntoTheRightEnvelope(t *testing.T) {
	h := newTestServer(t)

	cases := []struct {
		name     string
		body     string
		wantRPC  int  // JSON-RPC error code, 0 for none
		wantTool bool // result.isError
	}{
		{
			name:    "unknown method",
			body:    `{"jsonrpc":"2.0","id":1,"method":"resources/list"}`,
			wantRPC: CodeMethodNotFound,
		},
		{
			name:    "unknown tool",
			body:    `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nabugate_keys_dump","arguments":{}}}`,
			wantRPC: CodeMethodNotFound,
		},
		{
			name:    "params that are not an object",
			body:    `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":"nope"}`,
			wantRPC: CodeInvalidParams,
		},
		{
			name:     "a filter that matched nothing",
			body:     `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nabugate_usage_get","arguments":{"project":"no-such-project"}}}`,
			wantTool: true,
		},
		{
			name: "a tool called with no arguments key at all",
			body: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nabugate_models_list"}}`,
		},
		{
			name: "an argument of the wrong type falls back rather than panicking",
			body: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nabugate_requests_list","arguments":{"limit":"lots"}}}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := post(t, h, testToken, tc.body)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: a JSON-RPC failure is still an HTTP 200", rec.Code)
			}

			got := decode(t, rec)

			if tc.wantRPC != 0 {
				if got.Error == nil {
					t.Fatalf("want JSON-RPC error %d, got result %s", tc.wantRPC, got.Result)
				}
				if got.Error.Code != tc.wantRPC {
					t.Errorf("error code = %d, want %d", got.Error.Code, tc.wantRPC)
				}

				return
			}

			if got.Error != nil {
				t.Fatalf("want a result, got JSON-RPC error %d: %s", got.Error.Code, got.Error.Message)
			}

			var result toolResult
			if err := json.Unmarshal(got.Result, &result); err != nil {
				t.Fatalf("decode result: %v", err)
			}

			if result.IsError != tc.wantTool {
				t.Errorf("isError = %v, want %v", result.IsError, tc.wantTool)
			}
			if len(result.Content) != 1 || result.Content[0].Type != "text" {
				t.Fatalf("content = %+v, want exactly one text block", result.Content)
			}
		})
	}
}

// An unparseable body has no id to echo, so the answer carries none — and is
// still an HTTP 200 with a JSON-RPC envelope.
func TestAnUnparseableBodyIsAParseError(t *testing.T) {
	rec := post(t, newTestServer(t), testToken, `not json at all`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	got := decode(t, rec)
	if got.Error == nil || got.Error.Code != CodeParseError {
		t.Fatalf("want %d, got %+v", CodeParseError, got.Error)
	}
	if len(got.ID) != 0 && string(got.ID) != "null" {
		t.Errorf("id = %s, want none: no id could be read from the body", got.ID)
	}
}

// The rule the whole endpoint exists under: whatever a caller asks for, and
// however it fails, no secret comes back.
//
// THIS TEST IS ADAPTED PER REPO, NOT COPIED. The needles are NabuGate's own
// secrets — a provider key and the name of the variable it arrives in, a key
// embedded in a base_url, a console-minted project token, an agent tool's
// downstream credential, a system prompt — each seeded into the fixture above.
// A copied needle list would leave this green while the gateway leaked.
func TestNoResponseEverContainsASecret(t *testing.T) {
	h := newTestServer(t)

	bodies := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}`,
		`{"jsonrpc":"2.0","id":1,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nabugate_agents_list","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nabugate_health_get","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nabugate_models_list","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nabugate_requests_list","arguments":{"limit":200}}}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nabugate_usage_get","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nabugate_usage_get","arguments":{"project":"nabuwrite"}}}`,
		// The failure paths, which is where an error string would carry an
		// upstream URL or an echoed argument.
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nabugate_usage_get","arguments":{"project":"no-such-project"}}}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nabugate_keys_dump","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":1,"method":"resources/list"}`,
		`not json at all`,
	}

	needles := []string{
		theProviderKey,
		theKeyEnvName,
		theBaseURLSecret,
		theConsoleToken,
		theAgentToolKey,
		theAgentPrompt,
		// The MCP token itself: an echo of it in an error message would put it
		// in the client's transcript.
		testToken,
		// Generic shapes, so a future tool that returns a raw record trips the
		// test even if the fixture's needles happen not to be in that record.
		"api_key",
		"api_key_env",
		"password_hash",
		"client_secret",
		"base_url",
	}

	for _, body := range bodies {
		got := post(t, h, testToken, body).Body.String()

		for _, needle := range needles {
			if strings.Contains(got, needle) {
				t.Errorf("response to %s leaked %q: %s", body, needle, got)
			}
		}
	}

	// The transport-level refusals are answers too, and the wrong-token one is
	// the single most likely place a server echoes what it was handed.
	for _, token := range []string{"", "wrong-token", testToken + "-extra"} {
		got := post(t, h, token, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`).Body.String()
		if strings.Contains(got, testToken) || strings.Contains(got, "wrong-token") {
			t.Errorf("the 401 body echoed the presented token: %s", got)
		}
	}
}
