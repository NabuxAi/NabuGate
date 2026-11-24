package router

import (
	"context"
	"errors"
	"fmt"
	"time"

	"nabugate/internal/config"
	"nabugate/internal/provider"
)

// SetVideos installs the video-generation aliases. Separate from New so the
// existing call sites and their tests keep compiling.
func (r *Router) SetVideos(videos map[string]config.ModelRoute) {
	r.videos = videos
}

// VideoResult is a video job that a provider accepted.
type VideoResult struct {
	Alias    string
	Provider string
	Model    string // the upstream model the job runs on
	Job      provider.VideoJob
}

// errVideoCallerKey is why a caller-key rung is passed over for video. A job
// is finished by later polls, each its own request, and the gateway keeps no
// copy of a caller's key to poll with — so a job started on one could be
// started and never finished.
var errVideoCallerKey = errors.New("your own key cannot start a video job: finishing it needs that key on every poll, and the gateway does not keep it")

// Video resolves a video alias and starts the job on the first rung that
// accepts it. Only the start falls back: once a vendor has the job, the poll
// goes back to that vendor alone. A request the vendor calls malformed
// (provider.ErrInvalidVideoRequest) ends the chain, since it is the caller's
// to fix.
func (r *Router) Video(ctx context.Context, alias string, req provider.VideoRequest) (VideoResult, error) {
	route, ok := r.capabilityRoute(alias, r.videos)
	if !ok {
		return VideoResult{}, fmt.Errorf("unknown video alias %q", alias)
	}
	var failures targetErrors

	for i, t := range r.attempts(ctx, r.rungs(alias, route)) {
		if t.caller {
			failures.add(t.label, t.Model, errVideoCallerKey)
			continue
		}
		if t.adapter == nil {
			failures.add(t.label, t.Model, t.unavailable())
			continue
		}
		vid, ok := t.adapter.(provider.VideoAdapter)
		if !ok {
			failures.add(t.label, t.Model, fmt.Errorf("provider does not support video"))
			r.log.Warn("skip video target", "alias", alias, "provider", t.Provider, "reason", "no video support")
			continue
		}
		if !providerAllowed(ctx, t.Provider) {
			failures.add(t.label, t.Model, fmt.Errorf("provider not allowed by token policy"))
			continue
		}
		req.Model = t.Model
		req.ImageModel = t.ImageModel
		start := time.Now()
		job, err := vid.CreateVideo(ctx, req)
		attrs := []any{"capability", "video", "alias", alias, "provider", t.Provider, "model", t.Model, "attempt", i + 1, "latency_ms", time.Since(start).Milliseconds()}
		if err != nil {
			r.log.Warn("upstream failed", append(attrs, "error", err.Error())...)
			if errors.Is(err, provider.ErrInvalidVideoRequest) {
				return VideoResult{}, err
			}
			r.failed(&failures, t, err)
			continue
		}
		r.served(ctx, t)
		r.log.Info("upstream ok", append(attrs, "task", job.TaskID, "upstream_model", job.Model)...)
		model := job.Model
		if model == "" {
			model = t.Model
		}
		return VideoResult{Alias: alias, Provider: t.Provider, Model: model, Job: job}, nil
	}
	return VideoResult{}, failures.err("video", alias)
}

// VideoStatus asks the provider that started a job where it stands.
func (r *Router) VideoStatus(ctx context.Context, providerName, taskID string) (provider.VideoStatus, error) {
	adapter, ok := r.adapters[providerName]
	if !ok {
		return provider.VideoStatus{}, fmt.Errorf("provider %q is not available to poll this job", providerName)
	}
	vid, ok := adapter.(provider.VideoAdapter)
	if !ok {
		return provider.VideoStatus{}, fmt.Errorf("provider %q does not support video", providerName)
	}
	return vid.VideoStatus(ctx, taskID)
}
