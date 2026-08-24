// Package server exposes the OpenAI-compatible HTTP API that projects call.
package server

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nabugate/internal/adminstore"
	"nabugate/internal/agent"
	"nabugate/internal/flow"
	"nabugate/internal/memory"
	"nabugate/internal/photos"
	"nabugate/internal/policy"
	"nabugate/internal/provider"
	"nabugate/internal/router"
	"nabugate/internal/usage"
	"nabugate/web"
)

type policyCtxKey struct{}

// maxRequestBytes caps the size of a request body the gateway will read, so a
// single oversized (or slow-trickle) upload can't exhaust memory. Generous
// enough for long chat histories and large embedding batches.
const maxRequestBytes = 16 << 20 // 16 MiB

// Server wires the router, auth, policy, usage tracking and sub-agents into an
// http.Handler.
type Server struct {
	router *router.Router
	policy *policy.Enforcer
	usage  *usage.Tracker
	agents *agent.Registry
	flows  *flow.Registry // nil = no flows configured
	photos *photos.Client // nil = photo proxy disabled
	log    *slog.Logger

	// admin is the persisted console state: accounts, console-minted tokens and
	// usage that survives a restart. nil when no state path is configured, in
	// which case the console API is not mounted and the gateway behaves exactly
	// as it did before.
	admin *adminstore.Store

	// memory is the conversation store. nil disables the feature entirely and
	// every request behaves exactly as it did before.
	memory *memory.Store

	// toolExec runs agent-declared HTTP tools inside the chat tool-call loop
	// (see tools.go). Built from the environment in New.
	toolExec *agent.ToolExecutor

	// logins rate-limits console sign-in attempts. See throttle.go.
	logins *throttle
}

// SetMemory attaches the conversation store.
func (s *Server) SetMemory(m *memory.Store) { s.memory = m }

// SetAdminStore attaches the console state. Separate from New so existing
// callers and their tests keep compiling.
func (s *Server) SetAdminStore(st *adminstore.Store) {
	s.admin = st
	s.loadManagedAgents()
	s.loadManagedFlows()
}

// loadManagedAgents registers the console-created sub-agents into the live
// registry (upsert, so a console agent wins over a same-named baked one). Called
// when the admin store is attached and again after every console mutation, so a
// newly created or edited agent is immediately callable without a restart.
func (s *Server) loadManagedAgents() {
	if s.admin == nil || s.agents == nil {
		return
	}
	for _, rec := range s.admin.Agents() {
		_ = s.agents.Set(agent.Agent{
			Name:        rec.Name,
			Description: rec.Description,
			Model:       rec.Model,
			System:      rec.System,
			Temperature: rec.Temperature,
			MaxTokens:   rec.MaxTokens,
		})
	}
}

// New builds a Server. If the enforcer has no keys, authentication is disabled
// (dev mode) and a warning is logged by the caller. agents may be nil or empty
// when no sub-agents are configured.
func New(r *router.Router, enforcer *policy.Enforcer, tracker *usage.Tracker, agents *agent.Registry, log *slog.Logger) *Server {
	return &Server{router: r, policy: enforcer, usage: tracker, agents: agents, log: log, logins: newThrottle(), toolExec: agent.NewToolExecutor()}
}

// WithToolExecutor overrides the agent-tool executor — tests use it to permit
// loopback tool URLs without setting the process-wide env. Separate from New
// for the same reason as the other attachments.
func (s *Server) WithToolExecutor(e *agent.ToolExecutor) *Server {
	s.toolExec = e
	return s
}

// WithFlows attaches the flow registry. Separate from New for the same reason
// SetAdminStore is: existing callers and their tests keep compiling, and a
// deployment with no flows behaves exactly as it did before.
func (s *Server) WithFlows(f *flow.Registry) *Server {
	s.flows = f
	// The admin store may already be attached, in which case the flows it holds
	// have nowhere to have been registered yet.
	s.loadManagedFlows()
	return s
}

// WithPhotos enables the stock-photo proxy (GET /v1/photos/search). A nil
// client leaves the endpoint responding 503.
func (s *Server) WithPhotos(c *photos.Client) *Server {
	s.photos = c
	return s
}

