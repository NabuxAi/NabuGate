// Package mcp serves the Model Context Protocol over this service's own HTTP
// server, on its own path, behind its own token.
//
// It is deliberately a separate package from `server`: `server` already owns
// unexported helpers named writeJSON/writeError, and a second copy of them in
// the same package would not compile. Nothing here imports `server`, and
// nothing here reuses `server`'s helpers — this package carries its own.
//
// The transport is streamable HTTP, stateless: one JSON-RPC request per POST,
// one JSON response back. No SSE stream, no session id, no server-initiated
// messages. That is the smallest thing an MCP client will talk to, and it needs
// nothing beyond net/http and encoding/json.
package mcp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
)

// ProtocolVersion is the MCP revision these servers speak. Pinned rather than
// echoed back from the client so all four services answer identically.
const ProtocolVersion = "2024-11-05"

// maxRequestBytes caps a single JSON-RPC body. MCP calls are small; anything
// larger is a mistake or an attack.
const maxRequestBytes = 1 << 20 // 1 MiB

// JSON-RPC 2.0 error codes. Only these five are ever used.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// rpcRequest is one JSON-RPC message.
//
// ID is json.RawMessage because JSON-RPC allows a string or a number and the
// answer has to carry back the exact token the client sent. A missing ID means
// the message is a notification.
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// contentBlock is one typed block in a tools/call result. Only "text" is ever
// produced: a tool's payload is marshalled to JSON and carried as that string.
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// callResult is the tools/call envelope. IsError marks a failure the model can
// act on; a failure only a developer can fix is an rpcError instead.
type callResult struct {
	Content []contentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// toolDescriptor is one entry in tools/list.
type toolDescriptor struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Tool is one callable this service exposes.
//
// Handler returns the value to serialise, or an error. A *ToolFailure (build
// one with Failf) becomes an isError result; anything else becomes a -32603 and
// its text is replaced with a fixed string, because an arbitrary error from a
// provider client or a database driver is exactly where a credential leaks.
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Handler     func(ctx context.Context, args map[string]any) (any, error)
}

// ToolFailure is a failure the caller can do something about: no such id, a
// filter that matched nothing, an upstream that is down.
type ToolFailure struct{ Message string }

func (e *ToolFailure) Error() string { return e.Message }

// Failf builds a ToolFailure. Its message is shown to the model verbatim, so it
// must be written for that audience and must never interpolate anything read
// from config, an env var, or an upstream response body.
func Failf(format string, a ...any) error {
	return &ToolFailure{Message: fmt.Sprintf(format, a...)}
}

// Server is the MCP endpoint: a name, a token, and a set of tools.
type Server struct {
	name    string
	version string
	path    string
	token   string
	log     *slog.Logger

	tools map[string]Tool
	names []string // sorted, so tools/list is deterministic
}

// New builds a Server. token is the value of the environment variable named by
// `mcp.token_env`; an empty token leaves the server disabled and the route
// unmounted. There is no open/dev mode: this endpoint reads the same data the
// service's own admin surfaces read, and an unauthenticated one on any of these
// four services would hand over the whole estate.
func New(name, version, path, token string, log *slog.Logger) *Server {
	if path == "" {
		path = "/mcp"
	}

	return &Server{
		name:    name,
		version: version,
		path:    path,
		token:   token,
		log:     log,
		tools:   make(map[string]Tool),
	}
}

// Enabled reports whether a token was configured. Handler() must not be mounted
// when this is false.
func (s *Server) Enabled() bool { return s.token != "" }

// Path is where Handler() belongs on the mux.
func (s *Server) Path() string { return s.path }

// Register adds a tool. A duplicate name replaces the earlier one, and the
// order tools/list reports is always alphabetical.
func (s *Server) Register(t Tool) {
	if _, exists := s.tools[t.Name]; !exists {
		s.names = append(s.names, t.Name)
		sort.Strings(s.names)
	}

	s.tools[t.Name] = t
}

// Handler serves the whole endpoint: method check, token check, JSON-RPC.
//
// Mounted with a bare path (`mux.Handle(m.Path(), m.Handler())`) rather than
// "POST /mcp", so a client that opens the SSE stream with GET or tears a
// session down with DELETE gets a truthful 405 instead of the mux's 404.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeError(w, http.StatusMethodNotAllowed,
				"this endpoint speaks stateless streamable HTTP; POST one JSON-RPC request per call")

			return
		}

		// Checked before the body is read, so an unauthenticated caller cannot
		// make the service allocate.
		if !s.authorised(r) {
			writeError(w, http.StatusUnauthorized, "invalid or missing MCP token")

			return
		}

		var req rpcRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes)).Decode(&req); err != nil {
			writeJSON(w, http.StatusOK, rpcResponse{
				JSONRPC: "2.0",
				Error:   &rpcError{Code: CodeParseError, Message: "body is not valid JSON-RPC"},
			})

			return
		}

		// A notification has no id and takes no answer. Echoing a result with a
		// null id is the single most common way a hand-rolled MCP server breaks
		// a client, so it is spelled out here: 202, empty body.
		if len(req.ID) == 0 {
			w.WriteHeader(http.StatusAccepted)

			return
		}

		writeJSON(w, http.StatusOK, s.dispatch(r.Context(), req))
	})
}

