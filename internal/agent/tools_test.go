package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// validTool is a complete, valid declaration; tests mutate one field at a time.
func validTool() Tool {
	return Tool{
		Name:        "track_order",
		Type:        ToolTypeHTTP,
		Description: "fetch an order",
		Method:      "GET",
		URL:         "https://shop.example.com/orders/{order_id}",
		PathParams:  []string{"order_id"},
		Parameters:  json.RawMessage(`{"type":"object","properties":{"order_id":{"type":"string"}},"required":["order_id"]}`),
	}
}

func TestToolValidate(t *testing.T) {
	if err := validTool().Validate(); err != nil {
		t.Fatalf("valid tool rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*Tool)
	}{
		{"bad name", func(tl *Tool) { tl.Name = "track order!" }},
		{"empty type", func(tl *Tool) { tl.Type = "" }},
		{"unknown type", func(tl *Tool) { tl.Type = "datastore" }},
		{"non-http scheme", func(tl *Tool) { tl.URL = "ftp://shop.example.com/x" }},
		{"missing url host", func(tl *Tool) { tl.URL = "https://" }},
		{"bad method", func(tl *Tool) { tl.Method = "TELEPORT" }},
		{"no parameters", func(tl *Tool) { tl.Parameters = nil }},
		{"non-object parameters", func(tl *Tool) { tl.Parameters = json.RawMessage(`{"type":"array"}`) }},
		{"broken parameters", func(tl *Tool) { tl.Parameters = json.RawMessage(`{`) }},
		{"timeout too big", func(tl *Tool) { tl.TimeoutMS = MaxToolTimeoutMS + 1 }},
		{"response cap too big", func(tl *Tool) { tl.MaxResponseBytes = MaxResponseBytesCap + 1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := validTool()
			tc.mutate(&tool)
			if err := tool.Validate(); err == nil {
				t.Fatalf("expected validation error for %s", tc.name)
			}
		})
	}
}

func TestRegistryRejectsDuplicateToolNames(t *testing.T) {
	reg := NewRegistry()
	dup := validTool()
	dup.Name = "track_order"
	err := reg.Add(Agent{Name: "a", Model: "m", Tools: []Tool{validTool(), dup}})
	if err == nil || !strings.Contains(err.Error(), "duplicate tool") {
		t.Fatalf("err = %v, want duplicate tool rejection", err)
	}
}

func TestRegistryRejectsBadMaxToolSteps(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Add(Agent{Name: "a", Model: "m", MaxToolSteps: MaxToolStepsCap + 1}); err == nil {
		t.Fatal("expected max_tool_steps rejection")
	}
}

func TestOpenAIToolsWireShape(t *testing.T) {
	wire := OpenAITools([]Tool{validTool()})
	if len(wire) != 1 {
		t.Fatalf("len = %d", len(wire))
	}
	if wire[0]["type"] != "function" {
		t.Errorf("type = %v", wire[0]["type"])
	}
	fn, ok := wire[0]["function"].(map[string]any)
	if !ok {
		t.Fatalf("function key missing: %v", wire[0])
	}
	if fn["name"] != "track_order" || fn["description"] != "fetch an order" {
		t.Errorf("function = %v", fn)
	}
	// Parameters must ride through as the raw JSON schema, not a re-encoded string.
	params, _ := json.Marshal(fn["parameters"])
	if !strings.Contains(string(params), `"order_id"`) {
		t.Errorf("parameters = %s", params)
	}
}

func TestBuildURL(t *testing.T) {
	tool := validTool()
	tool.URL = "https://shop.example.com/orders/{order_id}?lang=fa"

	got, err := tool.buildURL(map[string]any{"order_id": "A 1024/ب", "extra": "x"})
	if err != nil {
		t.Fatal(err)
	}
	// Path param substituted and escaped; the undeclared leftover argument
	// becomes a query parameter; the pre-existing query survives.
	if !strings.Contains(got, "/orders/A%201024%2F%D8%A8") {
		t.Errorf("path substitution missing from %q", got)
	}
	if !strings.Contains(got, "extra=x") || !strings.Contains(got, "lang=fa") {
		t.Errorf("query params wrong in %q", got)
	}

	if _, err := tool.buildURL(map[string]any{}); err == nil {
		t.Error("missing path param should error")
	}

	broken := validTool()
	broken.PathParams = nil // placeholder left undeclared
	if _, err := broken.buildURL(map[string]any{"order_id": "7"}); err == nil ||
		!strings.Contains(err.Error(), "path_params") {
		t.Errorf("unfilled placeholder err = %v, want path_params hint", err)
	}

	scheme := validTool()
	scheme.URL = "gopher://old.example.com/x"
	if _, err := scheme.buildURL(map[string]any{"order_id": "7"}); err == nil {
		t.Error("non-http scheme should error at build time")
	}
}

func TestBuildURLExpandsEnv(t *testing.T) {
	t.Setenv("TOOL_TEST_HOST", "shop.example.com")
	tool := validTool()
	tool.URL = "https://${TOOL_TEST_HOST}/orders/{order_id}"
	got, err := tool.buildURL(map[string]any{"order_id": "7"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://shop.example.com/orders/7" {
		t.Errorf("url = %q", got)
	}
}

// toolHTTPServer-style fakes are inlined per test above; each records what it
// needs in closure variables.

func TestExecuteGetSendsDeclaredHeadersOnly(t *testing.T) {
	var gotAuth, gotExtra, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotExtra = r.Header.Get("X-Shop")
		gotUA = r.Header.Get("User-Agent")
		fmt.Fprint(w, `{"status":"paid"}`)
	}))
	t.Cleanup(srv.Close)

	// Set AFTER the Tool value exists: interpolation happens at call time.
	t.Setenv("TOOL_TEST_BASIC", "dG9rZW46c2VjcmV0")

	tool := validTool()
	tool.URL = srv.URL + "/orders/{order_id}"
	tool.Headers = map[string]string{
		"Authorization": "Basic ${TOOL_TEST_BASIC}",
		"X-Shop":        "order-{{order_id}}",
	}

	exec := NewToolExecutorForTest(true)
	out, err := exec.Execute(t.Context(), tool, map[string]any{"order_id": "42"})
	if err != nil {
		t.Fatal(err)
	}
	if out != `{"status":"paid"}` {
		t.Errorf("out = %q", out)
	}
	if gotAuth != "Basic dG9rZW46c2VjcmV0" {
		t.Errorf("Authorization = %q, want env-interpolated declared header", gotAuth)
	}
	if gotExtra != "order-42" {
		t.Errorf("X-Shop = %q, want arg-templated header", gotExtra)
	}
	if !strings.HasPrefix(gotUA, "NabuGate/") {
		t.Errorf("User-Agent = %q", gotUA)
	}
}

