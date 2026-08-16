package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeAgent drops one agent file into a fresh temp agents dir.
func writeAgent(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestAgentFileWithToolsParses verifies the tools: block of an agent YAML
// lands in the registry as runtime tools, schema and defaults intact.
func TestAgentFileWithToolsParses(t *testing.T) {
	t.Setenv("WOO_BASIC_TEST", "dGVzdA==")
	dir := writeAgent(t, "shop.yaml", `
name: shop-agent
model: nabu-smart
system: You help with orders.
max_tool_steps: 3
tools:
  - name: track_order
    type: http
    description: fetch an order
    method: GET
    url: "https://shop.example.com/orders/{order_id}"
    headers:
      Authorization: "Basic ${WOO_BASIC_TEST}"
    path_params: [order_id]
    parameters:
      type: object
      properties:
        order_id: {type: string, description: "شماره سفارش"}
      required: [order_id]
    timeout_ms: 8000
    max_response_bytes: 8192
  - name: follow_up
    type: http
    method: POST
    url: "https://vendor.example.com/api/followup"
    body_template: {order_number: "{{order_number}}", source: nabugate}
    parameters:
      type: object
      properties:
        order_number: {type: string}
      required: [order_number]
`)

	cfg := &Config{AgentsDir: dir}
	reg, warnings := cfg.BuildAgents()
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	ag, ok := reg.Lookup("shop-agent")
	if !ok {
		t.Fatal("agent not registered")
	}
	if ag.MaxSteps() != 3 {
		t.Errorf("max steps = %d", ag.MaxSteps())
	}
	if len(ag.Tools) != 2 {
		t.Fatalf("tools = %d", len(ag.Tools))
	}

	tr := ag.Tools[0]
	if tr.Name != "track_order" || tr.Method != "GET" {
		t.Errorf("tool = %+v", tr)
	}
	// Agent files expand ${VAR} at load, like the main config.
	if tr.Headers["Authorization"] != "Basic dGVzdA==" {
		t.Errorf("header = %q, want env-expanded at load", tr.Headers["Authorization"])
	}
	if !strings.Contains(string(tr.Parameters), `"order_id"`) ||
		!strings.Contains(string(tr.Parameters), `شماره سفارش`) {
		t.Errorf("parameters = %s", tr.Parameters)
	}
	if tr.TimeoutMS != 8000 || tr.MaxResponseBytes != 8192 {
		t.Errorf("limits = %d/%d", tr.TimeoutMS, tr.MaxResponseBytes)
	}
	if ag.Tools[1].BodyTemplate["source"] != "nabugate" {
		t.Errorf("body_template = %v", ag.Tools[1].BodyTemplate)
	}
}

// TestAgentFileWithBadToolSkipped: one broken tool must not take the agent —
// or the gateway — down; it warns and skips, like a provider with no key.
func TestAgentFileWithBadToolSkipped(t *testing.T) {
	dir := writeAgent(t, "broken.yaml", `
name: broken-agent
model: nabu-smart
tools:
  - name: call_everything
    type: http
    url: "https://api.example.com/x"
    parameters: {type: array}
`)

	cfg := &Config{AgentsDir: dir}
	reg, warnings := cfg.BuildAgents()
	if reg.Len() != 0 {
		t.Error("agent with an invalid tool should not register")
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "parameters") {
		t.Errorf("warnings = %v, want one naming the parameters problem", warnings)
	}
}

// TestExampleAgentsInRepoParse loads every agents/*.yaml shipped in the repo,
// so a malformed example fails CI rather than a deploy.
func TestExampleAgentsInRepoParse(t *testing.T) {
	repoAgents := filepath.Join("..", "..", "agents")
	if _, err := os.Stat(repoAgents); err != nil {
		t.Skip("repo agents dir not found")
	}
	cfg := &Config{AgentsDir: repoAgents}
	reg, warnings := cfg.BuildAgents()
	for _, w := range warnings {
		// Duplicates of inline names cannot happen here (no inline agents), so
		// any warning is a broken file.
		t.Errorf("agents dir warning: %s", w)
	}
	ag, ok := reg.Lookup("accountcity-support")
	if !ok {
		t.Fatal("accountcity-support example agent missing")
	}
	names := map[string]bool{}
	for _, tl := range ag.Tools {
		names[tl.Name] = true
	}
	if !names["track_order"] || !names["follow_up_order"] {
		t.Errorf("example tools = %v", names)
	}
}
