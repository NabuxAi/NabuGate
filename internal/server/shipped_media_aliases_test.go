package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"nabugate/internal/agent"
	"nabugate/internal/config"
	"nabugate/internal/policy"
	"nabugate/internal/router"
	"nabugate/internal/usage"
)

// The shipped config, keyed for DashScope alone, lists the new image and video
// aliases on /v1/models and reports each on /v1/health like any other alias:
// kind, live targets, providers.
func TestShippedMediaAliasesListAndReportHealth(t *testing.T) {
	t.Setenv("DASHSCOPE_API_KEY", "sk-test")
	cfg, err := config.Load("../../config.default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	adapters, _ := cfg.BuildAdapters()
	r := router.New(adapters, cfg.Models, cfg.Images, cfg.Audio, cfg.Embeddings, cfg.Transcription, cfg.Passthroughs(adapters), discardLogger())
	r.SetRegistry(cfg.Registry)
	r.SetVideos(cfg.Videos)
	ts := httptest.NewServer(New(r, policy.New(nil, nil), usage.New(cfg.Pricing), agent.NewRegistry(), discardLogger()).Handler())
	defer ts.Close()

	want := map[string]struct{ kind, provider string }{
		"nabu-video":       {"video", "dashscope-video"},
		"nabu-video-wan25": {"video", "dashscope-video"},
		"nabu-video-wan22": {"video", "dashscope-video"},
		"nabu-video-fast":  {"video", "dashscope-video"},
		"nabu-video-wan21": {"video", "dashscope-video"},
		"nabu-wan25":       {"image", "dashscope-image"},
		"nabu-wan22":       {"image", "dashscope-image"},
		"nabu-wan-flash":   {"image", "dashscope-image"},
	}

	listed := map[string]string{}
	models := videoCall(t, ts, http.MethodGet, "/v1/models", "", "")
	for _, m := range models.body["data"].([]any) {
		e := m.(map[string]any)
		listed[e["id"].(string)], _ = e["owned_by"].(string)
	}
	health := videoCall(t, ts, http.MethodGet, "/v1/health", "", "")
	reported := map[string]map[string]any{}
	for _, a := range health.body["aliases"].([]any) {
		e := a.(map[string]any)
		reported[e["id"].(string)] = e
	}

	for alias, w := range want {
		if owner, ok := listed[alias]; !ok || owner != w.provider {
			t.Errorf("/v1/models: %s listed %v, owned by %q; want %q", alias, ok, owner, w.provider)
		}
		h, ok := reported[alias]
		if !ok {
			t.Errorf("/v1/health does not report %s", alias)
			continue
		}
		providers, _ := h["providers"].([]any)
		if h["kind"] != w.kind || h["live_targets"] != float64(1) || len(providers) != 1 || providers[0] != w.provider {
			t.Errorf("/v1/health %s = %v; want kind %s, one live target on %s", alias, h, w.kind, w.provider)
		}
	}
}