// authorised compares the bearer token in constant time.
//
// Authorization: Bearer only. No X-Api-Key fallback and no query parameter: a
// second accepted place for the token is a second place it leaks from.
func (s *Server) authorised(r *http.Request) bool {
	if s.token == "" {
		return false
	}

	presented := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))

	return subtle.ConstantTimeCompare([]byte(presented), []byte(s.token)) == 1
}

// dispatch runs one JSON-RPC method.
func (s *Server) dispatch(ctx context.Context, req rpcRequest) rpcResponse {
	out := rpcResponse{JSONRPC: "2.0", ID: req.ID}

	switch req.Method {
	case "initialize":
		out.Result = map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": s.name, "version": s.version},
		}

	case "ping":
		out.Result = map[string]any{}

	case "tools/list":
		list := make([]toolDescriptor, 0, len(s.names))

		for _, name := range s.names {
			t := s.tools[name]

			schema := t.InputSchema
			if schema == nil {
				schema = ObjectSchema(nil, nil)
			}

			list = append(list, toolDescriptor{Name: t.Name, Description: t.Description, InputSchema: schema})
		}

		out.Result = map[string]any{"tools": list}

	case "tools/call":
		result, rpcErr := s.call(ctx, req.Params)
		if rpcErr != nil {
			out.Error = rpcErr

			return out
		}

		out.Result = result

	default:
		out.Error = &rpcError{Code: CodeMethodNotFound, Message: "unknown method: " + req.Method}
	}

	return out
}

// call runs one tool.
//
// The split between the two failure kinds is the rule to hold onto: a name that
// does not exist or params that will not decode are a -32601/-32602, because
// only whoever wrote the client can fix them; everything a model could react to
// — no such message, nothing matched, upstream unreachable — comes back as a
// normal result with isError set.
func (s *Server) call(ctx context.Context, params json.RawMessage) (*callResult, *rpcError) {
	var call struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}

	if err := json.Unmarshal(params, &call); err != nil {
		return nil, &rpcError{Code: CodeInvalidParams, Message: "params must be {\"name\":…,\"arguments\":{…}}"}
	}

	tool, ok := s.tools[call.Name]
	if !ok {
		return nil, &rpcError{Code: CodeMethodNotFound, Message: "unknown tool: " + call.Name}
	}

	// A client may omit "arguments" entirely for a no-argument tool; the helpers
	// below read from a nil map safely, but handing every tool a non-nil map
	// removes the question.
	if call.Arguments == nil {
		call.Arguments = map[string]any{}
	}

	value, err := tool.Handler(ctx, call.Arguments)
	if err != nil {
		var failure *ToolFailure
		if errors.As(err, &failure) {
			return errorResult(failure.Message), nil
		}

		// Anything else is unexpected: the detail goes to the log, where an
		// operator can read it, and never into the response, where a model and
		// whoever it is talking to can.
		s.log.Error("mcp tool failed", "tool", call.Name, "error", err)

		return errorResult("this tool failed; the reason is in the service log"), nil
	}

	payload, err := json.Marshal(value)
	if err != nil {
		s.log.Error("mcp tool result is not serialisable", "tool", call.Name, "error", err)

		return errorResult("this tool produced a result that could not be serialised"), nil
	}

	// One text block holding the JSON. The payload is a string, not an object:
	// content blocks are typed, and "text" is the only type these servers emit.
	// No structuredContent field — four services agreeing on one shape matters
	// more than the extra field.
	return &callResult{Content: []contentBlock{{Type: "text", Text: string(payload)}}}, nil
}

func errorResult(message string) *callResult {
	return &callResult{Content: []contentBlock{{Type: "text", Text: message}}, IsError: true}
}

// ObjectSchema builds the JSON Schema for a tool's arguments. properties maps a
// name to its schema fragment; required names the ones that must be present.
func ObjectSchema(properties map[string]any, required []string) map[string]any {
	if properties == nil {
		properties = map[string]any{}
	}

	schema := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}

// StringProp / IntProp build one property fragment each.
func StringProp(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func IntProp(description string, minimum, maximum int) map[string]any {
	return map[string]any{
		"type":        "integer",
		"description": description,
		"minimum":     minimum,
		"maximum":     maximum,
	}
}

// String reads a required string argument.
func String(args map[string]any, name string) (string, error) {
	value, ok := args[name].(string)
	if !ok || value == "" {
		return "", Failf("%q is required and must be a string", name)
	}

	return value, nil
}

// OptString reads an optional string argument.
func OptString(args map[string]any, name, fallback string) string {
	if value, ok := args[name].(string); ok && value != "" {
		return value
	}

	return fallback
}

// Int reads an optional integer argument and clamps it into [min, max].
//
// JSON numbers decode into float64, which is why this is not a type assertion
// to int — a caller sending 20 would otherwise be told 20 is not a number.
func Int(args map[string]any, name string, fallback, minimum, maximum int) int {
	value := fallback

	if raw, ok := args[name].(float64); ok {
		value = int(raw)
	}

	if value < minimum {
		return minimum
	}

	if value > maximum {
		return maximum
	}

	return value
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"message": message}})
}
