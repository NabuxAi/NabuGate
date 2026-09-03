// What each tool actually answers.
//
// mcp_test.go pins the transport, the envelope and the leak rule; those are the
// same seven properties in all four repos. This file is NabuGate's alone: it
// has the largest tool set of the four, and "the endpoint did not leak" is not
// the same claim as "the endpoint told the truth". A tool that returned an
// empty list would pass every test next door.
//
// The fixture is the one in mcp_test.go: one provider ("acme") holding a live
// key, a single-rung chat alias, a single-rung embedding alias, one agent with
// a credential-bearing tool, one project's usage, and two logged calls — one
// billed, one denied.
package mcp

import (
	"strings"
	"testing"
)

func TestModelsListReportsEachAliasAndItsLiveProviders(t *testing.T) {
	var got struct {
		Models []struct {
			ID        string   `json:"id"`
			Kind      string   `json:"kind"`
			Live      int      `json:"live_targets"`
			Providers []string `json:"providers"`
		} `json:"models"`
	}
	payload(t, callTool(t, newTestServer(t), "nabugate_models_list", ""), &got)

	if len(got.Models) != 2 {
		t.Fatalf("models = %d, want the fixture's two aliases: %+v", len(got.Models), got.Models)
	}

	// Ordered by kind then id, which is what the router already guarantees and
	// what makes two calls with the same config produce the same bytes.
	want := []struct {
		id   string
		kind string
	}{{"nabu-fast", "chat"}, {"nabu-embed", "embedding"}}

	for i, w := range want {
		m := got.Models[i]
		if m.ID != w.id || m.Kind != w.kind {
			t.Errorf("model %d = %s/%s, want %s/%s", i, m.ID, m.Kind, w.id, w.kind)
		}
		if m.Live != 1 {
			t.Errorf("%s live_targets = %d, want 1: the fixture's provider has a key", m.ID, m.Live)
		}
		if len(m.Providers) != 1 || m.Providers[0] != "acme" {
			t.Errorf("%s providers = %v, want [acme]", m.ID, m.Providers)
		}
	}
}

// The provider NAME is publishable; everything else about that provider is not.
// This is the assertion that would fail if a future edit swapped the hand-built
// view for the config struct the name came from.
func TestModelsListCarriesTheProviderNameAndNothingElseAboutIt(t *testing.T) {
	res := callTool(t, newTestServer(t), "nabugate_models_list", "")
	text := res.Content[0].Text

	if !strings.Contains(text, `"acme"`) {
		t.Fatalf("the provider name is the useful part of this answer and it is missing: %s", text)
	}
	for _, forbidden := range []string{"acme.example.invalid", theKeyEnvName, theProviderKey} {
		if strings.Contains(text, forbidden) {
			t.Errorf("models_list carried %q: %s", forbidden, text)
		}
	}
}

func TestHealthGetCountsDegradedAliasesAndSaysWhy(t *testing.T) {
	var got struct {
		Status   string `json:"status"`
		Degraded int    `json:"degraded"`
		Aliases  []struct {
			ID         string   `json:"id"`
			Live       int      `json:"live_targets"`
			Configured int      `json:"configured_targets"`
			Warnings   []string `json:"warnings"`
		} `json:"aliases"`
	}
	payload(t, callTool(t, newTestServer(t), "nabugate_health_get", ""), &got)

	// nabu-fast is a one-rung chat alias, which the router warns about. The
	// one-rung embedding alias is deliberately not warned about — crossing
	// vector widths mid-flight corrupts a stored index, so no fallback is the
	// correct configuration there.
	if got.Degraded != 1 {
		t.Errorf("degraded = %d, want 1 (nabu-fast has no fallback; nabu-embed is correctly single-rung)", got.Degraded)
	}
	if got.Status != "degraded" {
		t.Errorf("status = %q, want degraded", got.Status)
	}
	if len(got.Aliases) != 2 {
		t.Fatalf("aliases = %d, want 2", len(got.Aliases))
	}

	fast := got.Aliases[0]
	if fast.ID != "nabu-fast" {
		t.Fatalf("aliases[0] = %q, want nabu-fast", fast.ID)
	}
	if len(fast.Warnings) == 0 {
		t.Error("nabu-fast came back with no warnings, so the degraded count above is describing nothing")
	}
	if fast.Live != 1 || fast.Configured != 1 {
		t.Errorf("nabu-fast = %d/%d live/configured, want 1/1", fast.Live, fast.Configured)
	}
}