// Handler returns the root http.Handler with routes and middleware applied.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /v1/models", s.auth(s.handleModels))
	mux.HandleFunc("GET /v1/agents", s.auth(s.handleAgents))
	mux.HandleFunc("GET /v1/health", s.auth(s.handleKeyHealth))
	mux.HandleFunc("POST /v1/chat/completions", s.auth(s.handleChat))
	mux.HandleFunc("POST /v1/responses", s.auth(s.handleResponses))
	mux.HandleFunc("POST /v1/images/generations", s.auth(s.handleImages))
	mux.HandleFunc("POST /v1/audio/speech", s.auth(s.handleSpeech))
	mux.HandleFunc("POST /v1/audio/transcriptions", s.auth(s.handleTranscription))
	mux.HandleFunc("POST /v1/embeddings", s.auth(s.handleEmbeddings))
	mux.HandleFunc("GET /v1/usage", s.auth(s.handleUsage))
	mux.HandleFunc("GET /v1/photos/search", s.auth(s.handlePhotoSearch))
	s.mountConsole(mux)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		if assets, ok := web.Assets(); ok {
			http.ServeFileFS(w, r, assets, "landing.html")
		} else {
			http.Error(w, "Not found", 404)
		}
	})
	
	mux.HandleFunc("GET /fa", func(w http.ResponseWriter, r *http.Request) {
		if assets, ok := web.Assets(); ok {
			http.ServeFileFS(w, r, assets, "landing-fa.html")
		} else {
			http.Error(w, "Not found", 404)
		}
	})
	s.mountConsoleAPI(mux)
	s.mountConversationAPI(mux)
	return mux
}

// mountConsole serves the embedded admin console (web/dist) under /admin/ when
// the bundle is built into the binary.
//
// The shell is served without a session because it has to be: it contains the
// login form. Everything it can actually show comes from /admin/api/*, which
// requires one. What the shell must never do is carry a gateway key — it used
// to be described as safe because the data lived behind /v1/*, but that put the
// admin key in a browser and handed the console to anyone who found the URL.
func (s *Server) mountConsole(mux *http.ServeMux) {
	assets, ok := web.Assets()
	if !ok {
		return
	}
	fileServer := http.FileServer(http.FS(assets))
	mux.Handle("GET /admin/", http.StripPrefix("/admin/", spaFileServer(assets, fileServer)))
	// Bare /admin → /admin/ so the SPA's relative asset URLs resolve correctly.
	mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusMovedPermanently)
	})
}

