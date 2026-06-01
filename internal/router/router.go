// Package router resolves a public model alias to upstream targets and executes
// the request against the primary provider, falling back through the configured
// list on failure. Each upstream attempt is logged for observability.
package router

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"nabugate/internal/config"
	"nabugate/internal/provider"
)

// Router holds the live adapters and the alias routing table.
type Router struct {
	adapters map[string]provider.Adapter
	models   map[string]config.ModelRoute
	log      *slog.Logger
}

// New builds a Router.
func New(adapters map[string]provider.Adapter, models map[string]config.ModelRoute, log *slog.Logger) *Router {
	return &Router{adapters: adapters, models: models, log: log}
}

// Result is the outcome of a successful routed call.
type Result struct {
	Alias    string
	Provider string
	Model    string
	Response provider.ChatResponse
}

// Aliases returns the configured public model aliases.
func (r *Router) Aliases() []string {
	out := make([]string, 0, len(r.models))
	for a := range r.models {
		out = append(out, a)
	}
	return out
}

// Chat resolves the alias and tries the primary target then each fallback in
// order, returning the first success. If the alias is unknown but matches a
// live provider's real model name directly, callers should pre-resolve; here we
// only handle configured aliases.
func (r *Router) Chat(ctx context.Context, alias string, req provider.ChatRequest) (Result, error) {
	route, ok := r.models[alias]
	if !ok {
		return Result{}, fmt.Errorf("unknown model alias %q", alias)
	}

	targets := append([]config.Target{route.Primary}, route.Fallback...)
	var lastErr error

	for i, t := range targets {
		adapter, ok := r.adapters[t.Provider]
		if !ok {
			lastErr = fmt.Errorf("provider %q not available", t.Provider)
			r.log.Warn("skip target", "alias", alias, "provider", t.Provider, "model", t.Model, "reason", "provider unavailable")
			continue
		}

		req.Model = t.Model
		start := time.Now()
		resp, err := adapter.Chat(ctx, req)
		latency := time.Since(start)

		attrs := []any{
			"alias", alias,
			"provider", t.Provider,
			"model", t.Model,
			"attempt", i + 1,
			"latency_ms", latency.Milliseconds(),
		}
		if err != nil {
			lastErr = err
			r.log.Warn("upstream failed", append(attrs, "error", err.Error())...)
			continue
		}

		r.log.Info("upstream ok",
			append(attrs,
				"prompt_tokens", resp.Usage.PromptTokens,
				"completion_tokens", resp.Usage.CompletionTokens,
				"total_tokens", resp.Usage.TotalTokens,
			)...)

		return Result{Alias: alias, Provider: t.Provider, Model: t.Model, Response: resp}, nil
	}

	return Result{}, fmt.Errorf("all targets failed for alias %q: %w", alias, lastErr)
}

// MarshalAliases returns a JSON-friendly model list (OpenAI /v1/models shape).
func (r *Router) MarshalAliases() ([]byte, error) {
	type model struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		OwnedBy string `json:"owned_by"`
	}
	type list struct {
		Object string  `json:"object"`
		Data   []model `json:"data"`
	}
	out := list{Object: "list"}
	for alias, route := range r.models {
		out.Data = append(out.Data, model{ID: alias, Object: "model", OwnedBy: route.Primary.Provider})
	}
	return json.Marshal(out)
}
