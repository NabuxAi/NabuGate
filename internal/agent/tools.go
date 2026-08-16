package agent

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
)

// Tool-type and limit constants for agent-declared tools.
//
// An agent's tools are declared in its YAML file and executed by the gateway
// itself, server-side, inside a bounded tool-call loop: the model asks, the
// gateway runs the call and hands the result back, the model answers. The
// caller never sees the wire traffic — and never has to host the tool.
const (
	// ToolTypeHTTP is the only tool type so far: a declared HTTP endpoint the
	// gateway calls on the model's behalf (inspired by NabuChat's HTTP tool).
	ToolTypeHTTP = "http"

	// DefaultMaxToolSteps bounds the tool-call loop when the agent does not
	// say otherwise. Each step is one provider round-trip plus the tool calls
	// it asked for, so this is also the bound on how much one request can
	// spend before the model is made to answer with what it has.
	DefaultMaxToolSteps = 4
	// MaxToolStepsCap refuses absurd declarations. A loop deeper than this is
	// a runaway prompt bill, not an agent design.
	MaxToolStepsCap = 8

	// DefaultToolTimeoutMS is the per-call HTTP timeout when unset.
	DefaultToolTimeoutMS = 8000
	// MaxToolTimeoutMS caps a declared timeout. A tool slower than this makes
	// the whole chat request look hung; the model does better with a fast
	// failure it can talk about than a long silence.
	MaxToolTimeoutMS = 15000

	// DefaultMaxResponseBytes truncates a tool's response body when unset, so
	// one verbose endpoint cannot blow up the prompt.
	DefaultMaxResponseBytes = 8192
	// MaxResponseBytesCap is the most a tool may hand back in one call.
	MaxResponseBytesCap = 65536
)

// Tool is one function an agent offers its model, executed server-side by the
// gateway. `parameters` is the JSON-schema signature shown to the model; the
// rest is how the gateway turns one call into an HTTP request.
type Tool struct {
	// Name is the function name the model calls. Must be unique per agent.
	Name string
	// Type is the executor kind; only ToolTypeHTTP exists today.
	Type string
	// Description tells the model when to reach for the tool.
	Description string

	// Method is the HTTP verb (GET by default).
	Method string
	// URL is the endpoint. "{name}" placeholders are filled from the call
	// arguments listed in PathParams; "${VAR}" references are expanded from
	// the gateway environment (at agent-file load, and again at call time).
	URL string
	// Headers are sent on the tool request. Values may reference call
	// arguments as "{{arg}}" and env vars as "${VAR}". The caller's own
	// Authorization header is never forwarded — these headers are the only
	// credentials a tool call carries.
	Headers map[string]string
	// PathParams lists argument names substituted into "{name}" placeholders
	// in URL (URL-escaped). Any argument that is neither a path param nor
	// used by BodyTemplate is appended as a query parameter.
	PathParams []string
	// BodyTemplate, for body-carrying methods, is marshalled to JSON after
	// every string value has had "{{arg}}" placeholders replaced by the call
	// arguments. A value that is exactly "{{arg}}" keeps the argument's own
	// JSON type (a number stays a number); inside a longer string the
	// argument is rendered as text.
	BodyTemplate map[string]any
	// Parameters is the JSON schema ({"type":"object","properties":…}) that
	// forms the function signature shown to the model.
	Parameters json.RawMessage

	// TimeoutMS bounds one tool call (default DefaultToolTimeoutMS, capped at
	// MaxToolTimeoutMS).
	TimeoutMS int
	// MaxResponseBytes truncates the tool result handed back to the model
	// (default DefaultMaxResponseBytes, capped at MaxResponseBytesCap).
	MaxResponseBytes int
}

// toolNamePattern is the shape providers expect for function names.
var toolNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// Validate checks a tool declaration at registration time, so a broken tool
// fails loudly at startup (the agent is skipped with a warning) instead of
// surprising a caller mid-conversation.
func (t Tool) Validate() error {
	if !toolNamePattern.MatchString(t.Name) {
		return fmt.Errorf("tool name %q must match %s", t.Name, toolNamePattern.String())
	}
	if t.Type != ToolTypeHTTP {
		return fmt.Errorf("tool %q has unknown type %q (only %q is supported)", t.Name, t.Type, ToolTypeHTTP)
	}
	u, err := url.Parse(os.ExpandEnv(t.URL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("tool %q has an invalid url %q", t.Name, t.URL)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("tool %q url %q must be http or https", t.Name, t.URL)
	}
	switch strings.ToUpper(t.Method) {
	case "", "GET", "POST", "PUT", "PATCH", "DELETE":
	default:
		return fmt.Errorf("tool %q has unsupported method %q", t.Name, t.Method)
	}
	if len(t.Parameters) == 0 || string(t.Parameters) == "null" {
		return fmt.Errorf("tool %q has no parameters schema (the function signature the model sees)", t.Name)
	}
	var schema struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(t.Parameters, &schema); err != nil {
		return fmt.Errorf("tool %q has an invalid parameters schema: %w", t.Name, err)
	}
	if schema.Type != "object" {
		return fmt.Errorf("tool %q parameters must be a JSON schema of type \"object\"", t.Name)
	}
	if t.TimeoutMS < 0 || t.TimeoutMS > MaxToolTimeoutMS {
		return fmt.Errorf("tool %q timeout_ms %d is outside 0..%d", t.Name, t.TimeoutMS, MaxToolTimeoutMS)
	}
	if t.MaxResponseBytes < 0 || t.MaxResponseBytes > MaxResponseBytesCap {
		return fmt.Errorf("tool %q max_response_bytes %d is outside 0..%d", t.Name, t.MaxResponseBytes, MaxResponseBytesCap)
	}
	return nil
}

// timeout returns the effective per-call timeout in milliseconds.
func (t Tool) timeout() int {
	if t.TimeoutMS <= 0 {
		return DefaultToolTimeoutMS
	}
	return t.TimeoutMS
}

// maxResponse returns the effective response truncation limit in bytes.
func (t Tool) maxResponse() int {
	if t.MaxResponseBytes <= 0 {
		return DefaultMaxResponseBytes
	}
	return t.MaxResponseBytes
}

// openAI returns the tool in the OpenAI wire shape ("type": "function"), ready
// to be placed in a chat request's "tools" array.
func (t Tool) openAI() map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"parameters":  t.Parameters,
		},
	}
}

// OpenAITools renders the agent's tools as one OpenAI "tools" array.
func OpenAITools(tools []Tool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		out = append(out, t.openAI())
	}
	return out
}

// ToolNames returns the declared tool names, for listing endpoints.
func ToolNames(tools []Tool) []string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		out = append(out, t.Name)
	}
	return out
}

// validateTools checks a whole declaration: each tool valid, names unique.
func validateTools(tools []Tool) error {
	seen := make(map[string]bool, len(tools))
	for _, t := range tools {
		if err := t.Validate(); err != nil {
			return err
		}
		if seen[t.Name] {
			return fmt.Errorf("duplicate tool %q", t.Name)
		}
		seen[t.Name] = true
	}
	return nil
}

// MaxSteps returns the agent's tool-loop depth with the default applied.
func (a Agent) MaxSteps() int {
	if a.MaxToolSteps <= 0 {
		return DefaultMaxToolSteps
	}
	return a.MaxToolSteps
}