func TestExecutePostFillsBodyTemplate(t *testing.T) {
	var gotBody []byte
	var gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotCT = r.Header.Get("Content-Type")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	t.Cleanup(srv.Close)

	tool := validTool()
	tool.Name = "follow_up"
	tool.Method = "POST"
	tool.URL = srv.URL + "/api/followup"
	tool.PathParams = nil
	tool.BodyTemplate = map[string]any{
		"order_number": "{{order_number}}", // exact placeholder: keeps JSON type
		"message":      "پیگیری: {{message}}",
		"count":        "{{count}}",
		"source":       "nabugate",
		"nested":       map[string]any{"ref": "{{order_number}}"},
	}

	exec := NewToolExecutorForTest(true)
	_, err := exec.Execute(t.Context(), tool, map[string]any{
		"order_number": "77",
		"message":      "کجاست؟",
		"count":        float64(3),
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q", gotCT)
	}
	var sent map[string]any
	if err := json.Unmarshal(gotBody, &sent); err != nil {
		t.Fatalf("body not JSON: %v (%s)", err, gotBody)
	}
	if sent["order_number"] != "77" || sent["message"] != "پیگیری: کجاست؟" || sent["source"] != "nabugate" {
		t.Errorf("body = %v", sent)
	}
	if sent["count"] != float64(3) {
		t.Errorf("exact placeholder should keep JSON type, count = %#v", sent["count"])
	}
	nested, _ := sent["nested"].(map[string]any)
	if nested["ref"] != "77" {
		t.Errorf("nested template = %v", nested)
	}
}

func TestExecuteBlocksPrivateAddresses(t *testing.T) {
	// A loopback httptest server standing in for an internal target: the
	// default executor must refuse to dial it at all.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("SSRF guard let a loopback request through")
	}))
	t.Cleanup(srv.Close)

	tool := validTool()
	tool.URL = srv.URL + "/orders/{order_id}"

	exec := NewToolExecutorForTest(false)
	_, err := exec.Execute(t.Context(), tool, map[string]any{"order_id": "1"})
	if err == nil || !strings.Contains(err.Error(), EnvToolSSRAllow) {
		t.Fatalf("err = %v, want SSRF block naming %s", err, EnvToolSSRAllow)
	}

	// The cloud metadata endpoint is the classic SSRF target; an IP literal
	// needs no DNS, so the guard must refuse it without any network at all.
	meta := validTool()
	meta.URL = "http://169.254.169.254/latest/meta-data"
	if _, err := exec.Execute(t.Context(), meta, map[string]any{"order_id": "1"}); err == nil {
		t.Fatal("link-local metadata address was not blocked")
	}
}

