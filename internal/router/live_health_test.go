package router

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nabugate/internal/config"
	"nabugate/internal/provider"
)

func liveRouter(upstream string) *Router {
	adapters := map[string]provider.Adapter{"openai": provider.NewOpenAIAdapter("openai", upstream, "k", nil)}
	r := New(adapters, nil, nil, nil, nil, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r.SetLive(map[string]config.ModelRoute{"nabu-live": {Primary: config.Target{Provider: "openai", Model: "gpt-live-1"}}})
	return r
}

func liveHealthOf(t *testing.T, r *Router) AliasHealth {
	t.Helper()
	for _, h := range r.AliasHealthAll() {
		if h.ID == "nabu-live" && h.Kind == "live" {
			return h
		}
	}
	t.Fatal("nabu-live missing from AliasHealthAll")
	return AliasHealth{}
}

func listed(r *Router, id string) bool {
	for _, a := range r.AliasInfos() {
		if a.ID == id {
			return true
		}
	}
	return false
}

func TestARefusedLiveAliasIsNeverSignalled(t *testing.T) {
	hits := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"session":{"id":"x"}}`))
	}))
	defer up.Close()

	r := liveRouter(up.URL)
	r.SetLiveProblems(map[string]string{"nabu-live": "no per_minute price for openai/gpt-live-1"})

	_, err := r.LiveSession(context.Background(), "nabu-live", []byte(`{}`))
	var refused *LiveMisconfiguredError
	if !errors.As(err, &refused) || !strings.Contains(err.Error(), "live alias misconfigured") {
		t.Fatalf("err = %v", err)
	}
	if hits != 0 {
		t.Fatalf("a refused alias reached the vendor %d times", hits)
	}
}

func TestLiveAliasesAreListedAndHealthChecked(t *testing.T) {
	r := liveRouter("http://vendor.invalid")

	if !listed(r, "nabu-live") {
		t.Fatal("a servable live alias is missing from AliasInfos")
	}
	h := liveHealthOf(t, r)
	if h.Live != 1 || h.Disabled || !h.Healthy() || len(h.Warnings) != 0 {
		t.Fatalf("health = %+v", h)
	}

	r.SetLiveProblems(map[string]string{"nabu-live": "no per_minute price for openai/gpt-live-1"})
	if listed(r, "nabu-live") {
		t.Fatal("a refused live alias is still offered in AliasInfos")
	}
	h = liveHealthOf(t, r)
	if !h.Disabled || h.Healthy() || len(h.Warnings) == 0 || !strings.HasPrefix(h.Warnings[len(h.Warnings)-1], "refused:") {
		t.Fatalf("health = %+v", h)
	}
}

func TestLiveHealthCarriesTheLastUpstreamAnswer(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Incorrect API key provided"}}`))
	}))
	defer up.Close()
	r := liveRouter(up.URL)

	_, err := r.LiveSession(context.Background(), "nabu-live", []byte(`{}`))
	var refusal *provider.LiveUpstreamError
	if !errors.As(err, &refusal) || refusal.Status != http.StatusUnauthorized {
		t.Fatalf("the vendor's refusal is lost in the chain error: %v", err)
	}

	h := liveHealthOf(t, r)
	if h.LastErrorAt == nil || !strings.Contains(h.LastError, "Incorrect API key provided") {
		t.Fatalf("health = %+v", h)
	}
	if !strings.Contains(strings.Join(h.Warnings, " | "), "the last session request failed") {
		t.Fatalf("warnings = %v", h.Warnings)
	}
}