func TestAgentsListReturnsThreeFieldsAndNotThePromptOrItsTools(t *testing.T) {
	res := callTool(t, newTestServer(t), "nabugate_agents_list", "")

	var got struct {
		Agents []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Model       string `json:"model"`
		} `json:"agents"`
	}
	payload(t, res, &got)

	if len(got.Agents) != 1 {
		t.Fatalf("agents = %d, want the fixture's one: %+v", len(got.Agents), got.Agents)
	}

	a := got.Agents[0]
	if a.Name != "write-editor" || a.Model != "nabu-fast" || a.Description == "" {
		t.Errorf("agent = %+v, want write-editor on nabu-fast with a description", a)
	}

	// The fixture's agent carries a system prompt and an HTTP tool whose header
	// holds a downstream key. Both marshal perfectly well from agent.Agent,
	// which is exactly why this tool builds a three-field view by hand.
	text := res.Content[0].Text
	for _, forbidden := range []string{theAgentPrompt, theAgentToolKey, "lookup", "internal.example.invalid"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("agents_list carried %q: %s", forbidden, text)
		}
	}
}

func TestRequestsListReturnsNewestFirstWithTheDenialReason(t *testing.T) {
	var got struct {
		Requests []struct {
			At       string  `json:"at"`
			Project  string  `json:"project"`
			Provider string  `json:"provider"`
			Model    string  `json:"model"`
			Tokens   int64   `json:"tokens"`
			CostUSD  float64 `json:"cost_usd"`
			Denied   bool    `json:"denied"`
			Reason   string  `json:"reason"`
		} `json:"requests"`
	}
	payload(t, callTool(t, newTestServer(t), "nabugate_requests_list", ""), &got)

	if len(got.Requests) != 2 {
		t.Fatalf("requests = %d, want the fixture's two: %+v", len(got.Requests), got.Requests)
	}

	// Newest first. The denied row was logged one minute after the billed one.
	denied := got.Requests[0]
	if !denied.Denied || denied.Model != "nabu-smart" {
		t.Errorf("requests[0] = %+v, want the denied nabu-smart call first", denied)
	}
	// The reason is the whole content of the answer: "your key was refused" is
	// not actionable, "refused nabu-smart" is a one-line fix to the allow-list.
	if denied.Reason == "" {
		t.Error("a denied call came back with no reason, which is the only thing that makes the row useful")
	}

	billed := got.Requests[1]
	if billed.Denied || billed.Provider != "acme" || billed.Tokens != 1500 {
		t.Errorf("requests[1] = %+v, want the billed 1500-token acme call", billed)
	}
	if !strings.HasPrefix(billed.At, "2026-01-01T") {
		t.Errorf("at = %q, want an RFC3339 UTC timestamp", billed.At)
	}
}

// The ring lower-cases the project it compares against. A filter that forgot to
// would return zero rows and read as "this project sent no traffic", which is a
// worse answer than an error.
func TestRequestsListFiltersByProjectRegardlessOfCase(t *testing.T) {
	h := newTestServer(t)

	for _, project := range []string{"nabuwrite", "NabuWrite", "NABUWRITE"} {
		var got struct {
			Requests []struct {
				Project string `json:"project"`
			} `json:"requests"`
		}
		payload(t, callTool(t, h, "nabugate_requests_list", `{"project":"`+project+`"}`), &got)

		if len(got.Requests) != 1 {
			t.Errorf("filtering by %q returned %d rows, want 1", project, len(got.Requests))
		}
	}

	// And a project with no traffic is an empty list, not every project's rows.
	var none struct {
		Requests []struct{} `json:"requests"`
	}
	payload(t, callTool(t, h, "nabugate_requests_list", `{"project":"no-such-project"}`), &none)
	if len(none.Requests) != 0 {
		t.Errorf("an unknown project returned %d rows; a filter that falls open is how one project reads another's traffic", len(none.Requests))
	}
}

