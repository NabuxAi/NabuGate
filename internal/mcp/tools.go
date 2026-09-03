// The tools this service exposes. One file per repo; mcp.go beside it is
// identical in all four.
//
// Every tool is read-only, and every one of them builds its answer out of a
// struct declared at the bottom of this file with named fields. Nothing here
// returns a config struct, a provider adapter, a store row that carries a
// credential, or an upstream error's text.
//
// The strongest form of that rule available to this repo is structural, and it
// is worth stating plainly: THIS FILE DOES NOT IMPORT internal/config. NabuGate
// is the service that holds every provider key in the estate, and every one of
// them is reachable from *config.Config — config.ProviderConfig carries both
// BaseURL and APIKeyEnv, and policy.KeyConfig carries a live gateway key. A
// tools.go that cannot name those types cannot leak them by accident, which is
// a better guarantee than remembering not to. Everything below is derived from
// the already-sanitised views the router, the usage tracker, the agent registry
// and the request log expose.
package mcp

import (
	"context"
	"sort"
	"strings"
	"time"

	"nabugate/internal/adminstore"
	"nabugate/internal/agent"
	"nabugate/internal/router"
	"nabugate/internal/usage"
)

// Version is what serverInfo reports. Bump it when the tool set changes.
const Version = "1.0.0"

// maxRequests caps how many rows nabugate_requests_list will return. The log
// itself is a bounded ring (adminstore.NewRequestLog(500)); this keeps one
// answer inside a size a model can actually read.
const maxRequests = 200

// Register wires this service's tools onto a Server.
//
// The dependencies are the same concrete types server.New already takes, plus
// the request-log ring the server builds for itself (handed over by main.go via
// (*server.Server).Requests()). Nothing new is constructed and nothing here
// calls the gateway over HTTP: these are the same in-process reads the console
// API performs.
func Register(s *Server, r *router.Router, tracker *usage.Tracker, agents *agent.Registry, requests *adminstore.RequestLog) {
	s.Register(Tool{
		Name: "nabugate_agents_list",
		Description: "List the configured sub-agents addressable as a model: name, description, and the alias each runs on. " +
			"System prompts and agent tool definitions are never included.",
		InputSchema: ObjectSchema(nil, nil),
		Handler: func(_ context.Context, _ map[string]any) (any, error) {
			names := agents.Names()
			out := make([]agentView, 0, len(names))

			for _, name := range names {
				a, ok := agents.Lookup(name)
				if !ok {
					continue
				}

				// Three fields, chosen by hand. agent.Agent also carries System
				// (the prompt) and Tools, and agent.Tool carries a URL and a
				// Headers map — which is exactly where a downstream API key
				// lives. Marshalling the agent whole would publish both.
				out = append(out, agentView{Name: a.Name, Description: a.Description, Model: a.Model})
			}

			sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

			return map[string]any{"agents": out}, nil
		},
	})

	s.Register(Tool{
		Name: "nabugate_health_get",
		Description: "Report how many live upstream targets stand behind each alias right now, which aliases are degraded, and why. " +
			"Provider names only: no base URLs and no API-key variable names.",
		InputSchema: ObjectSchema(nil, nil),
		Handler: func(_ context.Context, _ map[string]any) (any, error) {
			all := r.AliasHealthAll()

			aliases := make([]healthView, 0, len(all))
			degraded := 0

			for _, a := range all {
				if len(a.Warnings) > 0 {
					degraded++
				}

				aliases = append(aliases, newHealthView(a))
			}

			status := "ok"
			switch {
			case len(aliases) == 0:
				status = "unusable"
			case degraded > 0:
				status = "degraded"
			}

			return map[string]any{"status": status, "degraded": degraded, "aliases": aliases}, nil
		},
	})

	s.Register(Tool{
		Name: "nabugate_models_list",
		Description: "List the routing aliases this gateway serves: id, kind, how many live upstream targets each resolves to, and the provider names behind it. " +
			"Never a base URL and never the name of the variable a provider's key arrives in.",
		InputSchema: ObjectSchema(nil, nil),
		Handler: func(_ context.Context, _ map[string]any) (any, error) {
			all := r.AliasHealthAll()

			models := make([]modelView, 0, len(all))
			for _, a := range all {
				models = append(models, modelView{
					ID:        a.ID,
					Kind:      a.Kind,
					Live:      a.Live,
					Providers: append([]string(nil), a.Providers...),
				})
			}

			return map[string]any{"models": models}, nil
		},
	})

	s.Register(Tool{
		Name: "nabugate_requests_list",
		Description: "List recent calls through the gateway, newest first: project, alias or upstream model, provider, tokens, cost, and the reason any call was denied. " +
			"Prompt and completion bodies are never recorded and never returned.",
		InputSchema: ObjectSchema(map[string]any{
			"project": StringProp("Project name to filter by; omit for every project"),
			"limit":   IntProp("How many rows to return", 1, maxRequests),
		}, nil),
		Handler: func(_ context.Context, args map[string]any) (any, error) {
			project := OptString(args, "project", "")
			limit := Int(args, "limit", 50, 1, maxRequests)

			// nil means every project, which is deliberate here: this endpoint
			// is not a gateway key bound to one project, it is the operator's
			// read across the deployment, and the token that reaches it is the
			// one the contract requires to be separate from every project key.
			//
			// The ring lower-cases the project it compares against, so the
			// filter has to be lower-cased too — a mixed-case argument would
			// otherwise return zero rows and read as "no traffic".
			var filter map[string]bool
			if project != "" {
				filter = map[string]bool{strings.ToLower(project): true}
			}

			rows := requests.Recent(filter, limit)

			out := make([]requestView, 0, len(rows))
			for _, e := range rows {
				out = append(out, newRequestView(e))
			}

			return map[string]any{"requests": out}, nil
		},
	})

	s.Register(Tool{
		Name: "nabugate_usage_get",
		Description: "Report accumulated spend: requests, prompt and completion tokens, and USD cost, broken down per project and per upstream model. " +
			"Filter to one project by name.",
		InputSchema: ObjectSchema(map[string]any{
			"project": StringProp("Project name to report on; omit for every project"),
		}, nil),
		Handler: func(_ context.Context, args map[string]any) (any, error) {
			byProject, byModel := tracker.Snapshot()

			if project := OptString(args, "project", ""); project != "" {
				stat, ok := byProject[project]
				if !ok {
					// A ToolFailure, not a JSON-RPC error: the caller asked a
					// reasonable question and can act on the answer. The only
					// thing interpolated is the name they supplied.
					return nil, Failf("no usage has been recorded for project %q", project)
				}

				return map[string]any{"projects": []statView{newStatView(project, stat)}}, nil
			}

			return map[string]any{
				"projects": statViews(byProject),
				"models":   statViews(byModel),
			}, nil
		},
	})
}