func TestExecuteTruncatesLongResponses(t *testing.T) {
	big := strings.Repeat("الف", 5000) // well past the cap below
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, big)
	}))
	t.Cleanup(srv.Close)

	tool := validTool()
	tool.URL = srv.URL + "/orders/{order_id}"
	tool.MaxResponseBytes = 256

	exec := NewToolExecutorForTest(true)
	out, err := exec.Execute(t.Context(), tool, map[string]any{"order_id": "1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) > 256+len("…[truncated]") || !strings.HasSuffix(out, "…[truncated]") {
		t.Errorf("len(out) = %d, want 256 + marker", len(out))
	}
}

func TestExecuteReportsHTTPErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"no such order"}`)
	}))
	t.Cleanup(srv.Close)

	tool := validTool()
	tool.URL = srv.URL + "/orders/{order_id}"

	exec := NewToolExecutorForTest(true)
	_, err := exec.Execute(t.Context(), tool, map[string]any{"order_id": "999"})
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("err = %v, want HTTP status surfaced", err)
	}
}

func TestExecuteHonoursTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		fmt.Fprint(w, "{}")
	}))
	t.Cleanup(srv.Close)

	tool := validTool()
	tool.URL = srv.URL + "/orders/{order_id}"
	tool.TimeoutMS = 50

	exec := NewToolExecutorForTest(true)
	start := time.Now()
	if _, err := exec.Execute(t.Context(), tool, map[string]any{"order_id": "1"}); err == nil {
		t.Fatal("expected a timeout error")
	}
	if time.Since(start) > 250*time.Millisecond {
		t.Errorf("timeout not honoured; took %s", time.Since(start))
	}
}

func TestExecuteCapsRedirects(t *testing.T) {
	var hops int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops++
		http.Redirect(w, r, "/next", http.StatusFound)
	}))
	t.Cleanup(srv.Close)

	tool := validTool()
	tool.URL = srv.URL + "/orders/{order_id}"

	exec := NewToolExecutorForTest(true)
	if _, err := exec.Execute(t.Context(), tool, map[string]any{"order_id": "1"}); err == nil ||
		!strings.Contains(err.Error(), "redirect") {
		t.Fatalf("err = %v, want redirect cap", err)
	}
	if hops > maxToolRedirects+1 {
		t.Errorf("followed %d hops, cap is %d", hops, maxToolRedirects)
	}
}

func TestMaxStepsDefault(t *testing.T) {
	var a Agent
	if a.MaxSteps() != DefaultMaxToolSteps {
		t.Errorf("default = %d", a.MaxSteps())
	}
	a.MaxToolSteps = 2
	if a.MaxSteps() != 2 {
		t.Errorf("override = %d", a.MaxSteps())
	}
}
