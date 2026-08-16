// Package agent — httptool.go: the server-side executor for agent HTTP tools.
//
// The executor is the dangerous half of agent tools: it makes outbound HTTP
// requests that a language model chose the arguments for. Everything here is
// written assuming the model (or the prompt that steered it) can be hostile:
// only http/https, no client credentials forwarded, private-network targets
// refused, redirects capped and re-validated, time and response size bounded.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// EnvToolSSRAllow is the escape hatch for the SSRF guard. Tool URLs are meant
// to point at public APIs; a gateway that legitimately tools against an
// in-cluster service (its own CRM, an internal search) sets this to "1" and
// takes responsibility for what agent YAML it trusts.
const EnvToolSSRAllow = "NABUGATE_TOOL_SSRF_ALLOW"

// maxToolRedirects caps how many redirects one tool call follows. More than
// this is almost always a misconfiguration or a loop, and every hop is
// re-checked against the SSRF guard anyway.
const maxToolRedirects = 3

// ToolExecutor runs agent-declared HTTP tools. The zero-value policy refuses
// private-network targets; NewToolExecutor reads EnvToolSSRAllow.
type ToolExecutor struct {
	// allowPrivate permits requests to loopback/RFC1918/link-local addresses.
	// Off by default; tests and in-cluster deployments turn it on explicitly.
	allowPrivate bool
}

// NewToolExecutor builds an executor honouring the environment. Set
// NABUGATE_TOOL_SSRF_ALLOW=1 to permit private-network tool URLs.
func NewToolExecutor() *ToolExecutor {
	return &ToolExecutor{allowPrivate: os.Getenv(EnvToolSSRAllow) == "1"}
}

// NewToolExecutorForTest builds an executor with an explicit SSRF policy.
func NewToolExecutorForTest(allowPrivate bool) *ToolExecutor {
	return &ToolExecutor{allowPrivate: allowPrivate}
}

// argTemplate matches "{{arg}}" placeholders in headers and body templates.
// Whitespace inside the braces is tolerated: "{{ order_id }}".
var argTemplate = regexp.MustCompile(`\{\{\s*[^{}\s]+\s*\}\}`)

// Execute runs one tool call: args is the decoded JSON object the model sent
// as the function arguments. It returns the (possibly truncated) response body
// as the tool result, or an error describing the failure — the tool loop feeds
// either back to the model as the tool message content.
func (e *ToolExecutor) Execute(ctx context.Context, tool Tool, args map[string]any) (string, error) {
	rawURL, err := tool.buildURL(args)
	if err != nil {
		return "", err
	}

	method := strings.ToUpper(strings.TrimSpace(tool.Method))
	if method == "" {
		method = http.MethodGet
	}

	var body io.Reader
	if len(tool.BodyTemplate) > 0 && method != http.MethodGet && method != http.MethodDelete {
		filled := fillTemplate(tool.BodyTemplate, args)
		encoded, err := json.Marshal(filled)
		if err != nil {
			return "", fmt.Errorf("tool %q body template: %w", tool.Name, err)
		}
		body = bytes.NewReader(encoded)
	}

	timeout := time.Duration(tool.timeout()) * time.Millisecond
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// The SSRF guard runs at dial time: it resolves the hostname itself and
	// refuses the connection when any answer is a non-public address. Because
	// every request and every redirect hop dials through this transport, there
	// is no path — short of the env escape hatch — that reaches a private
	// address. (Resolution and dial are two steps, so a DNS answer could in
	// theory change between them; for a gateway calling declared YAML tools
	// that residual race is accepted, and the env guard stays off by default.)
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(dialCtx context.Context, network, addr string) (net.Conn, error) {
			if err := e.guard(dialCtx, addr); err != nil {
				return nil, err
			}
			return dialer.DialContext(dialCtx, network, addr)
		},
	}
	client := &http.Client{
		Timeout:   timeout + time.Second, // safety net; callCtx is the real bound
		Transport: &toolTransport{base: transport},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxToolRedirects {
				return fmt.Errorf("tool %q: too many redirects (max %d)", tool.Name, maxToolRedirects)
			}
			// The dial guard re-checks the new destination before connecting,
			// so a redirect to an internal address dies at dial time — but the
			// scheme check is cheap enough to do here too.
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("tool %q: redirect to unsupported scheme %q", tool.Name, req.URL.Scheme)
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(callCtx, method, rawURL, body)
	if err != nil {
		return "", fmt.Errorf("tool %q: %w", tool.Name, err)
	}
	for k, v := range tool.Headers {
		req.Header.Set(k, fillString(os.ExpandEnv(v), args))
	}
	// Only the declared headers go out. The caller's Authorization — the
	// gateway key — must never reach a tool endpoint, and because we build the
	// request from scratch, it structurally cannot.
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("tool %q call failed: %w", tool.Name, err)
	}
	defer resp.Body.Close()

	limited, err := io.ReadAll(io.LimitReader(resp.Body, int64(tool.maxResponse())+1))
	if err != nil {
		return "", fmt.Errorf("tool %q read failed: %w", tool.Name, err)
	}
	truncated := len(limited) > tool.maxResponse()
	if truncated {
		limited = limited[:tool.maxResponse()]
	}
	result := string(limited)
	if truncated {
		result += "…[truncated]"
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("tool %q returned HTTP %d: %s", tool.Name, resp.StatusCode, truncateStr(result, 500))
	}
	return result, nil
}