// limit is clamped into the range the schema advertises rather than rejected,
// so an out-of-range value costs the caller a re-read and not a round trip.
// Note that 0 clamps UP to the minimum of 1: it does not mean "no limit".
func TestRequestsListClampsTheLimit(t *testing.T) {
	h := newTestServer(t)

	cases := []struct {
		args string
		want int
	}{
		{`{"limit":1}`, 1},
		{`{"limit":0}`, 1},
		{`{"limit":-5}`, 1},
		{`{"limit":9999}`, 2}, // clamped to maxRequests; the fixture holds two rows
		{`{"limit":"lots"}`, 2},
		{`{}`, 2},
	}

	for _, tc := range cases {
		var got struct {
			Requests []struct{} `json:"requests"`
		}
		payload(t, callTool(t, h, "nabugate_requests_list", tc.args), &got)

		if len(got.Requests) != tc.want {
			t.Errorf("%s returned %d rows, want %d", tc.args, len(got.Requests), tc.want)
		}
	}
}

func TestUsageGetReportsTokensAndCostPerProjectAndPerModel(t *testing.T) {
	var got struct {
		Projects []statPayload `json:"projects"`
		Models   []statPayload `json:"models"`
	}
	payload(t, callTool(t, newTestServer(t), "nabugate_usage_get", ""), &got)

	if len(got.Projects) != 1 || got.Projects[0].Name != "nabuwrite" {
		t.Fatalf("projects = %+v, want one entry for nabuwrite", got.Projects)
	}
	if len(got.Models) != 1 || got.Models[0].Name != "acme/gpt-fast" {
		t.Fatalf("models = %+v, want one entry for acme/gpt-fast", got.Models)
	}

	p := got.Projects[0]
	if p.Requests != 1 || p.PromptTokens != 1000 || p.CompletionTokens != 500 {
		t.Errorf("project usage = %+v, want 1 request / 1000 in / 500 out", p)
	}
	// 1000 in at $1 per 1M plus 500 out at $2 per 1M.
	if p.CostUSD != 0.002 {
		t.Errorf("cost_usd = %v, want 0.002", p.CostUSD)
	}
}

func TestUsageGetScopedToOneProjectReturnsOnlyThatProject(t *testing.T) {
	var got struct {
		Projects []statPayload `json:"projects"`
		Models   []statPayload `json:"models"`
	}
	payload(t, callTool(t, newTestServer(t), "nabugate_usage_get", `{"project":"nabuwrite"}`), &got)

	if len(got.Projects) != 1 || got.Projects[0].Name != "nabuwrite" {
		t.Fatalf("projects = %+v, want just nabuwrite", got.Projects)
	}
	// The per-model breakdown is every project's spend by model. Scoping to one
	// project must not hand it over as a consolation prize.
	if len(got.Models) != 0 {
		t.Errorf("models = %+v, want none when the answer is scoped to one project", got.Models)
	}
}

// An unknown project is a ToolFailure, not an empty result: a model handed
// {"projects":[]} concludes the project spent nothing, which is a different and
// wrong answer from "there is no such project here".
func TestUsageGetOnAnUnknownProjectSaysSoRatherThanReturningZero(t *testing.T) {
	res := callTool(t, newTestServer(t), "nabugate_usage_get", `{"project":"no-such-project"}`)

	if !res.IsError {
		t.Fatalf("an unknown project came back as a normal result: %s", res.Content[0].Text)
	}
	if !strings.Contains(res.Content[0].Text, "no-such-project") {
		t.Errorf("the message does not name what was asked for: %q", res.Content[0].Text)
	}
}

type statPayload struct {
	Name             string  `json:"name"`
	Requests         int64   `json:"requests"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}