// The response types. Named fields only — that is the whole defence.

type agentView struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Model       string `json:"model,omitempty"`
}

type modelView struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Live      int      `json:"live_targets"`
	Providers []string `json:"providers,omitempty"`
}

type healthView struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Live       int      `json:"live_targets"`
	Configured int      `json:"configured_targets"`
	Providers  []string `json:"providers,omitempty"`
	Warnings   []string `json:"warnings,omitempty"`
}

// newHealthView copies router.AliasHealth field by field rather than embedding
// it. The two structs are the same shape today; the copy is what stops a field
// added to AliasHealth tomorrow from appearing on this endpoint unreviewed.
//
// Warnings are carried through because every one of them is a fixed literal
// built in router.aliasHealth, at worst suffixed with a provider *name* — the
// same names this tool already reports. No warning interpolates a base URL or
// an api_key_env value; that was checked, not assumed.
func newHealthView(a router.AliasHealth) healthView {
	return healthView{
		ID:         a.ID,
		Kind:       a.Kind,
		Live:       a.Live,
		Configured: a.Configured,
		Providers:  append([]string(nil), a.Providers...),
		Warnings:   append([]string(nil), a.Warnings...),
	}
}

type requestView struct {
	At       string  `json:"at"`
	Project  string  `json:"project,omitempty"`
	Provider string  `json:"provider,omitempty"`
	Model    string  `json:"model,omitempty"`
	Tokens   int64   `json:"tokens,omitempty"`
	CostUSD  float64 `json:"cost_usd,omitempty"`
	Denied   bool    `json:"denied,omitempty"`
	Reason   string  `json:"reason,omitempty"`
}

func newRequestView(e adminstore.RequestEntry) requestView {
	return requestView{
		At:       e.At.UTC().Format(time.RFC3339),
		Project:  e.Project,
		Provider: e.Provider,
		Model:    e.Model,
		Tokens:   e.Tokens,
		CostUSD:  e.CostUSD,
		Denied:   e.Denied,
		Reason:   e.Reason,
	}
}

type statView struct {
	Name             string  `json:"name"`
	Requests         int64   `json:"requests"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}

func newStatView(name string, s usage.Stat) statView {
	return statView{
		Name:             name,
		Requests:         s.Requests,
		PromptTokens:     s.PromptTokens,
		CompletionTokens: s.CompletionTokens,
		CostUSD:          s.CostUSD,
	}
}

// statViews flattens a snapshot map into a sorted slice, so two calls with the
// same data produce the same bytes.
func statViews(m map[string]usage.Stat) []statView {
	out := make([]statView, 0, len(m))
	for name, s := range m {
		out = append(out, newStatView(name, s))
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out
}
