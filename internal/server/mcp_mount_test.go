// How the MCP endpoint is attached to this server's mux.
//
// The MCP contract asks for a BARE path — the equivalent of
// mux.Handle("/mcp", …) rather than "POST /mcp" — so that a client opening an
// SSE stream with GET, or tearing a session down with DELETE, is told the
// endpoint is POST-only instead of that it does not exist.
//
// On THIS mux that registration panics at startup. The admin console is
// registered as "GET /", and under Go 1.22 pattern precedence "/mcp" matches
// more methods while "GET /" matches fewer paths, so neither pattern is a
// subset of the other and net/http rejects the pair. The console bundle is
// committed to the repo, so web.Assets() is true in every build: this would
// have been a panic on boot inside the distroless image, which no test of the
// mcp package alone would have caught.
//
// So Handler() intercepts the one exact path ahead of the mux instead. These
// tests pin both halves of that: the endpoint is reachable with every method,
// and nothing else about the server moved.
package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nabugate/internal/agent"
	"nabugate/internal/mcp"
	"nabugate/internal/policy"
	"nabugate/internal/router"
	"nabugate/internal/usage"
)

const mcpTestToken = "server-mount-test-token"

// mountTestServer builds a Server with the MCP endpoint attached or not.
// Deliberately not using newTestServer: this needs the handler itself, and
// building it is the operation under test — Handler() panicking is the failure
// being guarded against.
func mountTestServer(t *testing.T, token string) http.Handler {
	t.Helper()

	r := router.New(nil, nil, nil, nil, nil, nil, nil, discardLogger())
	srv := New(r, policy.New(nil, nil), usage.New(nil), agent.NewRegistry(), discardLogger())

	m := mcp.New("nabugate", mcp.Version, "/mcp", token, discardLogger())
	mcp.Register(m, r, usage.New(nil), agent.NewRegistry(), srv.Requests())

	return srv.WithMCP(m).Handler()
}

// The one that would have fired on boot. If Handler() ever goes back to
// registering the bare path on the mux, this panics rather than fails.
func TestHandlerDoesNotPanicWithBothTheConsoleAndMCPMounted(t *testing.T) {
	h := mountTestServer(t, mcpTestToken)

	// And the console still serves, so the interception did not swallow it.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("GET /healthz = %d, want 200: mounting MCP must not disturb the existing routes", rec.Code)
	}
}

// Every method reaches the MCP handler's own method check, which is the whole
// reason the contract asks for a bare path. A "POST /mcp" registration would
// give GET a 405 from the mux with no Allow header, or a 404.
func TestMCPIsReachableWithEveryMethodSoItCanAnswer405Itself(t *testing.T) {
	h := mountTestServer(t, mcpTestToken)

	for _, method := range []string{http.MethodGet, http.MethodDelete, http.MethodPut} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, "/mcp", nil))

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /mcp = %d, want 405 from the MCP handler itself", method, rec.Code)
		}
		if got := rec.Header().Get("Allow"); got != http.MethodPost {
			t.Errorf("%s /mcp Allow = %q, want POST", method, got)
		}
	}

	// POST with the token gets through to the JSON-RPC layer.
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	req.Header.Set("Authorization", "Bearer "+mcpTestToken)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /mcp = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"result"`) {
		t.Errorf("POST /mcp did not reach the JSON-RPC layer: %s", rec.Body.String())
	}
}

// No token means the route is never mounted and /mcp is an unknown path like
// any other. This is the rule that differs from the surrounding code: the
// gateway serves /v1/* open when no keys are configured, and that idiom must
// not be carried onto /mcp.
func TestMCPWithoutATokenIsNotMountedAtAll(t *testing.T) {
	h := mountTestServer(t, "")

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("an MCP endpoint with no configured token answered a call: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"result"`) {
		t.Errorf("/mcp answered JSON-RPC with no token configured: %s", rec.Body.String())
	}
}