// guard resolves the host being dialled and refuses when any answer is a
// non-public address (loopback, RFC1918, link-local, unspecified, multicast)
// unless the executor was built to allow it.
func (e *ToolExecutor) guard(ctx context.Context, addr string) error {
	if e.allowPrivate {
		return nil
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("tool address %q: %w", addr, err)
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("tool host %q: %w", host, err)
	}
	for _, ipa := range ips {
		ip := ipa.IP
		if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("tool calls to non-public address %s (%s) are blocked (set %s=1 to allow)", ip, host, EnvToolSSRAllow)
		}
	}
	return nil
}

// toolTransport identifies the gateway on tool calls.
type toolTransport struct {
	base *http.Transport
}

func (t *toolTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", "NabuGate/1.0 (agent-tool)")
	}
	return t.base.RoundTrip(req)
}

// buildURL renders the tool URL: env expansion, path-param substitution, then
// any argument not consumed by the path or the body template appended as a
// query parameter — so a GET tool's declared arguments always reach the wire.
func (t Tool) buildURL(args map[string]any) (string, error) {
	raw := os.ExpandEnv(t.URL)

	consumed := map[string]bool{}
	for _, name := range t.PathParams {
		v, ok := args[name]
		if !ok {
			return "", fmt.Errorf("tool %q: missing path parameter %q", t.Name, name)
		}
		raw = strings.ReplaceAll(raw, "{"+name+"}", url.PathEscape(argString(v)))
		consumed[name] = true
	}
	if strings.Contains(raw, "{") && strings.Contains(raw, "}") {
		// A leftover placeholder is almost always a path_params declaration
		// forgotten in YAML; say so rather than sending braces upstream.
		return "", fmt.Errorf("tool %q: url %q still has an unfilled placeholder (declare it in path_params)", t.Name, t.URL)
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("tool %q: bad url: %w", t.Name, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("tool %q: url scheme %q must be http or https", t.Name, u.Scheme)
	}

	bodyKeys := map[string]bool{}
	collectTemplateArgs(t.BodyTemplate, bodyKeys)

	q := u.Query()
	for name, v := range args {
		if consumed[name] || bodyKeys[name] {
			continue
		}
		q.Set(name, argString(v))
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// argString renders a call argument as text for URLs and header templates.
func argString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprint(x)
		}
		return string(b)
	}
}

// fillString replaces "{{arg}}" placeholders in one string.
func fillString(s string, args map[string]any) string {
	return argTemplate.ReplaceAllStringFunc(s, func(m string) string {
		name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(m, "{{"), "}}"))
		if v, ok := args[name]; ok {
			return argString(v)
		}
		return m
	})
}

// fillTemplate deep-copies a body template, replacing placeholders. A value
// that is exactly "{{arg}}" takes the argument's own JSON type; placeholders
// inside a longer string are rendered as text.
func fillTemplate(tpl map[string]any, args map[string]any) map[string]any {
	out := make(map[string]any, len(tpl))
	for k, v := range tpl {
		out[k] = fillValue(v, args)
	}
	return out
}

func fillValue(v any, args map[string]any) any {
	switch x := v.(type) {
	case string:
		if name, exact := exactPlaceholder(x); exact {
			if val, ok := args[name]; ok {
				return val
			}
			return nil
		}
		return fillString(x, args)
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, vv := range x {
			out[k] = fillValue(vv, args)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, vv := range x {
			out[i] = fillValue(vv, args)
		}
		return out
	default:
		return v
	}
}

// exactPlaceholder reports whether s is exactly one "{{arg}}" and its name.
func exactPlaceholder(s string) (string, bool) {
	if !strings.HasPrefix(s, "{{") || !strings.HasSuffix(s, "}}") {
		return "", false
	}
	name := strings.TrimSpace(s[2 : len(s)-2])
	if name == "" || strings.ContainsAny(name, "{}") {
		return "", false
	}
	return name, true
}

// collectTemplateArgs records argument names referenced anywhere in a body
// template, so buildURL does not also send them as query parameters.
func collectTemplateArgs(v any, out map[string]bool) {
	switch x := v.(type) {
	case string:
		for _, m := range argTemplate.FindAllString(x, -1) {
			name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(m, "{{"), "}}"))
			out[name] = true
		}
	case map[string]any:
		for _, vv := range x {
			collectTemplateArgs(vv, out)
		}
	case []any:
		for _, vv := range x {
			collectTemplateArgs(vv, out)
		}
	}
}

// truncateStr shortens a tool error body for the model-facing message.
func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
