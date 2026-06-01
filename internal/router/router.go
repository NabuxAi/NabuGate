// Package router resolves a public model alias to upstream targets and executes
// the request against the primary provider, falling back through the configured
// list on failure. Each upstream attempt is logged for observability.
package router

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"nabugate/internal/config"
	"nabugate/internal/provider"
)

// Router holds the live adapters and the alias routing tables (one per
// capability: chat, images, audio).
type Router struct {
	adapters   map[string]provider.Adapter
	models     map[string]config.ModelRoute
	images     map[string]config.ModelRoute
	audio      map[string]config.ModelRoute
	embeddings map[string]config.ModelRoute
	log        *slog.Logger
}

// New builds a Router.
func New(adapters map[string]provider.Adapter, models, images, audio, embeddings map[string]config.ModelRoute, log *slog.Logger) *Router {
	return &Router{adapters: adapters, models: models, images: images, audio: audio, embeddings: embeddings, log: log}
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

// StreamResult is the outcome of a (possibly partial) streaming completion.
type StreamResult struct {
	Provider string
	Model    string
	Usage    provider.Usage
}

// ChatStream resolves a chat alias and streams the first stream-capable target,
// falling back to the next target only while no delta has been emitted yet
// (once bytes are on the wire we are committed to that provider). onMeta is
// called with the chosen provider/model before each attempt so the caller can
// emit response headers lazily on the first delta.
func (r *Router) ChatStream(ctx context.Context, alias string, req provider.ChatRequest, onMeta func(providerName, model string), onDelta provider.DeltaFunc) (StreamResult, error) {
	route, ok := r.models[alias]
	if !ok {
		return StreamResult{}, fmt.Errorf("unknown model alias %q", alias)
	}
	targets := append([]config.Target{route.Primary}, route.Fallback...)
	var lastErr error

	for i, t := range targets {
		adapter, ok := r.adapters[t.Provider]
		if !ok {
			lastErr = fmt.Errorf("provider %q not available", t.Provider)
			continue
		}
		streamer, ok := adapter.(provider.StreamAdapter)
		if !ok {
			lastErr = fmt.Errorf("provider %q does not support streaming", t.Provider)
			r.log.Warn("skip stream target", "alias", alias, "provider", t.Provider, "reason", "no stream support")
			continue
		}

		onMeta(t.Provider, t.Model)
		req.Model = t.Model
		started := false
		start := time.Now()
		usage, err := streamer.ChatStream(ctx, req, func(delta string) error {
			started = true
			return onDelta(delta)
		})
		attrs := []any{"capability", "chat-stream", "alias", alias, "provider", t.Provider, "model", t.Model, "attempt", i + 1, "latency_ms", time.Since(start).Milliseconds()}
		if err != nil {
			lastErr = err
			r.log.Warn("upstream failed", append(attrs, "error", err.Error(), "started", started)...)
			if started {
				// Cannot fall back once the client has received bytes.
				return StreamResult{Provider: t.Provider, Model: t.Model, Usage: usage}, err
			}
			continue
		}
		r.log.Info("upstream ok", append(attrs, "total_tokens", usage.TotalTokens)...)
		return StreamResult{Provider: t.Provider, Model: t.Model, Usage: usage}, nil
	}
	return StreamResult{}, fmt.Errorf("all targets failed for alias %q: %w", alias, lastErr)
}

// ImageResult is the outcome of a successful image generation.
type ImageResult struct {
	Alias    string
	Provider string
	Model    string
	Images   []string // base64 PNG
}

// Image resolves an image alias and tries primary then fallbacks.
func (r *Router) Image(ctx context.Context, alias string, req provider.ImageRequest) (ImageResult, error) {
	route, ok := r.images[alias]
	if !ok {
		return ImageResult{}, fmt.Errorf("unknown image alias %q", alias)
	}
	targets := append([]config.Target{route.Primary}, route.Fallback...)
	var lastErr error

	for i, t := range targets {
		adapter, ok := r.adapters[t.Provider]
		if !ok {
			lastErr = fmt.Errorf("provider %q not available", t.Provider)
			continue
		}
		imgAdapter, ok := adapter.(provider.ImageAdapter)
		if !ok {
			lastErr = fmt.Errorf("provider %q does not support images", t.Provider)
			r.log.Warn("skip image target", "alias", alias, "provider", t.Provider, "reason", "no image support")
			continue
		}

		req.Model = t.Model
		start := time.Now()
		resp, err := imgAdapter.Image(ctx, req)
		attrs := []any{"capability", "image", "alias", alias, "provider", t.Provider, "model", t.Model, "attempt", i + 1, "latency_ms", time.Since(start).Milliseconds()}
		if err != nil {
			lastErr = err
			r.log.Warn("upstream failed", append(attrs, "error", err.Error())...)
			continue
		}
		r.log.Info("upstream ok", append(attrs, "images", len(resp.Images))...)
		return ImageResult{Alias: alias, Provider: t.Provider, Model: t.Model, Images: resp.Images}, nil
	}
	return ImageResult{}, fmt.Errorf("all targets failed for image alias %q: %w", alias, lastErr)
}

// SpeechResult is the outcome of a successful speech synthesis.
type SpeechResult struct {
	Alias       string
	Provider    string
	Model       string
	Audio       []byte
	ContentType string
}

// Speech resolves an audio alias and tries primary then fallbacks.
func (r *Router) Speech(ctx context.Context, alias string, req provider.SpeechRequest) (SpeechResult, error) {
	route, ok := r.audio[alias]
	if !ok {
		return SpeechResult{}, fmt.Errorf("unknown audio alias %q", alias)
	}
	targets := append([]config.Target{route.Primary}, route.Fallback...)
	var lastErr error

	for i, t := range targets {
		adapter, ok := r.adapters[t.Provider]
		if !ok {
			lastErr = fmt.Errorf("provider %q not available", t.Provider)
			continue
		}
		spAdapter, ok := adapter.(provider.SpeechAdapter)
		if !ok {
			lastErr = fmt.Errorf("provider %q does not support speech", t.Provider)
			r.log.Warn("skip audio target", "alias", alias, "provider", t.Provider, "reason", "no speech support")
			continue
		}

		req.Model = t.Model
		start := time.Now()
		resp, err := spAdapter.Speech(ctx, req)
		attrs := []any{"capability", "speech", "alias", alias, "provider", t.Provider, "model", t.Model, "attempt", i + 1, "latency_ms", time.Since(start).Milliseconds()}
		if err != nil {
			lastErr = err
			r.log.Warn("upstream failed", append(attrs, "error", err.Error())...)
			continue
		}
		r.log.Info("upstream ok", append(attrs, "bytes", len(resp.Audio))...)
		return SpeechResult{Alias: alias, Provider: t.Provider, Model: t.Model, Audio: resp.Audio, ContentType: resp.ContentType}, nil
	}
	return SpeechResult{}, fmt.Errorf("all targets failed for audio alias %q: %w", alias, lastErr)
}

// EmbedResult is the outcome of a successful embedding call.
type EmbedResult struct {
	Alias      string
	Provider   string
	Model      string
	Embeddings [][]float64
	Usage      provider.Usage
}

// Embed resolves an embedding alias and tries primary then fallbacks.
func (r *Router) Embed(ctx context.Context, alias string, req provider.EmbeddingRequest) (EmbedResult, error) {
	route, ok := r.embeddings[alias]
	if !ok {
		return EmbedResult{}, fmt.Errorf("unknown embedding alias %q", alias)
	}
	targets := append([]config.Target{route.Primary}, route.Fallback...)
	var lastErr error

	for i, t := range targets {
		adapter, ok := r.adapters[t.Provider]
		if !ok {
			lastErr = fmt.Errorf("provider %q not available", t.Provider)
			continue
		}
		embAdapter, ok := adapter.(provider.EmbeddingAdapter)
		if !ok {
			lastErr = fmt.Errorf("provider %q does not support embeddings", t.Provider)
			r.log.Warn("skip embedding target", "alias", alias, "provider", t.Provider, "reason", "no embedding support")
			continue
		}

		req.Model = t.Model
		start := time.Now()
		resp, err := embAdapter.Embed(ctx, req)
		attrs := []any{"capability", "embedding", "alias", alias, "provider", t.Provider, "model", t.Model, "attempt", i + 1, "latency_ms", time.Since(start).Milliseconds()}
		if err != nil {
			lastErr = err
			r.log.Warn("upstream failed", append(attrs, "error", err.Error())...)
			continue
		}
		r.log.Info("upstream ok", append(attrs, "vectors", len(resp.Embeddings), "total_tokens", resp.Usage.TotalTokens)...)
		return EmbedResult{Alias: alias, Provider: t.Provider, Model: t.Model, Embeddings: resp.Embeddings, Usage: resp.Usage}, nil
	}
	return EmbedResult{}, fmt.Errorf("all targets failed for embedding alias %q: %w", alias, lastErr)
}

// AliasInfo describes one public alias and the provider that primarily serves it.
type AliasInfo struct {
	ID    string
	Owner string
}

// AliasInfos returns every configured alias across all capabilities.
func (r *Router) AliasInfos() []AliasInfo {
	var out []AliasInfo
	add := func(registry map[string]config.ModelRoute) {
		for alias, route := range registry {
			out = append(out, AliasInfo{ID: alias, Owner: route.Primary.Provider})
		}
	}
	add(r.models)
	add(r.images)
	add(r.audio)
	add(r.embeddings)
	return out
}
