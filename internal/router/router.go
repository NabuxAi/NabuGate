// Package router resolves a public model alias to upstream targets and executes
// the request against the primary provider, falling back through the configured
// list on failure. Each upstream attempt is logged for observability.
package router

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"nabugate/internal/config"
	"nabugate/internal/provider"
)

// discoveryTTL is how long a provider's live-discovered model catalogue is
// cached before the next /v1/models call refreshes it.
const discoveryTTL = 5 * time.Minute

// Router holds the live adapters and the alias routing tables (one per
// capability: chat, images, audio).
type Router struct {
	adapters      map[string]provider.Adapter
	models        map[string]config.ModelRoute
	images        map[string]config.ModelRoute
	audio         map[string]config.ModelRoute
	transcription map[string]config.ModelRoute
	embeddings    map[string]config.ModelRoute
	log           *slog.Logger

	// passthrough maps a passthrough-enabled provider name to its static model
	// catalogue. Its presence as a key is what makes "<provider>/<model>" direct
	// routing (and live discovery) legal for that provider.
	passthrough map[string][]string

	// registry maps a logical model name to the providers that can serve it.
	// A target naming a model with no provider expands through this.
	registry map[string]config.ModelEntry

	// logicalOf is the reverse index: "provider/upstreamModel" -> logical name.
	// It is what lets a pinned coordinate find the same model elsewhere.
	logicalOf map[string]string

	// catalog caches each passthrough provider's live-discovered model list.
	catMu   sync.Mutex
	catalog map[string]catalogEntry
	ttl     time.Duration
	now     func() time.Time
}

// catalogEntry is one provider's cached live-discovered model IDs.
type catalogEntry struct {
	models  []string
	fetched time.Time
}

// New builds a Router. passthrough maps each passthrough-enabled provider to its
// optional static model catalogue (nil/empty is fine); pass nil to disable
// passthrough entirely.
func New(adapters map[string]provider.Adapter, models, images, audio, embeddings, transcription map[string]config.ModelRoute, passthrough map[string][]string, log *slog.Logger) *Router {
	return &Router{
		adapters:      adapters,
		models:        models,
		images:        images,
		audio:         audio,
		embeddings:    embeddings,
		transcription: transcription,
		log:           log,
		passthrough:   passthrough,
		catalog:       make(map[string]catalogEntry),
		ttl:           discoveryTTL,
		now:           time.Now,
	}
}

// resolvePassthrough resolves a public model name of the form
// "<provider>/<upstream-model>" to a concrete target, when <provider> is a
// passthrough-enabled provider with a live adapter. The split is on the FIRST
// "/" only, so vendor-namespaced upstream IDs keep their own slashes intact
// (e.g. "parspack/openai/gpt-5.5" -> provider "parspack", model
// "openai/gpt-5.5"). ok is false for anything that is not a passthrough route.
func (r *Router) resolvePassthrough(model string) (config.Target, bool) {
	prov, upstream, found := strings.Cut(model, "/")
	if !found || prov == "" || upstream == "" {
		return config.Target{}, false
	}
	if _, ok := r.passthrough[prov]; !ok {
		return config.Target{}, false
	}
	if _, ok := r.adapters[prov]; !ok {
		return config.Target{}, false
	}
	return config.Target{Provider: prov, Model: upstream}, true
}

// SetRegistry installs the logical-model registry and builds the reverse index
// used to recover a logical model from a concrete provider coordinate.
// Separate from New so the existing call sites and their tests keep working.
func (r *Router) SetRegistry(reg map[string]config.ModelEntry) {
	r.registry = reg
	r.logicalOf = make(map[string]string, len(reg))
	for name, entry := range reg {
		for _, sv := range entry.Serves {
			r.logicalOf[sv.Provider+"/"+sv.Model] = name
		}
	}
}