// spaFileServer serves static files from the console bundle and falls back to
// index.html for paths that don't map to a file, so client-side navigation
// (and a refresh on a deep link) keeps working.
func spaFileServer(assets fs.FS, fileServer http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			fileServer.ServeHTTP(w, r)
			return
		}
		if _, err := fs.Stat(assets, p); err != nil {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

// project returns the calling key's project (or a placeholder in dev mode).
func (s *Server) project(r *http.Request) string {
	if pol, ok := r.Context().Value(policyCtxKey{}).(policy.Policy); ok && pol.Project != "" {
		return pol.Project
	}
	return "(unscoped)"
}

// record attributes a call's usage to the project/model and logs the cost.
// lookupConsoleToken resolves a console-minted token, if the console state is
// attached at all.
func (s *Server) lookupConsoleToken(token string) (adminstore.Token, bool) {
	if s.admin == nil {
		return adminstore.Token{}, false
	}
	return s.admin.Lookup(token)
}

func (s *Server) record(r *http.Request, prov, model string, u provider.Usage) {
	project := s.project(r)
	cost := s.usage.Record(project, prov, model, u)
	// Also accumulate into the persisted counters, so the console's numbers are
	// real across restarts rather than resetting to zero on every redeploy.
	if s.admin != nil {
		s.admin.RecordUsage(project, int64(u.PromptTokens), int64(u.CompletionTokens), cost)
	}
	s.log.Info("billed", "project", project, "provider", prov, "model", model,
		"total_tokens", u.TotalTokens, "cost_usd", cost)
}

// handleUsage reports accumulated usage. Admin keys (not bound to a project,
// i.e. the simple api_keys) see all projects and models; project-scoped keys
// see only their own totals. Admin-ness is based on project scope, not alias
// permissions — an all-alias project key must not see other projects' usage.
func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	pol, hasPol := r.Context().Value(policyCtxKey{}).(policy.Policy)
	// Admin is granted only to the simple full-access api_keys, not to any rich
	// key that merely omits `project:` (which would leak every project's usage).
	admin := !s.policy.Enabled() || (hasPol && pol.Admin)
	if admin {
		byProject, byModel := s.usage.Snapshot()
		writeJSON(w, http.StatusOK, map[string]any{"by_project": byProject, "by_model": byModel})
		return
	}
	project := s.project(r)
	writeJSON(w, http.StatusOK, map[string]any{"project": project, "stats": s.usage.ProjectSnapshot(project)})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleKeyHealth answers, for the calling key, "what can I reach and is any of
// it already broken" — without spending a single upstream request.
//
// /healthz says the process is up and /v1/models says what a key is permitted
// to name. Neither answers the question consuming projects actually end up
// debugging: a key that lists several models, some of which lost their
// provider's API key in this deployment and now fail every request with an
// error written for the gateway's operators rather than for the caller.
//
// Scoped to the caller, because an alias this key may not use is not this
// key's business — and the warnings name providers.
func (s *Server) handleKeyHealth(w http.ResponseWriter, r *http.Request) {
	pol, hasPol := r.Context().Value(policyCtxKey{}).(policy.Policy)
	scoped := s.policy.Enabled() && hasPol

	aliases := make([]router.AliasHealth, 0)
	degraded := 0

	for _, alias := range s.router.AliasHealthAll() {
		if scoped && !pol.Allows(alias.ID) {
			continue
		}
		if len(alias.Warnings) > 0 {
			degraded++
		}
		aliases = append(aliases, alias)
	}

	// Agents and flows are addressable exactly like models, so a key that can
	// reach neither an alias nor an agent can do nothing at all — worth saying
	// plainly rather than leaving as an empty list to interpret.
	agents := allowedNames(s.agents.Names(), pol, scoped)
	flows := allowedNames(s.flows.Names(), pol, scoped)

	status := "ok"
	switch {
	case len(aliases)+len(agents)+len(flows) == 0:
		status = "unusable"
	case degraded > 0:
		status = "degraded"
	}

	body := map[string]any{
		"status":   status,
		"project":  s.project(r),
		"aliases":  aliases,
		"agents":   agents,
		"flows":    flows,
		"degraded": degraded,
	}

	if scoped {
		// Echoed back because the commonest 401 in this gateway's life is a
		// caller naming a model outside its own allow-list, and until now the
		// only place to read that list was the console.
		body["allow"] = pol.Allow
		body["rate_limit"] = pol.RateLimit
	}

	writeJSON(w, http.StatusOK, body)
}

// allowedNames filters agent/flow names down to what this key may address.
func allowedNames(names []string, pol policy.Policy, scoped bool) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		if scoped && !pol.Allows(name) {
			continue
		}
		out = append(out, name)
	}
	return out
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	pol, hasPol := r.Context().Value(policyCtxKey{}).(policy.Policy)
	// Configured aliases plus every passthrough provider's live-discovered and
	// statically configured models (e.g. "parspack/openai/gpt-5.5"), plus the
	// configured sub-agents (addressable as a "model" like anything else).
	infos := s.router.AliasInfos()
	infos = append(infos, s.router.CatalogModels(r.Context())...)
	for _, name := range s.agents.Names() {
		infos = append(infos, router.AliasInfo{ID: name, Owner: "agent"})
	}
	// Flows list too, and for the same reason agents do: a caller decides what
	// to send by reading this, and a chain they cannot discover is one they
	// will never use.
	for _, name := range s.flows.Names() {
		infos = append(infos, router.AliasInfo{ID: name, Owner: "flow"})
	}

	data := make([]map[string]string, 0, len(infos))
	seen := make(map[string]bool, len(infos))
	for _, a := range infos {
		if seen[a.ID] {
			continue
		}
		if s.policy.Enabled() && hasPol && !pol.Allows(a.ID) {
			continue // hide models this key may not use
		}
		if hasPol && len(pol.Providers) > 0 && a.Owner != "agent" && a.Owner != "flow" && !contains(pol.Providers, a.Owner) {
			continue
		}
		seen[a.ID] = true
		data = append(data, map[string]string{"id": a.ID, "object": "model", "owned_by": a.Owner})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

// handleResponses proxies the OpenAI Responses API (POST /v1/responses). Like
// chat, only "model" (a NabuGate alias or a "<provider>/<model>" passthrough)
// is inspected and rewritten to the upstream model; the rest of the body is
// forwarded verbatim, and the upstream response — JSON or streaming SSE — is
// copied straight back. Token usage is not metered here (the gateway does not
// parse the Responses schema); observability comes from the router's logs.
func (s *Server) handleResponses(w http.ResponseWriter, r *http.Request) {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	var model string
	if len(raw["model"]) > 0 {
		_ = json.Unmarshal(raw["model"], &model)
	}
	if model == "" {
		writeError(w, http.StatusBadRequest, "field 'model' (alias) is required")
		return
	}
	if !s.aliasAllowed(w, r, model) {
		return
	}

	// Sub-agent expansion for the Responses API: the agent's system prompt maps
	// to `instructions` and its defaults fill unset params, then we route to the
	// agent's underlying model.
	routeModel := model
	if ag, ok := s.agents.Lookup(model); ok {
		applyAgentToResponses(ag, raw)
		routeModel = ag.Model
		w.Header().Set("X-Nabu-Agent", ag.Name)
	}

	resp, prov, upstream, err := s.router.Responses(r.Context(), routeModel, raw)
	if err != nil {
		writeError(w, aliasErrStatus(err, "unknown model alias"), err.Error())
		return
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/json"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Nabu-Provider", prov)
	w.Header().Set("X-Nabu-Model", upstream)
	if strings.HasPrefix(ct, "text/event-stream") {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
	}
	w.WriteHeader(resp.StatusCode)

	// Stream the upstream body through, flushing so SSE deltas reach the client
	// as they arrive instead of buffering until the response completes.
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 16<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if rerr != nil {
			return
		}
	}
}

