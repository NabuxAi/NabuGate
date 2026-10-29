// Package server exposes the OpenAI-compatible HTTP API that projects call.
package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"nabugate/internal/adminstore"
	"nabugate/internal/agent"
	"nabugate/internal/config"
	"nabugate/internal/flow"
	"nabugate/internal/mcp"
	"nabugate/internal/memory"
	"nabugate/internal/nabupay"
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

	// providers is what the config knows about upstreams that the router does
	// not see — the ones with no key set, and whether spending the gateway's
	// own key on them needs approval. Empty is fine: the catalogue then shows
	// only what came up.
	providers map[string]config.ProviderMeta
	// plans are the subscriptions this deployment sells; empty means it sells
	// none and gated providers stay admin-approval only.
	plans []config.Plan

	// corsOrigins are the browser origins allowed to call the gateway. Empty —
	// the default — sends no CORS headers at all, exactly as before this
	// existed, so upgrading opens nothing.
	corsOrigins []string

	// admin is the persisted console state: accounts, console-minted tokens and
	// usage that survives a restart. nil when no state path is configured, in
	// which case the console API is not mounted and the gateway behaves exactly
	// as it did before.
	admin *adminstore.Store

	// requests is the recent-calls ring the console reads. In memory and
	// bounded — see adminstore.RequestLog for why it is not persisted.
	requests *adminstore.RequestLog

	// live is the registry of realtime voice sessions this gateway signalled
	// and has not yet billed to completion. See live.go.
	live *liveSessions

	// pay starts and confirms real payments through NabuPay. nil when the
	// deployment has no gateway configured, in which case the panel says
	// recharging is unavailable rather than pretending to take money.
	pay *nabupay.Client
	// gateways caches the bridge's method list for the status endpoint.
	gateways gatewayCache
	// reserved is the money held against owners' balances for their
	// requests in flight; see reserve.go.
	reserved reservations

	// payGateway is the bridge gateway slug used when the panel does not name
	// one — the deployment's default way to take money.
	payGateway string

	// memory is the conversation store. nil disables the feature entirely and
	// every request behaves exactly as it did before.
	memory *memory.Store

	// toolExec runs agent-declared HTTP tools inside the chat tool-call loop
	// (see tools.go). Built from the environment in New.
	toolExec *agent.ToolExecutor

	// logins rate-limits console sign-in attempts. See throttle.go.
	logins *throttle

	// mcp is the Model Context Protocol endpoint. nil, or configured without a
	// token, means the route is never mounted — this service holds every
	// provider key in the estate and mints the tokens projects call it with,
	// and an open MCP endpoint on it would be all of them.
	mcp *mcp.Server
}

// SetMemory attaches the conversation store.
func (s *Server) SetMemory(m *memory.Store) { s.memory = m }

// SetAdminStore attaches the console state. Separate from New so existing
// callers and their tests keep compiling.
func (s *Server) SetAdminStore(st *adminstore.Store) {
	s.admin = st
	s.loadManagedAgents()
	s.loadManagedFlows()
	s.applyAliasSettings()
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
	return &Server{router: r, policy: enforcer, usage: tracker, agents: agents, log: log, logins: newThrottle(), toolExec: agent.NewToolExecutor(), requests: adminstore.NewRequestLog(500), live: newLiveSessions()}
}

// WithToolExecutor overrides the agent-tool executor — tests use it to permit
// loopback tool URLs without setting the process-wide env. Separate from New
// for the same reason as the other attachments.
func (s *Server) WithToolExecutor(e *agent.ToolExecutor) *Server {
	s.toolExec = e
	return s
}

// WithMCP attaches the MCP endpoint. Separate from New for the same reason
// every other attachment here is: a deployment without an MCP token behaves
// exactly as it did before, and /mcp simply does not exist.
func (s *Server) WithMCP(m *mcp.Server) *Server {
	s.mcp = m
	return s
}

// Requests exposes the recent-calls ring so main.go can hand it to a reader
// that is not this package — the MCP endpoint's requests tool, which must not
// import internal/server.
//
// Read-only by construction: RequestLog's only reader is Recent, and the only
// writer is this package.
func (s *Server) Requests() *adminstore.RequestLog { return s.requests }