// siblings returns the other providers serving the same logical model as t,
// in registry order, excluding t's own provider.
//
// This is what keeps a retry on the model the caller actually asked for. A
// request naming "parspack/openai/gpt-5.5" pins a provider, but the caller
// wanted gpt-5.5 — so when Parspack fails, the same model on AvalAI is the
// correct next attempt, and a different model would not be.
func (r *Router) siblings(t config.Target) []config.Target {
	name, ok := r.logicalOf[t.Provider+"/"+t.Model]
	if !ok {
		return nil
	}
	entry := r.registry[name]

	out := make([]config.Target, 0, len(entry.Serves))
	for _, sv := range entry.Serves {
		if sv.Provider == t.Provider {
			continue
		}
		if _, live := r.adapters[sv.Provider]; !live {
			continue
		}
		style := entry.ParamStyle
		if sv.ParamStyle != "" {
			style = sv.ParamStyle
		}
		out = append(out, config.Target{Provider: sv.Provider, Model: sv.Model, ParamStyle: style})
	}
	return out
}

// expand turns one configured target into the concrete attempts it stands for.
//
// A target with a provider is already concrete. A target naming only a model
// expands into one attempt per provider that serves it, in the registry's
// order — which is what makes a provider outage invisible: the caller named a
// model, and the next provider serving that same model is tried automatically.
//
// Providers with no live adapter are dropped here rather than attempted, so a
// registry listing five providers on a gateway keyed for two costs nothing.
func (r *Router) expand(t config.Target) []config.Target {
	if t.Provider != "" {
		return []config.Target{t}
	}
	entry, ok := r.registry[t.Model]
	if !ok {
		return nil
	}

	out := make([]config.Target, 0, len(entry.Serves))
	for _, s := range entry.Serves {
		if _, live := r.adapters[s.Provider]; !live {
			continue
		}
		// Most specific wins: the serving entry knows about provider-specific
		// wrapping, the registry entry knows the model, the target is the
		// caller's explicit override.
		style := entry.ParamStyle
		if s.ParamStyle != "" {
			style = s.ParamStyle
		}
		if t.ParamStyle != "" {
			style = t.ParamStyle
		}
		out = append(out, config.Target{Provider: s.Provider, Model: s.Model, ParamStyle: style})
	}
	return out
}

// resolveChatTargets returns the ordered upstream targets for a public chat
// model name: a configured alias expands to its primary + fallbacks, an
// unknown "<provider>/<model>" name resolves to a direct passthrough target,
// and a bare registry model name resolves to every provider serving it.
func (r *Router) resolveChatTargets(model string) ([]config.Target, bool) {
	if route, ok := r.models[model]; ok {
		var out []config.Target
		for _, t := range append([]config.Target{route.Primary}, route.Fallback...) {
			out = append(out, r.expand(t)...)
		}
		return out, len(out) > 0
	}
	if t, ok := r.resolvePassthrough(model); ok {
		// Pinning a provider still asks for a model. If that model is in the
		// registry, the other providers serving it are legitimate retries —
		// same model, different provider.
		if style, known := r.logicalOf[t.Provider+"/"+t.Model]; known {
			entry := r.registry[style]
			if t.ParamStyle == "" {
				t.ParamStyle = entry.ParamStyle
			}
		}
		return append([]config.Target{t}, r.siblings(t)...), true
	}
	// A model named directly, e.g. {"model": "gpt-5.5"}: try every provider
	// that serves it.
	if out := r.expand(config.Target{Model: model}); len(out) > 0 {
		return out, true
	}
	return nil, false
}

// Result is the outcome of a successful routed call.
type Result struct {
	Alias    string
	Provider string
	Model    string
	Response provider.ChatResponse
}