// handleChat accepts the full OpenAI-compatible chat body. Only "model" (a
// NabuGate alias) and "messages" are inspected here; the entire body is carried
// through to the upstream provider so any OpenAI parameter — tools, tool_choice,
// response_format, top_p, stop, seed, penalties — passes through untouched.
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var alias string
	if len(raw["model"]) > 0 {
		_ = json.Unmarshal(raw["model"], &alias)
	}
	if alias == "" {
		writeError(w, http.StatusBadRequest, "field 'model' (alias) is required")
		return
	}

	var msgsRaw []json.RawMessage
	if len(raw["messages"]) == 0 || json.Unmarshal(raw["messages"], &msgsRaw) != nil || len(msgsRaw) == 0 {
		writeError(w, http.StatusBadRequest, "field 'messages' must not be empty")
		return
	}
	if !s.aliasAllowed(w, r, alias) {
		return
	}

	// Typed fields are used by the non-OpenAI adapters (Anthropic, Gemini); the
	// OpenAI-wire adapter forwards the raw body directly.
	var temperature, topP *float64
	var maxTokens *int
	var stream bool
	var msgs []provider.Message
	_ = json.Unmarshal(raw["messages"], &msgs)
	if len(raw["temperature"]) > 0 {
		_ = json.Unmarshal(raw["temperature"], &temperature)
	}
	if len(raw["top_p"]) > 0 {
		_ = json.Unmarshal(raw["top_p"], &topP)
	}
	if len(raw["max_tokens"]) > 0 {
		_ = json.Unmarshal(raw["max_tokens"], &maxTokens)
	}
	if len(raw["stream"]) > 0 {
		_ = json.Unmarshal(raw["stream"], &stream)
	}

	chatReq := provider.ChatRequest{
		Messages:    msgs,
		Temperature: temperature,
		TopP:        topP,
		MaxTokens:   maxTokens,
		Stop:        raw["stop"],
		Raw:         raw,
	}

	// Flow expansion. Checked before agents because a flow is the bigger thing
	// and a name is only ever one of them; a flow named after an agent is a
	// definition mistake, not a request the gateway should silently resolve the
	// smaller way.
	//
	// A flow is several provider calls, so it cannot stream: the first token of
	// the answer does not exist until the last step starts. Saying so beats
	// streaming an intermediate step's draft as if it were the answer.
	if fl, ok := s.flows.Lookup(alias); ok {
		if stream {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("flow %q cannot be streamed: its answer is not written until the last step runs", alias))
			return
		}

		s.completeFlow(w, r, fl, chatReq)
		return
	}

	// Sub-agent expansion: if the requested "model" names a configured agent,
	// inject its system prompt and default params and route to its underlying
	// model. The client-facing name (alias) is still echoed back as the model.
	routeModel := alias
	var ag agent.Agent
	isAgent := false
	if found, ok := s.agents.Lookup(alias); ok {
		ag, isAgent = found, true
		applyAgentToChat(ag, &chatReq)
		routeModel = ag.Model
		w.Header().Set("X-Nabu-Agent", ag.Name)
	}

	// Conversation memory. The turns arriving in this request are captured
	// before history is prepended, so only they get stored — replaying and then
	// re-storing would double the conversation on every call.
	convID := conversationID(raw)
	newTurns := append([]provider.Message(nil), chatReq.Messages...)
	if convID != "" {
		if _, err := s.loadHistory(r, convID, &chatReq); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		w.Header().Set("X-Nabu-Conversation", convID)
	}

	// Agent tool calling: an agent that declares tools gets the server-side
	// tool-call loop — unless the caller sent its own tools, in which case the
	// caller's win and the request passes through untouched (see tools.go).
	if isAgent && len(ag.Tools) > 0 && !clientSuppliesTools(raw) {
		s.completeAgentTools(w, r, alias, ag, routeModel, chatReq, convID, newTurns, stream)
		return
	}

	if stream {
		s.streamChat(w, r, alias, routeModel, chatReq, convID, newTurns)
		return
	}

	result, err := s.router.Chat(r.Context(), routeModel, chatReq)
	if err != nil {
		// Unknown alias is a client error; everything else is upstream/bad gateway.
		status := http.StatusBadGateway
		if strings.HasPrefix(err.Error(), "unknown model alias") {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	s.record(r, result.Provider, result.Model, result.Response.Usage)
	s.saveTurn(r, convID, newTurns, result.Response.Content)

	w.Header().Set("X-Nabu-Provider", result.Provider)
	w.Header().Set("X-Nabu-Model", result.Model)

	message := map[string]any{"role": "assistant", "content": result.Response.Content}
	if len(result.Response.ToolCalls) > 0 {
		message["tool_calls"] = result.Response.ToolCalls
	}
	finish := result.Response.FinishReason
	if finish == "" {
		finish = "stop"
	}

	resp := map[string]any{
		"id":             "nabu-" + fmt.Sprint(time.Now().UnixNano()),
		"object":         "chat.completion",
		"created":        time.Now().Unix(),
		"model":          alias, // echoes the requested alias or agent name
		"provider":       result.Provider,
		"upstream_model": result.Model,
		"choices": []map[string]any{{
			"index":         0,
			"finish_reason": finish,
			"message":       message,
		}},
		"usage": map[string]int{
			"prompt_tokens":     result.Response.Usage.PromptTokens,
			"completion_tokens": result.Response.Usage.CompletionTokens,
			"total_tokens":      result.Response.Usage.TotalTokens,
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

// streamChat streams a chat completion as OpenAI-style SSE chunks. alias is the
// client-facing name echoed in each chunk's "model" field (an alias or an agent
// name); routeModel is the underlying model the router resolves and calls (they
// differ when alias is a sub-agent). Response headers (including the chosen
// provider) are written lazily on the first delta so that, if every target fails
// before producing output, we can still return a normal JSON error with the
// right status code.
func (s *Server) streamChat(w http.ResponseWriter, r *http.Request, alias, routeModel string, req provider.ChatRequest, convID string, newTurns []provider.Message) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	id := "nabu-" + fmt.Sprint(time.Now().UnixNano())
	created := time.Now().Unix()
	var metaProvider, metaModel string
	headersWritten := false

	// The assembled reply, so the conversation store gets what the user got.
	var replyBuf strings.Builder

	writeSSE := func(v any) {
		payload, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", payload)
		flusher.Flush()
	}
	startHeaders := func() {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.Header().Set("X-Nabu-Provider", metaProvider)
		w.Header().Set("X-Nabu-Model", metaModel)
		w.WriteHeader(http.StatusOK)
		headersWritten = true
		writeSSE(streamChunk(id, created, alias, metaProvider, metaModel, map[string]any{"role": "assistant"}, nil))
	}

	result, err := s.router.ChatStream(r.Context(), routeModel, req,
		func(p, m string) { metaProvider, metaModel = p, m },
		func(delta string) error {
			if !headersWritten {
				startHeaders()
			}
			replyBuf.WriteString(delta)
			writeSSE(streamChunk(id, created, alias, metaProvider, metaModel, map[string]any{"content": delta}, nil))
			return nil
		},
	)

	if !headersWritten {
		if err != nil {
			writeError(w, aliasErrStatus(err, "unknown model alias"), err.Error())
			return
		}
		startHeaders() // succeeded but produced no text; emit an empty stream
	}

	// Bill only successful generations, matching the non-streaming path. A
	// mid-stream failure leaves result.Usage zero-valued and must not inflate
	// per-project request/usage counters.
	if err == nil {
		s.record(r, result.Provider, result.Model, result.Usage)
		// Store only a stream that produced something. A failed or empty
		// generation is not a turn, and persisting it would replay an empty
		// assistant message into every later call.
		s.saveTurn(r, convID, newTurns, replyBuf.String())
	}

	finish := "stop"
	if err != nil {
		finish = "error"
	}
	writeSSE(streamChunk(id, created, alias, result.Provider, result.Model, map[string]any{}, &finish))
	fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// applyAgentToChat layers a sub-agent's system prompt and default sampling
// params onto a chat request. The system prompt is prepended as the first
// message so it takes effect ahead of the caller's own messages, and defaults
// fill only parameters the caller left unset. Both the typed request (used by
// the Anthropic/Gemini adapters) and the raw body (forwarded verbatim by the
// OpenAI-wire adapters) are updated, so the agent applies on whichever provider
// the router picks.
func applyAgentToChat(ag agent.Agent, req *provider.ChatRequest) {
	if sys := strings.TrimSpace(ag.System); sys != "" {
		sysMsg := provider.Message{Role: "system", Content: ag.System}
		req.Messages = append([]provider.Message{sysMsg}, req.Messages...)
		if req.Raw != nil {
			var msgs []json.RawMessage
			_ = json.Unmarshal(req.Raw["messages"], &msgs)
			sysRaw, _ := json.Marshal(sysMsg)
			req.Raw["messages"], _ = json.Marshal(append([]json.RawMessage{sysRaw}, msgs...))
		}
	}
	if ag.Temperature != nil && req.Temperature == nil {
		req.Temperature = ag.Temperature
		setRawParam(req.Raw, "temperature", *ag.Temperature)
	}
	if ag.TopP != nil && req.TopP == nil {
		req.TopP = ag.TopP
		setRawParam(req.Raw, "top_p", *ag.TopP)
	}
	if ag.MaxTokens != nil && req.MaxTokens == nil {
		req.MaxTokens = ag.MaxTokens
		setRawParam(req.Raw, "max_tokens", *ag.MaxTokens)
	}
}

// applyAgentToResponses layers a sub-agent onto an OpenAI Responses API body.
// The system prompt maps to `instructions` and defaults fill unset params; a
// value the caller already supplied is never overwritten.
func applyAgentToResponses(ag agent.Agent, raw map[string]json.RawMessage) {
	if raw == nil {
		return
	}
	if sys := strings.TrimSpace(ag.System); sys != "" {
		if _, has := raw["instructions"]; !has {
			raw["instructions"], _ = json.Marshal(ag.System)
		}
	}
	if ag.Temperature != nil {
		setRawParam(raw, "temperature", *ag.Temperature)
	}
	if ag.TopP != nil {
		setRawParam(raw, "top_p", *ag.TopP)
	}
}

// setRawParam writes v under key in the raw body only if the caller did not
// already set that key, so an explicit request value always wins over an agent
// default.
func setRawParam(raw map[string]json.RawMessage, key string, v any) {
	if raw == nil {
		return
	}
	if _, exists := raw[key]; exists {
		return
	}
	if b, err := json.Marshal(v); err == nil {
		raw[key] = b
	}
}

// streamChunk builds one OpenAI-style chat.completion.chunk object.
func streamChunk(id string, created int64, alias, prov, model string, delta map[string]any, finish *string) map[string]any {
	return map[string]any{
		"id":             id,
		"object":         "chat.completion.chunk",
		"created":        created,
		"model":          alias,
		"provider":       prov,
		"upstream_model": model,
		"choices": []map[string]any{{
			"index":         0,
			"delta":         delta,
			"finish_reason": finish,
		}},
	}
}

// imageRequestBody is the OpenAI-compatible image request. "model" is an alias.
type imageRequestBody struct {
	Model       string `json:"model"`
	Prompt      string `json:"prompt"`
	N           int    `json:"n"`
	Size        string `json:"size"`
	AspectRatio string `json:"aspect_ratio"`
}

func (s *Server) handleImages(w http.ResponseWriter, r *http.Request) {
	var body imageRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Model == "" || body.Prompt == "" {
		writeError(w, http.StatusBadRequest, "fields 'model' (alias) and 'prompt' are required")
		return
	}
	if !s.aliasAllowed(w, r, body.Model) {
		return
	}

	result, err := s.router.Image(r.Context(), body.Model, provider.ImageRequest{
		Prompt:      body.Prompt,
		N:           body.N,
		Size:        body.Size,
		AspectRatio: body.AspectRatio,
	})
	if err != nil {
		writeError(w, aliasErrStatus(err, "unknown image alias"), err.Error())
		return
	}

	w.Header().Set("X-Nabu-Provider", result.Provider)
	w.Header().Set("X-Nabu-Model", result.Model)

	data := make([]map[string]string, 0, len(result.Images))
	for _, b64 := range result.Images {
		data = append(data, map[string]string{"b64_json": b64})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"created":        time.Now().Unix(),
		"model":          result.Alias,
		"provider":       result.Provider,
		"upstream_model": result.Model,
		"data":           data,
	})
}

// speechRequestBody is the OpenAI-compatible speech request. "model" is an alias.
type speechRequestBody struct {
	Model          string `json:"model"`
	Input          string `json:"input"`
	Voice          string `json:"voice"`
	ResponseFormat string `json:"response_format"`
}

func (s *Server) handleSpeech(w http.ResponseWriter, r *http.Request) {
	var body speechRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Model == "" || body.Input == "" {
		writeError(w, http.StatusBadRequest, "fields 'model' (alias) and 'input' are required")
		return
	}
	if !s.aliasAllowed(w, r, body.Model) {
		return
	}

	result, err := s.router.Speech(r.Context(), body.Model, provider.SpeechRequest{
		Input:  body.Input,
		Voice:  body.Voice,
		Format: body.ResponseFormat,
	})
	if err != nil {
		writeError(w, aliasErrStatus(err, "unknown audio alias"), err.Error())
		return
	}

	// OpenAI's /v1/audio/speech returns raw audio bytes, so we do too.
	w.Header().Set("X-Nabu-Provider", result.Provider)
	w.Header().Set("X-Nabu-Model", result.Model)
	w.Header().Set("Content-Type", result.ContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(result.Audio)
}

// embeddingRequestBody is the OpenAI-compatible embeddings request. "input"
// may be a single string or an array of strings.
type embeddingRequestBody struct {
	Model      string          `json:"model"`
	Input      json.RawMessage `json:"input"`
	Dimensions *int            `json:"dimensions,omitempty"`
	// EncodingFormat is "float" or "base64", as in OpenAI's API.
	//
	// It has to be honoured rather than ignored, because the official OpenAI
	// SDK asks for base64 by DEFAULT — so most callers send it without ever
	// choosing to. A client that asks for base64 and receives a JSON array
	// decodes that array as packed float32 bytes and ends up with a vector a
	// quarter of the expected length: 384 floats where 1536 were promised.
	// Nothing errors. The vector store then rejects the write, or worse
	// accepts it, and the whole retrieval pipeline is quietly wrong.
	EncodingFormat string `json:"encoding_format,omitempty"`
}

// encodeEmbedding renders one vector in the format the caller asked for.
//
// base64 is little-endian float32, which is what OpenAI returns and therefore
// what every client that decodes base64 expects.
// The gateway carries vectors as float64; OpenAI's base64 encoding is packed
// float32, so the narrowing happens here rather than in the caller. It costs
// precision no embedding model actually provides.
func encodeEmbedding(vec []float64, format string) any {
	if format != "base64" {
		return vec
	}
	buf := make([]byte, 4*len(vec))
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(float32(v)))
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func (s *Server) handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	var body embeddingRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Model == "" {
		writeError(w, http.StatusBadRequest, "field 'model' (alias) is required")
		return
	}

	// Accept both string and []string for "input".
	var inputs []string
	if err := json.Unmarshal(body.Input, &inputs); err != nil {
		var single string
		if err2 := json.Unmarshal(body.Input, &single); err2 != nil {
			writeError(w, http.StatusBadRequest, "field 'input' must be a string or array of strings")
			return
		}
		inputs = []string{single}
	}
	if len(inputs) == 0 {
		writeError(w, http.StatusBadRequest, "field 'input' must not be empty")
		return
	}
	// Refuse a format we cannot produce rather than silently sending floats to
	// someone waiting for base64 — that failure is invisible until a vector
	// store rejects the write, far from here.
	switch body.EncodingFormat {
	case "", "float", "base64":
	default:
		writeError(w, http.StatusBadRequest, "field 'encoding_format' must be 'float' or 'base64'")
		return
	}
	if !s.aliasAllowed(w, r, body.Model) {
		return
	}

	result, err := s.router.Embed(r.Context(), body.Model, provider.EmbeddingRequest{
		Input:      inputs,
		Dimensions: body.Dimensions,
	})
	if err != nil {
		writeError(w, aliasErrStatus(err, "unknown embedding alias"), err.Error())
		return
	}

	s.record(r, result.Provider, result.Model, result.Usage)

	w.Header().Set("X-Nabu-Provider", result.Provider)
	w.Header().Set("X-Nabu-Model", result.Model)

	data := make([]map[string]any, 0, len(result.Embeddings))
	for i, vec := range result.Embeddings {
		data = append(data, map[string]any{
			"object":    "embedding",
			"index":     i,
			"embedding": encodeEmbedding(vec, body.EncodingFormat),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"object":         "list",
		"model":          result.Alias,
		"provider":       result.Provider,
		"upstream_model": result.Model,
		"data":           data,
		"usage": map[string]int{
			"prompt_tokens": result.Usage.PromptTokens,
			"total_tokens":  result.Usage.TotalTokens,
		},
	})
}

// handlePhotoSearch proxies stock-photo search to Pexels. The Pexels API key
// stays inside the gateway; callers authenticate with their normal NabuGate
// key. Disabled (503) when PEXELS_API_KEY is not configured.
func (s *Server) handlePhotoSearch(w http.ResponseWriter, r *http.Request) {
	if s.photos == nil {
		writeError(w, http.StatusServiceUnavailable, "photo proxy is not configured (set PEXELS_API_KEY)")
		return
	}
	q := r.URL.Query()
	// An empty query serves the curated feed (default gallery content).
	query := strings.TrimSpace(q.Get("query"))
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	page, _ := strconv.Atoi(q.Get("page"))

	result, err := s.photos.Search(r.Context(), photos.SearchParams{
		Query:       query,
		Orientation: strings.TrimSpace(q.Get("orientation")),
		Size:        strings.TrimSpace(q.Get("size")),
		PerPage:     perPage,
		Page:        page,
		Locale:      strings.TrimSpace(q.Get("locale")),
	})
	if err != nil {
		s.log.Warn("photo search failed", "project", s.project(r), "query", query, "error", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.log.Info("photos served", "project", s.project(r), "query", query, "results", len(result.Photos))
	writeJSON(w, http.StatusOK, result)
}

// aliasErrStatus maps an unknown-alias error to 400 and everything else to 502.
func aliasErrStatus(err error, unknownPrefix string) int {
	if strings.HasPrefix(err.Error(), unknownPrefix) {
		return http.StatusBadRequest
	}
	return http.StatusBadGateway
}

// auth validates the bearer token, enforces the per-key rate limit, and stores
// the resolved policy in the request context for later alias checks. When no
// keys are configured, requests pass through (dev mode).
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Cap the body before any handler reads it (bounds memory / slow-loris).
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
		if !s.policy.Enabled() {
			next(w, r)
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))

		pol, ok := s.policy.Lookup(token)
		if !ok {
			// Not in the baked config — try the tokens minted from the console.
			// Config keys are checked first so a deployment's declared keys can
			// never be shadowed by something added at runtime.
			if t, found := s.lookupConsoleToken(token); found {
				if !originAllowed(t.AllowedOrigins, r) {
					s.admin.RecordDenied(t.Name)
					s.log.Warn("origin refused", "project", t.Name,
						"origin", requestOriginHost(r), "allowed", t.AllowedOrigins)
					writeError(w, http.StatusForbidden, "this key is not permitted from this origin")
					return
				}
				pol = policy.Policy{Project: t.Name, Allow: t.Allow, RateLimit: t.RateLimit, Providers: t.Providers}
				ok = true
			}
		}
		if !ok {
			if s.admin != nil {
				s.admin.RecordDenied("(unknown)")
			}
			writeError(w, http.StatusUnauthorized, "invalid or missing API key")
			return
		}
		if !s.policy.RateOK(token) {
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		ctx := context.WithValue(r.Context(), policyCtxKey{}, pol)
		ctx = context.WithValue(ctx, router.AllowedProvidersCtxKey{}, pol.Providers)
		next(w, r.WithContext(ctx))
	}
}

// aliasAllowed reports whether the request's key may use the given alias, and
// writes a 403 if not. Returns true when policy is disabled.
func (s *Server) aliasAllowed(w http.ResponseWriter, r *http.Request, alias string) bool {
	if !s.policy.Enabled() {
		return true
	}
	pol, ok := r.Context().Value(policyCtxKey{}).(policy.Policy)
	if ok && pol.Allows(alias) {
		return true
	}
	writeError(w, http.StatusForbidden, fmt.Sprintf("alias %q is not permitted for this key", alias))
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"message": msg},
	})
}

func contains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