// WithPayments attaches the NabuPay bridge client. Separate from New for the
// same reason the others are: a deployment with no payment gateway configured
// keeps working exactly as it did, and its panel says so.
func (s *Server) WithPayments(c *nabupay.Client, defaultGateway string) *Server {
	s.pay = c
	s.payGateway = defaultGateway
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
	mux.HandleFunc("POST /v1/live/sessions", s.auth(s.handleLiveSession))
	mux.HandleFunc("POST /v1/live/sessions/{id}/usage", s.auth(s.handleLiveUsage))
	mux.HandleFunc("POST /v1/audio/transcriptions", s.auth(s.handleTranscription))
	mux.HandleFunc("POST /v1/embeddings", s.auth(s.handleEmbeddings))
	mux.HandleFunc("POST /v1/systemone", s.auth(s.handleDecisions))
	mux.HandleFunc("POST /v1/decisions", s.auth(s.handleDecisions))
	mux.HandleFunc("POST /v1/decisions/calibrate", s.auth(s.handleDecisionCalibrate))
	mux.HandleFunc("GET /v1/usage", s.auth(s.handleUsage))
	mux.HandleFunc("GET /v1/photos/search", s.auth(s.handlePhotoSearch))

	site := s.webSite()
	if site != nil {
		mux.Handle("GET /", site)
	}

	mux.HandleFunc("GET /api/public/models", func(w http.ResponseWriter, r *http.Request) {
		infos := s.router.AliasInfos()
		infos = append(infos, s.router.CatalogModels(r.Context())...)

		names := make([]string, 0, len(infos))
		for _, info := range infos {
			names = append(names, info.ID)
		}
		writeJSON(w, http.StatusOK, names)
	})

	s.mountConsole(mux, site)
	s.mountConsoleAPI(mux)
	s.mountConversationAPI(mux)

	// CORS wraps everything, including the MCP endpoint: a preflight carries no
	// Authorization header, so it has to be answered before any auth runs.
	return s.cors(s.withMCP(mux))
}