// KnowsAlias reports whether a name resolves to something this router can chat
// with — a configured alias or a passthrough "<provider>/<model>".
//
// Exists so a caller can refuse a bad name at the moment somebody writes it
// down, rather than at the moment somebody else calls it.
func (r *Router) KnowsAlias(name string) bool {
	_, ok := r.resolveChatTargets(name)
	return ok
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
	targets, ok := r.resolveChatTargets(alias)
	if !ok {
		return Result{}, fmt.Errorf("unknown model alias %q", alias)
	}
	var failures targetErrors

	for i, t := range targets {
		adapter, ok := r.adapters[t.Provider]
		if !ok {
			failures.add(t.Provider, t.Model, fmt.Errorf("provider not available (is its API key set?)"))
			r.log.Warn("skip target", "alias", alias, "provider", t.Provider, "model", t.Model, "reason", "provider unavailable")
			continue
		}

		req.Model = t.Model
		req.ParamStyle = t.ParamStyle
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
			failures.add(t.Provider, t.Model, err)
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

	return Result{}, failures.err("model", alias)
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
	targets, ok := r.resolveChatTargets(alias)
	if !ok {
		return StreamResult{}, fmt.Errorf("unknown model alias %q", alias)
	}
	var failures targetErrors

	for i, t := range targets {
		adapter, ok := r.adapters[t.Provider]
		if !ok {
			failures.add(t.Provider, t.Model, fmt.Errorf("provider not available (is its API key set?)"))
			continue
		}
		streamer, ok := adapter.(provider.StreamAdapter)
		if !ok {
			failures.add(t.Provider, t.Model, fmt.Errorf("provider does not support streaming"))
			r.log.Warn("skip stream target", "alias", alias, "provider", t.Provider, "reason", "no stream support")
			continue
		}

		onMeta(t.Provider, t.Model)
		req.Model = t.Model
		req.ParamStyle = t.ParamStyle
		started := false
		start := time.Now()
		usage, err := streamer.ChatStream(ctx, req, func(delta string) error {
			started = true
			return onDelta(delta)
		})
		attrs := []any{"capability", "chat-stream", "alias", alias, "provider", t.Provider, "model", t.Model, "attempt", i + 1, "latency_ms", time.Since(start).Milliseconds()}
		if err != nil {
			failures.add(t.Provider, t.Model, err)
			r.log.Warn("upstream failed", append(attrs, "error", err.Error(), "started", started)...)
			if started {
				// Cannot fall back once the client has received bytes.
				return StreamResult{Provider: t.Provider, Model: t.Model, Usage: usage}, err
			}
			continue
		}
		// A clean close having emitted nothing is a failure, not a success.
		// Some upstreams answer a stream with a role delta and a stop and no
		// content at all, while the same model returns text non-streaming.
		// Treating that as success ends the chain on an empty response and the
		// remaining targets are never tried. Nothing reached the client yet —
		// that is what `started` guarantees — so falling back is safe.
		if !started {
			failures.add(t.Provider, t.Model, fmt.Errorf("stream produced no content"))
			r.log.Warn("upstream produced an empty stream", attrs...)
			continue
		}

		r.log.Info("upstream ok", append(attrs, "total_tokens", usage.TotalTokens)...)
		return StreamResult{Provider: t.Provider, Model: t.Model, Usage: usage}, nil
	}
	return StreamResult{}, failures.err("model", alias)
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
	var failures targetErrors

	for i, t := range targets {
		adapter, ok := r.adapters[t.Provider]
		if !ok {
			failures.add(t.Provider, t.Model, fmt.Errorf("provider not available (is its API key set?)"))
			continue
		}
		imgAdapter, ok := adapter.(provider.ImageAdapter)
		if !ok {
			failures.add(t.Provider, t.Model, fmt.Errorf("provider does not support images"))
			r.log.Warn("skip image target", "alias", alias, "provider", t.Provider, "reason", "no image support")
			continue
		}

		req.Model = t.Model
		start := time.Now()
		resp, err := imgAdapter.Image(ctx, req)
		attrs := []any{"capability", "image", "alias", alias, "provider", t.Provider, "model", t.Model, "attempt", i + 1, "latency_ms", time.Since(start).Milliseconds()}
		if err != nil {
			failures.add(t.Provider, t.Model, err)
			r.log.Warn("upstream failed", append(attrs, "error", err.Error())...)
			continue
		}
		r.log.Info("upstream ok", append(attrs, "images", len(resp.Images))...)
		return ImageResult{Alias: alias, Provider: t.Provider, Model: t.Model, Images: resp.Images}, nil
	}
	return ImageResult{}, failures.err("image", alias)
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
	var failures targetErrors

	for i, t := range targets {
		adapter, ok := r.adapters[t.Provider]
		if !ok {
			failures.add(t.Provider, t.Model, fmt.Errorf("provider not available (is its API key set?)"))
			continue
		}
		spAdapter, ok := adapter.(provider.SpeechAdapter)
		if !ok {
			failures.add(t.Provider, t.Model, fmt.Errorf("provider does not support speech"))
			r.log.Warn("skip audio target", "alias", alias, "provider", t.Provider, "reason", "no speech support")
			continue
		}

		req.Model = t.Model
		start := time.Now()
		resp, err := spAdapter.Speech(ctx, req)
		attrs := []any{"capability", "speech", "alias", alias, "provider", t.Provider, "model", t.Model, "attempt", i + 1, "latency_ms", time.Since(start).Milliseconds()}
		if err != nil {
			failures.add(t.Provider, t.Model, err)
			r.log.Warn("upstream failed", append(attrs, "error", err.Error())...)
			continue
		}
		r.log.Info("upstream ok", append(attrs, "bytes", len(resp.Audio))...)
		return SpeechResult{Alias: alias, Provider: t.Provider, Model: t.Model, Audio: resp.Audio, ContentType: resp.ContentType}, nil
	}
	return SpeechResult{}, failures.err("audio", alias)
}

// targetErrors accumulates why each rung of a fallback chain failed.
//
// The loops below used to keep only the last error, so a chain whose FIRST rung
// failed for the interesting reason — no API key, a retired model name —
// reported whatever the last rung happened to say instead. Debugging
// "all targets failed … provider \"cloudflare\" not available" tells you nothing
// about the primary that actually broke.
type targetErrors []string

func (t *targetErrors) add(provider, model string, err error) {
	*t = append(*t, fmt.Sprintf("%s/%s: %v", provider, model, err))
}

// err renders the whole chain, or nil when nothing was recorded.
func (t targetErrors) err(kind, alias string) error {
	if len(t) == 0 {
		return fmt.Errorf("all targets failed for %s alias %q: no targets configured", kind, alias)
	}

	return fmt.Errorf("all targets failed for %s alias %q: %s", kind, alias, strings.Join(t, "; "))
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
	var failures targetErrors

	for i, t := range targets {
		adapter, ok := r.adapters[t.Provider]
		if !ok {
			// Almost always an unset API key: a provider whose key is missing is
			// skipped at start-up, so it never reaches the adapter map.
			failures.add(t.Provider, t.Model, fmt.Errorf("provider not available (is its API key set?)"))
			continue
		}
		embAdapter, ok := adapter.(provider.EmbeddingAdapter)
		if !ok {
			failures.add(t.Provider, t.Model, fmt.Errorf("provider does not support embeddings"))
			r.log.Warn("skip embedding target", "alias", alias, "provider", t.Provider, "reason", "no embedding support")
			continue
		}

		req.Model = t.Model
		start := time.Now()
		resp, err := embAdapter.Embed(ctx, req)
		attrs := []any{"capability", "embedding", "alias", alias, "provider", t.Provider, "model", t.Model, "attempt", i + 1, "latency_ms", time.Since(start).Milliseconds()}
		if err != nil {
			failures.add(t.Provider, t.Model, err)
			r.log.Warn("upstream failed", append(attrs, "error", err.Error())...)
			continue
		}
		r.log.Info("upstream ok", append(attrs, "vectors", len(resp.Embeddings), "total_tokens", resp.Usage.TotalTokens)...)
		return EmbedResult{Alias: alias, Provider: t.Provider, Model: t.Model, Embeddings: resp.Embeddings, Usage: resp.Usage}, nil
	}
	return EmbedResult{}, failures.err("embedding", alias)
}

// AliasInfo describes one public alias and the provider that primarily serves it.
type AliasInfo struct {
	ID    string
	Owner string
}

// AliasInfos returns every configured alias across all capabilities.
// firstReachableProvider returns the provider that would serve this route today
// — the first rung whose adapter exists — and whether any rung does at all.
//
// A rung with an EMPTY provider is not a missing provider: per config.Target,
// it names an entry in the model registry that the router expands into one
// target per serving provider at resolve time. Whether any of those is
// available cannot be answered here, so such a rung counts as reachable and the
// alias stays listed. Treating it as unavailable hid nabu-fast, nabu-smart and
// nabu-cheap — three aliases that work — the first time this filter shipped.
func (r *Router) firstReachableProvider(route config.ModelRoute) (string, bool) {
	for _, t := range append([]config.Target{route.Primary}, route.Fallback...) {
		if t.Provider == "" {
			return "", true
		}
		if _, ok := r.adapters[t.Provider]; ok {
			return t.Provider, true
		}
	}

	return "", false
}

func (r *Router) AliasInfos() []AliasInfo {
	var out []AliasInfo
	add := func(registry map[string]config.ModelRoute) {
		for alias, route := range registry {
			// Only aliases something can actually serve.
			//
			// A provider whose API key is unset is skipped at start-up and never
			// reaches the adapter map, so an alias whose every rung names such a
			// provider is configuration for a deployment this is not. Listing it
			// anyway offers callers a model that fails every request — and one
			// consumer presents this catalogue directly as its users' model
			// picker, so those become options a person can choose and cannot use.
			owner, ok := r.firstReachableProvider(route)
			if !ok {
				continue
			}
			// The provider that will actually serve it, not the configured
			// primary: when the primary is unavailable and a fallback carries the
			// traffic, naming the primary describes a route nothing takes.
			out = append(out, AliasInfo{ID: alias, Owner: owner})
		}
	}
	add(r.models)
	add(r.images)
	add(r.audio)
	add(r.embeddings)
	return out
}

// CatalogModels returns the passthrough providers' catalogues as public model
// IDs of the form "<provider>/<upstream-id>". For each passthrough provider it
// lists its statically configured models plus, when the provider's adapter can
// enumerate them, the models discovered live from the provider's own
// /v1/models (cached for r.ttl). Discovery failures are logged and fall back to
// the last good (or static-only) list so /v1/models keeps responding.
func (r *Router) CatalogModels(ctx context.Context) []AliasInfo {
	var out []AliasInfo
	seen := make(map[string]bool)
	for prov, static := range r.passthrough {
		ids := make([]string, 0, len(static))
		ids = append(ids, static...)
		ids = append(ids, r.discover(ctx, prov)...)
		for _, id := range ids {
			full := prov + "/" + id
			if seen[full] {
				continue
			}
			seen[full] = true
			out = append(out, AliasInfo{ID: full, Owner: prov})
		}
	}
	return out
}

// discover returns a passthrough provider's live model IDs, using a cached
// result while it is fresh. On a discovery error it returns the last cached
// list (possibly stale) if one exists, else nil — never an error, so a single
// unreachable provider cannot break the whole /v1/models response.
func (r *Router) discover(ctx context.Context, prov string) []string {
	adapter, ok := r.adapters[prov]
	if !ok {
		return nil
	}
	lister, ok := adapter.(provider.ModelLister)
	if !ok {
		return nil // provider cannot enumerate models (e.g. non-OpenAI-wire)
	}

	r.catMu.Lock()
	if entry, ok := r.catalog[prov]; ok && r.now().Sub(entry.fetched) < r.ttl {
		models := entry.models
		r.catMu.Unlock()
		return models
	}
	r.catMu.Unlock()

	models, err := lister.ListModels(ctx)
	if err != nil {
		r.log.Warn("model discovery failed", "provider", prov, "error", err.Error())
		r.catMu.Lock()
		defer r.catMu.Unlock()
		if entry, ok := r.catalog[prov]; ok {
			return entry.models // serve the last good list rather than nothing
		}
		return nil
	}

	r.catMu.Lock()
	r.catalog[prov] = catalogEntry{models: models, fetched: r.now()}
	r.catMu.Unlock()
	return models
}

// Responses resolves a chat model name (alias or "<provider>/<model>"
// passthrough) and proxies an OpenAI Responses API call to the first
// Responses-capable (OpenAI-wire) target, rewriting only "model". It returns
// the raw upstream *http.Response for the caller to stream straight back
// (JSON or SSE); the caller MUST close its Body. Fallback to the next target
// happens only on a transport error, before any bytes are read — an upstream
// HTTP error status is passed through transparently.
func (r *Router) Responses(ctx context.Context, model string, body map[string]json.RawMessage) (*http.Response, string, string, error) {
	targets, ok := r.resolveChatTargets(model)
	if !ok {
		return nil, "", "", fmt.Errorf("unknown model alias %q", model)
	}
	var failures targetErrors
	for _, t := range targets {
		adapter, ok := r.adapters[t.Provider]
		if !ok {
			failures.add(t.Provider, t.Model, fmt.Errorf("provider not available (is its API key set?)"))
			continue
		}
		responder, ok := adapter.(provider.ResponsesAdapter)
		if !ok {
			failures.add(t.Provider, t.Model, fmt.Errorf("provider does not support the Responses API"))
			r.log.Warn("skip responses target", "model", model, "provider", t.Provider, "reason", "no responses support")
			continue
		}

		body["model"], _ = json.Marshal(t.Model)
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, "", "", err
		}
		start := time.Now()
		resp, err := responder.Responses(ctx, raw)
		attrs := []any{"capability", "responses", "model", model, "provider", t.Provider, "upstream_model", t.Model, "latency_ms", time.Since(start).Milliseconds()}
		if err != nil {
			failures.add(t.Provider, t.Model, err)
			r.log.Warn("upstream failed", append(attrs, "error", err.Error())...)
			continue
		}
		r.log.Info("upstream ok", append(attrs, "status", resp.StatusCode)...)
		return resp, t.Provider, t.Model, nil
	}
	return nil, "", "", failures.err("model", model)
}