// withMCP puts the MCP endpoint in front of the mux rather than on it.
//
// The contract asks for a BARE path — mux.Handle("/mcp", …) rather than
// "POST /mcp" — so that a client opening an SSE stream with GET, or tearing a
// session down with DELETE, is told the endpoint is POST-only instead of that
// it does not exist. On this mux that registration panics at startup: the
// console bundle is registered as "GET /" and, under Go 1.22 pattern
// precedence, "/mcp" matches more methods while "GET /" matches fewer paths,
// so neither is a subset of the other and net/http rejects the pair. The
// bundle is committed to the repo, so web.Assets() is true in every build —
// this would have been a panic on boot in the distroless image, not a
// test-only surprise.
//
// Intercepting one exact path ahead of the mux gives the contract exactly what
// it asked for: every method reaches the MCP handler's own method check, which
// answers 405 with Allow: POST. The one exception is a browser preflight when
// cors_origins is configured — OPTIONS from a permitted origin is answered by
// the CORS wrapper outside this, which is what a browser needs and what the
// contract's 405 would defeat.
func (s *Server) withMCP(mux *http.ServeMux) http.Handler {
	if s.mcp == nil || !s.mcp.Enabled() {
		return mux
	}

	handler, path := s.mcp.Handler(), s.mcp.Path()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == path {
			handler.ServeHTTP(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// webSite loads the embedded web bundle (web/dist), or returns nil when the
// binary was built without one so the gateway still starts as a bare API.
func (s *Server) webSite() *staticSite {
	assets, ok := web.Assets()
	if !ok {
		return nil
	}
	site, err := newStaticSite(assets)
	if err != nil {
		s.log.Warn("web bundle unreadable; serving the API only", "error", err)
		return nil
	}
	return site
}

// mountConsole serves the embedded admin console (web/dist) under /admin/ and
// /panel/ when the bundle is built into the binary. Unknown sub-paths fall back
// to the app shell (see staticSite), so a refresh on a deep link keeps working.
//
// The shell is served without a session because it has to be: it contains the
// login form. Everything it can actually show comes from /admin/api/*, which
// requires one. What the shell must never do is carry a gateway key — it used
// to be described as safe because the data lived behind /v1/*, but that put the
// admin key in a browser and handed the console to anyone who found the URL.
func (s *Server) mountConsole(mux *http.ServeMux, site *staticSite) {
	if site == nil {
		return
	}
	mux.Handle("GET /admin/", http.StripPrefix("/admin/", site))
	// Bare /admin → /admin/ so the SPA's relative asset URLs resolve correctly.
	mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusMovedPermanently)
	})

	mux.Handle("GET /panel/", http.StripPrefix("/panel/", site))
	mux.HandleFunc("GET /panel", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/panel/", http.StatusMovedPermanently)
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
// isConfigKey reports whether token is one of the deployment's config keys,
// whose rate limit RateOK already applied.
func isConfigKey(s *Server, token string) bool {
	_, ok := s.policy.Lookup(token)
	return ok
}

// authEnforced reports whether requests must carry a key. Open (development)
// mode is a checkout with no keys at all: none in config and none minted in
// the console. It used to follow config alone, so a deployment whose config
// keys were all unset served every caller unauthenticated and unbilled while
// its customers' console keys sat unused.
func (s *Server) authEnforced() bool {
	return s.policy.Enabled() || (s.admin != nil && s.admin.HasTokens())
}

func (s *Server) lookupConsoleToken(token string) (adminstore.Token, bool) {
	if s.admin == nil {
		return adminstore.Token{}, false
	}
	return s.admin.Lookup(token)
}

// record meters one call billed by its tokens and returns what the caller was
// charged for it, so the handler can tell them: a product that bills its own
// tenants per request (NabuCRM's per-business AI credit) needs the figure the
// gateway settled on, not a guess from the token counts at a price list of its
// own.
func (s *Server) record(r *http.Request, prov, model string, u provider.Usage) float64 {
	return s.recordMeteredFor(r, s.project(r), prov, model, usage.Tokens(u))
}

// recordForProject is record with the project named, for work attributed to a
// project that is not the caller's own — the console's flow tests, which run
// on the gateway's own provider keys and belong on the books under their own
// name rather than inside a customer's usage.
func (s *Server) recordForProject(r *http.Request, project, prov, model string, u provider.Usage) float64 {
	return s.recordMeteredFor(r, project, prov, model, usage.Tokens(u))
}

// recordMetered meters one call in whatever units its work was measured in —
// tokens, seconds of audio, images, characters spoken — and returns what the
// caller was charged.
func (s *Server) recordMetered(r *http.Request, prov, model string, m usage.Metered) float64 {
	return s.recordMeteredFor(r, s.project(r), prov, model, m)
}

func (s *Server) recordMeteredFor(r *http.Request, project, prov, model string, m usage.Metered) float64 {
	list, priced := s.usage.PriceOf(prov, model, m)
	charged, providerCost := s.charge(r.Context(), false, list)
	s.meter(r, project, prov, model, adminstore.UsageEntry{
		Requests:         1,
		PromptTokens:     int64(m.Tokens.PromptTokens),
		CompletionTokens: int64(m.Tokens.CompletionTokens),
		CostUSD:          charged,
		ProviderCostUSD:  providerCost,
		// The unpriced flag is about the gateway's own books: a call served on
		// the caller's vendor key cost the gateway nothing and was never going
		// to be priced here, so it does not count as "nobody priced this".
		Unpriced:     !priced && !servedByCaller(r.Context()),
		Estimated:    m.Tokens.Estimated,
		AudioSeconds: m.Seconds,
		Images:       m.Images,
		Characters:   m.Characters,
	})
	return charged
}

// recordBrokenUsage books a request that ended before it completed, and
// decides who pays by who ended it. The upstream breaking is the vendor's
// failure and is not the caller's bill. The caller hanging up is the caller's
// choice: the tokens delivered up to then were generated for them on the
// gateway's credential, and booking those as failed made "read the answer,
// drop the connection before the last chunk" a way to use the gateway free.
func (s *Server) recordBrokenUsage(r *http.Request, prov, model string, u provider.Usage) {
	if r.Context().Err() != nil && (u.PromptTokens > 0 || u.CompletionTokens > 0) {
		s.record(r, prov, model, u)
		return
	}
	s.recordFailedUsage(r, prov, model, u)
}

// recordFailedUsage books vendor work on a request that never completed — a
// stream cut mid-flight, a tool loop that errored on a later step. The tokens
// and the vendor cost are real and land on the books; the caller is charged
// nothing and the request is not counted as served.
func (s *Server) recordFailedUsage(r *http.Request, prov, model string, u provider.Usage) {
	if prov == "" || (u.PromptTokens == 0 && u.CompletionTokens == 0) {
		return
	}
	list, priced := s.usage.PriceOf(prov, model, usage.Tokens(u))
	_, providerCost := s.charge(r.Context(), false, list)
	s.meter(r, s.project(r), prov, model, adminstore.UsageEntry{
		Requests:         0,
		PromptTokens:     int64(u.PromptTokens),
		CompletionTokens: int64(u.CompletionTokens),
		CostUSD:          0,
		ProviderCostUSD:  providerCost,
		Unpriced:         !priced && !servedByCaller(r.Context()),
		Estimated:        u.Estimated,
		Failed:           true,
	})
}

// charge splits a call's list cost into what the caller is charged and what the
// gateway paid the vendor for it.
//
// A call served on the caller's own vendor key cost the gateway nothing and is
// charged nothing: their vendor already billed them, and the usage is recorded
// only so the console shows what ran. One served on the gateway's credential
// was paid for with the gateway's money, and the caller is charged their plan's
// rate on it — the list cost itself when they have no plan, which is how every
// deployment behaved before subscriptions existed.
func (s *Server) charge(ctx context.Context, byCaller bool, list float64) (charged, providerCost float64) {
	if byCaller || servedByCaller(ctx) {
		return 0, 0
	}
	return gatewayRateFrom(ctx).apply(list), list
}

// meter writes one metered call everywhere usage is kept: the in-memory
// tracker, the persisted counters the console reads (which also take the charge
// off the owner's balance) and the recent-requests log.
func (s *Server) meter(r *http.Request, project, prov, model string, e adminstore.UsageEntry) {
	u := provider.Usage{
		PromptTokens:     int(e.PromptTokens),
		CompletionTokens: int(e.CompletionTokens),
		TotalTokens:      int(e.PromptTokens + e.CompletionTokens),
	}
	s.usage.RecordEntry(project, prov, model, usage.Entry{
		Requests:         e.Requests,
		PromptTokens:     int(e.PromptTokens),
		CompletionTokens: int(e.CompletionTokens),
		CostUSD:          e.CostUSD,
		ProviderCostUSD:  e.ProviderCostUSD,
		Unpriced:         e.Unpriced,
		Estimated:        e.Estimated,
		Failed:           e.Failed,
		AudioSeconds:     e.AudioSeconds,
		Images:           e.Images,
		Characters:       e.Characters,
	})
	// Also accumulate into the persisted counters, so the console's numbers are
	// real across restarts rather than resetting to zero on every redeploy.
	if s.admin != nil {
		s.admin.RecordUsage(project, prov, model, e)
	}
	s.requests.Add(adminstore.RequestEntry{
		Project:         project,
		Provider:        prov,
		Model:           model,
		Tokens:          int64(u.TotalTokens),
		CostUSD:         e.CostUSD,
		ProviderCostUSD: e.ProviderCostUSD,
	})
	s.log.Info("billed", "project", project, "provider", prov, "model", model,
		"total_tokens", u.TotalTokens, "cost_usd", e.CostUSD, "provider_cost_usd", e.ProviderCostUSD,
		"unpriced", e.Unpriced, "estimated", e.Estimated, "failed", e.Failed)
}

// costHeader is what a metered response says the request cost, in dollars.
// Six decimals: a short chat on a cheap model is a few millionths.
const costHeader = "X-Nabu-Cost-USD"

func formatCost(cost float64) string {
	return strconv.FormatFloat(cost, 'f', 6, 64)
}

// handleUsage reports accumulated usage. Admin keys (not bound to a project,
// i.e. the simple api_keys) see all projects and models; project-scoped keys
// see only their own totals. Admin-ness is based on project scope, not alias
// permissions — an all-alias project key must not see other projects' usage.
func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	pol, hasPol := r.Context().Value(policyCtxKey{}).(policy.Policy)
	// Admin is granted only to the simple full-access api_keys, not to any rich
	// key that merely omits `project:` (which would leak every project's usage).
	admin := !s.authEnforced() || (hasPol && pol.Admin)
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
	scoped := s.authEnforced() && hasPol

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
		if s.authEnforced() && hasPol && !pol.Allows(a.ID) {
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

// maxResponsesCapture caps how much of a proxied Responses body is kept aside
// to pull the usage object out of. One mebibyte covers any sane final event.
// The cap is on the capture side alone — the caller's copy streams whole, no
// matter how long the body runs; what falls outside the cap is only the
// vendor's usage object, and the request then meters by size instead.
const maxResponsesCapture = 1 << 20

// cappedWriter is the capture side of the metering tee: it keeps the first
// cap bytes written to it and discards the rest, while every Write reports
// the full length as consumed — so the tee it sits behind never throttles
// what the caller receives. (An io.LimitReader on the upstream body would
// have done the opposite: returned EOF at the cap and truncated the proxied
// response itself.)
type cappedWriter struct {
	buf       bytes.Buffer
	remaining int
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	if c.remaining > 0 {
		keep := p
		if len(keep) > c.remaining {
			keep = keep[:c.remaining]
		}
		c.buf.Write(keep)
		c.remaining -= len(keep)
	}
	return len(p), nil
}

// responsesUsageWire is the usage object the Responses API carries — in the
// JSON body, and inside the response object of the "response.completed" SSE
// event. input_tokens/output_tokens is OpenAI's spelling for this API;
// prompt_tokens/completion_tokens is accepted as well because resellers that
// proxy the endpoint have been seen emitting either.
type responsesUsageWire struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	TotalTokens      int `json:"total_tokens"`
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

func (u responsesUsageWire) usage() provider.Usage {
	prompt, completion := u.InputTokens, u.OutputTokens
	if prompt == 0 && completion == 0 {
		prompt, completion = u.PromptTokens, u.CompletionTokens
	}
	// Some OpenAI-compatible servers report the split but omit the total; the
	// sum is what the rest of the metering expects to see.
	total := u.TotalTokens
	if total == 0 {
		total = prompt + completion
	}
	return provider.Usage{PromptTokens: prompt, CompletionTokens: completion, TotalTokens: total}
}

// parseResponsesUsage pulls the token usage out of a captured Responses body:
// the "usage" object of a JSON response, or the one inside the final
// "response.completed" event of an SSE stream. ok is false when neither is
// found — the caller then meters the request by size instead of nothing.
func parseResponsesUsage(captured []byte) (u provider.Usage, ok bool) {
	trimmed := bytes.TrimLeft(captured, " \t\r\n")
	if len(trimmed) == 0 {
		return provider.Usage{}, false
	}
	if trimmed[0] != '{' {
		// SSE: walk the data lines for the completion event, which is the one
		// that carries the finished response's usage.
		for _, line := range strings.Split(string(trimmed), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			var ev struct {
				Type     string `json:"type"`
				Response struct {
					Usage *responsesUsageWire `json:"usage"`
				} `json:"response"`
			}
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data:")), &ev) != nil {
				continue
			}
			if ev.Type == "response.completed" && ev.Response.Usage != nil {
				return ev.Response.Usage.usage(), true
			}
		}
		return provider.Usage{}, false
	}

	var body struct {
		Usage *responsesUsageWire `json:"usage"`
	}
	if json.Unmarshal(trimmed, &body) != nil || body.Usage == nil {
		return provider.Usage{}, false
	}
	return body.Usage.usage(), true
}

// handleResponses proxies the OpenAI Responses API (POST /v1/responses). Like
// chat, only "model" (a NabuGate alias or a "<provider>/<model>" passthrough)
// is inspected and rewritten to the upstream model; the rest of the body is
// forwarded verbatim, and the upstream response — JSON or streaming SSE — is
// copied straight back, byte for byte.
//
// Metering reads the usage out of a capped copy of the upstream body kept
// while the bytes stream past, so a served request lands on the books: the
// vendor's own usage object when one is present, otherwise one request
// estimated from the body sizes and flagged as a guess. Only a 2xx upstream
// is metered — an error body carries no work worth billing.
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
	// as they arrive instead of buffering until the response completes. A 2xx
	// body is teed through a capped capture on the way out — the tee sits on
	// the read side and the cap lives in the writer, so the caller's copy is
	// byte-identical and complete however long the body runs — and the usage
	// is metered afterwards.
	capture := resp.StatusCode >= 200 && resp.StatusCode < 300
	var captured cappedWriter
	body := io.Reader(resp.Body)
	if capture {
		captured.remaining = maxResponsesCapture
		body = io.TeeReader(resp.Body, &captured)
	}
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 16<<10)
	for {
		n, rerr := body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				// The caller hung up; nothing more to proxy. Whatever was
				// captured still meters below.
				break
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if rerr != nil {
			break
		}
	}

	if !capture {
		return
	}
	if u, ok := parseResponsesUsage(captured.buf.Bytes()); ok && (u.PromptTokens > 0 || u.CompletionTokens > 0) {
		s.record(r, prov, upstream, u)
		return
	}
	// No usage object survived the trip: the request still cost the vendor
	// something, so it is metered as one request estimated from the sizes of
	// what went out and came back — a flagged guess rather than a silent zero.
	pTok := provider.EstimateTokens(len(raw["input"]) + len(raw["instructions"]))
	cTok := provider.EstimateTokens(captured.buf.Len())
	s.record(r, prov, upstream, provider.Usage{
		PromptTokens:     pTok,
		CompletionTokens: cTok,
		TotalTokens:      pTok + cTok,
		Estimated:        true,
	})
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

	cost := s.record(r, result.Provider, result.Model, result.Response.Usage)
	s.saveTurn(r, convID, newTurns, result.Response.Content)

	w.Header().Set("X-Nabu-Provider", result.Provider)
	w.Header().Set("X-Nabu-Model", result.Model)
	w.Header().Set(costHeader, formatCost(cost))

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
		"usage": map[string]any{
			"prompt_tokens":     result.Response.Usage.PromptTokens,
			"completion_tokens": result.Response.Usage.CompletionTokens,
			"total_tokens":      result.Response.Usage.TotalTokens,
			"cost_usd":          cost,
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

	// include_usage is OpenAI's own opt-in for a final usage chunk on the
	// stream. When the caller asked for it (the gateway's adapters always do
	// towards upstream), one standard-shaped chunk — empty choices, a usage
	// object — closes the stream, so an OpenAI-SDK client that asked gets its
	// numbers from the wire and every other caller gets not one extra byte.
	var wantUsageChunk bool
	var streamOpts struct {
		IncludeUsage bool `json:"include_usage"`
	}
	if len(req.Raw["stream_options"]) > 0 {
		if json.Unmarshal(req.Raw["stream_options"], &streamOpts) == nil {
			wantUsageChunk = streamOpts.IncludeUsage
		}
	}

	// The caller's tools are theirs to run, so a streamed tool call is
	// forwarded as OpenAI's own delta and the stream ends with finish_reason
	// "tool_calls" — what LangGraph, the Vercel AI SDK and the Agents SDK wait
	// for before they execute anything.
	sawToolCalls := false
	ctx := r.Context()
	if clientSuppliesTools(req.Raw) {
		ctx = provider.WithToolDeltas(ctx, func(raw json.RawMessage) error {
			if !headersWritten {
				startHeaders()
			}
			sawToolCalls = true
			writeSSE(streamChunk(id, created, alias, metaProvider, metaModel, map[string]any{"tool_calls": raw}, nil))
			return nil
		})
	}

	result, err := s.router.ChatStream(ctx, routeModel, req,
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

	// Bill successful generations, matching the non-streaming path. A stream
	// that broke mid-flight after the upstream had already metered work keeps
	// its real token counts — booked as failed: vendor cost on the books, no
	// charge to the caller, no served request counted.
	var cost float64
	if err == nil {
		cost = s.record(r, result.Provider, result.Model, result.Usage)
		// Store only a stream that produced something. A failed or empty
		// generation is not a turn, and persisting it would replay an empty
		// assistant message into every later call.
		s.saveTurn(r, convID, newTurns, replyBuf.String())
	} else if result.Provider != "" {
		s.recordBrokenUsage(r, result.Provider, result.Model, result.Usage)
	}

	finish := "stop"
	if sawToolCalls {
		finish = "tool_calls"
	}
	if err != nil {
		finish = "error"
	}
	writeSSE(streamChunk(id, created, alias, result.Provider, result.Model, map[string]any{}, &finish))
	if wantUsageChunk && err == nil {
		// OpenAI's own shape for the terminal usage chunk: choices is empty,
		// usage carries the token counts, and cost_usd rides alongside as the
		// gateway's additive extra so the caller sees what they were charged.
		chunk := streamChunk(id, created, alias, result.Provider, result.Model, nil, nil)
		chunk["choices"] = []any{}
		chunk["usage"] = map[string]any{
			"prompt_tokens":     result.Usage.PromptTokens,
			"completion_tokens": result.Usage.CompletionTokens,
			"total_tokens":      result.Usage.TotalTokens,
			"cost_usd":          cost,
		}
		writeSSE(chunk)
	}
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
	Quality     string `json:"quality"`
	// Not OpenAI's; honoured by the vendors that take one (Wan).
	NegativePrompt string `json:"negative_prompt"`
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
		Prompt:         body.Prompt,
		N:              body.N,
		Size:           body.Size,
		AspectRatio:    body.AspectRatio,
		Quality:        body.Quality,
		NegativePrompt: body.NegativePrompt,
	})
	if err != nil {
		writeError(w, aliasErrStatus(err, "unknown image alias"), err.Error())
		return
	}

	cost := s.recordMetered(r, result.Provider, result.Model, usage.Metered{Images: len(result.Images)})

	w.Header().Set("X-Nabu-Provider", result.Provider)
	w.Header().Set("X-Nabu-Model", result.Model)
	w.Header().Set(costHeader, formatCost(cost))

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

	// Speech is billed by the characters spoken: a vendor quotes it per
	// character, or per token of an input that is these same characters.
	cost := s.recordMetered(r, result.Provider, result.Model, usage.Metered{Characters: utf8.RuneCountInString(body.Input)})

	// OpenAI's /v1/audio/speech returns raw audio bytes, so we do too.
	w.Header().Set("X-Nabu-Provider", result.Provider)
	w.Header().Set("X-Nabu-Model", result.Model)
	w.Header().Set(costHeader, formatCost(cost))
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

// looksTokenized reports whether an embeddings input is OpenAI's
// pre-tokenized form: an array of integers, or an array of such arrays.
func looksTokenized(raw json.RawMessage) bool {
	var flat []int
	if json.Unmarshal(raw, &flat) == nil && len(flat) > 0 {
		return true
	}
	var nested [][]int
	return json.Unmarshal(raw, &nested) == nil && len(nested) > 0
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
			if looksTokenized(body.Input) {
				// Token ids are one vendor's tokenizer, and an alias may fall
				// back to another vendor, where the same ids are different
				// words: the vectors would be wrong without any error. This
				// is LangChain's OpenAIEmbeddings default, so the fix is named.
				writeErrorCode(w, http.StatusBadRequest, "tokenized_input",
					"field 'input' must be text, not token ids: an alias can be served by a vendor with a different tokenizer. "+
						"With LangChain's OpenAIEmbeddings set check_embedding_ctx_length=False; with LlamaIndex or LiteLLM send strings", nil)
				return
			}
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

	// Gemini's embedding endpoint reports no usage at all, and 28,460 calls of
	// it were metered at nothing before this was counted here.
	usedTokens := result.Usage
	estimatedUsage := false
	if usedTokens.PromptTokens == 0 {
		var inputBytes int
		for _, in := range inputs {
			inputBytes += len(in)
		}
		usedTokens.PromptTokens = provider.EstimateTokens(inputBytes)
		usedTokens.TotalTokens = usedTokens.PromptTokens
		usedTokens.Estimated = true
		estimatedUsage = true
	}
	cost := s.record(r, result.Provider, result.Model, usedTokens)

	w.Header().Set("X-Nabu-Provider", result.Provider)
	w.Header().Set("X-Nabu-Model", result.Model)
	w.Header().Set(costHeader, formatCost(cost))

	data := make([]map[string]any, 0, len(result.Embeddings))
	for i, vec := range result.Embeddings {
		data = append(data, map[string]any{
			"object":    "embedding",
			"index":     i,
			"embedding": encodeEmbedding(vec, body.EncodingFormat),
		})
	}
	// The usage the caller sees is the usage that was metered: a count the
	// gateway estimated is returned instead of the vendor's zero, flagged as
	// such, so a client keeping its own books reads the same number we booked.
	usageOut := map[string]any{
		"prompt_tokens": usedTokens.PromptTokens,
		"total_tokens":  usedTokens.TotalTokens,
		"cost_usd":      cost,
	}
	if estimatedUsage {
		usageOut["estimated"] = true
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"object":         "list",
		"model":          result.Alias,
		"provider":       result.Provider,
		"upstream_model": result.Model,
		"data":           data,
		"usage":          usageOut,
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

// lowBalanceUSD is the remaining balance under which metered responses start
// carrying X-Nabu-Balance-Warning. One dollar is a few hundred nabu-smart
// calls: enough notice to top up, not so much that the header is always on.
const lowBalanceUSD = 1.0

// auth validates the bearer token, enforces the per-key rate limit, and stores
// the resolved policy in the request context for later alias checks. When no
// keys are configured, requests pass through (dev mode).
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Cap the body before any handler reads it (bounds memory / slow-loris).
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
		// Caller-supplied upstream credentials ride on every request, whether or
		// not this deployment enforces its own keys.
		keyCtx, _ := withCallerKeys(r.Context(), r)
		r = r.WithContext(keyCtx)
		if !s.authEnforced() {
			next(w, r)
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))

		// owner stays empty for a key baked into the deployment's config. Only
		// a console-minted token belongs to a person, and only a person has
		// stored credentials or access grants — which is what keeps every
		// existing internal integration untouched by both.
		var owner string
		pol, ok := s.policy.Lookup(token)
		if !ok {
			// Not in the baked config — try the tokens minted from the console.
			// Config keys are checked first so a deployment's declared keys can
			// never be shadowed by something added at runtime.
			if t, found := s.lookupConsoleToken(token); found {
				if !originAllowed(t.AllowedOrigins, r) {
					s.admin.RecordDenied(t.Name)
					s.requests.Add(adminstore.RequestEntry{
						Project: t.Name, Denied: true,
						Reason: "origin not permitted for this key",
					})
					s.log.Warn("origin refused", "project", t.Name,
						"origin", requestOriginHost(r), "allowed", t.AllowedOrigins)
					writeError(w, http.StatusForbidden, "this key is not permitted from this origin")
					return
				}
				if t.Owner != "" && s.admin.GetUser(t.Owner) == nil {
					// The account the key bills is gone. Serving it would be
					// serving a key nobody pays for.
					s.admin.RecordDenied(t.Name)
					writeError(w, http.StatusForbidden, "the account this key belongs to no longer exists")
					return
				}
				if t.Owner != "" {
					if u := s.admin.GetUser(t.Owner); u != nil {
						if u.Balance <= 0 {
							s.admin.RecordDenied(t.Name)
							s.requests.Add(adminstore.RequestEntry{
								Project: t.Name, Denied: true,
								Reason: "insufficient balance",
							})
							s.log.Warn("insufficient balance", "project", t.Name, "owner", t.Owner, "balance", u.Balance)
							writeError(w, http.StatusPaymentRequired, "insufficient balance: top up your NabuGate account to keep this key working")
							return
						}
						// A hard 402 should never be the first thing a customer hears
						// about their balance. Every metered response carries what is
						// left, and a warning header once it drops under the threshold,
						// so a caller can alert or top up before requests start failing.
						w.Header().Set("X-Nabu-Balance-USD", strconv.FormatFloat(u.Balance, 'f', 4, 64))
						if u.Balance < lowBalanceUSD {
							w.Header().Set("X-Nabu-Balance-Warning", "low")
						}
					}
				}
				pol = policy.Policy{Project: t.Name, Allow: t.Allow, RateLimit: t.RateLimit, Providers: t.Providers}
				owner = t.Owner
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
		limited := !s.policy.RateOK(token)
		if !limited && !isConfigKey(s, token) {
			// A console token's limit lives in the admin store, which the
			// enforcer was never configured with; it used to be ignored.
			limited = !s.policy.RateOKLimit(pol.Project, pol.RateLimit)
		}
		if limited {
			s.requests.Add(adminstore.RequestEntry{
				Project: pol.Project, Denied: true,
				Reason: "rate limit exceeded",
			})
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		if owner != "" && !spendsOwnKeysOnly(callerKeysFrom(r)) {
			if u := s.admin.GetUser(owner); u != nil {
				if !s.reserved.hold(owner, u.Balance) {
					s.requests.Add(adminstore.RequestEntry{
						Project: pol.Project, Denied: true,
						Reason: "balance held by requests in flight",
					})
					w.Header().Set("Retry-After", "1")
					writeError(w, http.StatusPaymentRequired, "insufficient balance for another concurrent request: wait for the running ones to finish, or top up")
					return
				}
				defer s.reserved.release(owner)
			}
		}
		ctx := context.WithValue(r.Context(), policyCtxKey{}, pol)
		ctx = context.WithValue(ctx, router.AllowedProvidersCtxKey{}, pol.Providers)
		ctx = s.withOwnerCredentials(ctx, owner)
		next(w, r.WithContext(ctx))
	}
}

// aliasAllowed reports whether the request's key may use the given alias, and
// writes a 403 if not. Returns true when policy is disabled.
func (s *Server) aliasAllowed(w http.ResponseWriter, r *http.Request, alias string) bool {
	if !s.authEnforced() {
		return true
	}
	pol, ok := r.Context().Value(policyCtxKey{}).(policy.Policy)
	if ok && pol.Allows(alias) {
		return true
	}
	// The alias is named in the entry because that is the whole content of the
	// answer: "your key was refused" is not actionable, "your key was refused
	// nabu-smart" is a one-line fix to the allow-list.
	s.requests.Add(adminstore.RequestEntry{
		Project: pol.Project, Model: alias, Denied: true,
		Reason: "alias not permitted for this key",
	})
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

// writeErrorCode is writeError with a stable machine code, plus any facts the
// client needs to act on it. The message stays English; the console translates
// by code, because a sentence from here reaching a Persian page verbatim is a
// broken page, and a code survives every wording change.
func writeErrorCode(w http.ResponseWriter, status int, code, msg string, extra map[string]any) {
	e := map[string]any{"message": msg, "code": code}
	for k, v := range extra {
		e[k] = v
	}
	writeJSON(w, status, map[string]any{"error": e})
}

func contains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