// ProviderNames returns the providers that actually came up, sorted. A provider
// whose key env was empty is skipped at startup and does not appear here — which
// is the distinction the console needs: configured is not the same as running.
func (r *Router) ProviderNames() []string {
	out := make([]string, 0, len(r.adapters))
	for name := range r.adapters {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// IsPassthrough reports whether a provider accepts "<provider>/<model>" routing.
func (r *Router) IsPassthrough(name string) bool {
	_, ok := r.passthrough[name]
	return ok
}

// TranscribeResult is the outcome of a successful transcription.
type TranscribeResult struct {
	Alias    string
	Provider string
	Model    string
	Text     string
	Language string
	Duration float64
	Segments []provider.TranscriptionSegment
	Usage    provider.Usage
}

// Transcribe resolves a transcription alias and tries primary then fallbacks.
//
// The audio is held in memory across attempts on purpose: a fallback that had
// to re-read a consumed stream could not retry at all, and re-uploading a few
// megabytes is the cheap half of this call.
func (r *Router) Transcribe(ctx context.Context, alias string, req provider.TranscriptionRequest) (TranscribeResult, error) {
	route, ok := r.transcription[alias]
	if !ok {
		return TranscribeResult{}, fmt.Errorf("unknown transcription alias %q", alias)
	}
	targets := append([]config.Target{route.Primary}, route.Fallback...)
	var failures targetErrors

	for i, t := range targets {
		adapter, ok := r.adapters[t.Provider]
		if !ok {
			failures.add(t.Provider, t.Model, fmt.Errorf("provider not available (is its API key set?)"))
			continue
		}
		trAdapter, ok := adapter.(provider.TranscriptionAdapter)
		if !ok {
			failures.add(t.Provider, t.Model, fmt.Errorf("provider does not support transcription"))
			r.log.Warn("skip transcription target", "alias", alias, "provider", t.Provider, "reason", "no transcription support")
			continue
		}

		req.Model = t.Model
		start := time.Now()
		resp, err := trAdapter.Transcribe(ctx, req)
		attrs := []any{"capability", "transcription", "alias", alias, "provider", t.Provider, "model", t.Model, "attempt", i + 1, "latency_ms", time.Since(start).Milliseconds()}
		if err != nil {
			failures.add(t.Provider, t.Model, err)
			r.log.Warn("upstream failed", append(attrs, "error", err.Error())...)
			continue
		}
		r.log.Info("upstream ok", append(attrs, "chars", len(resp.Text), "segments", len(resp.Segments), "audio_seconds", resp.Duration)...)
		return TranscribeResult{
			Alias: alias, Provider: t.Provider, Model: t.Model,
			Text: resp.Text, Language: resp.Language, Duration: resp.Duration,
			Segments: resp.Segments, Usage: resp.Usage,
		}, nil
	}
	return TranscribeResult{}, failures.err("transcription", alias)
}
