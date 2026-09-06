package router

import (
	"context"
	"errors"
	"strings"
	"testing"

	"nabugate/internal/config"
	"nabugate/internal/provider"
)

// callerFactory stands in for the config's adapter factory, recording which key
// it was asked to build with.
func callerFactory(built *map[string]string, adapters map[string]provider.Adapter) CallerAdapterFunc {
	return func(name, apiKey string) (provider.Adapter, bool) {
		if *built == nil {
			*built = map[string]string{}
		}
		(*built)[name] = apiKey
		a, ok := adapters[name]
		return a, ok
	}
}

func byokRouter(global map[string]provider.Adapter, caller CallerAdapterFunc) *Router {
	r := New(global, nil, nil, nil, nil,
		routeWith(config.Target{Provider: "a", Model: "whisper-1"}), nil, discardLogger())
	r.SetCallerAdapter(caller)
	return r
}

func ctxWithKeys(mode string, keys map[string]string) (context.Context, *KeySource) {
	rec := NewKeySource()
	ctx := context.WithValue(context.Background(), CallerKeysCtxKey{}, CallerKeys{Keys: keys, Mode: mode})
	return context.WithValue(ctx, KeySourceCtxKey{}, rec), rec
}

// The default with keys present is own-first: the caller's credential is tried
// before the gateway's, so the caller pays their own vendor bill.
func TestCallerKeyIsTriedFirst(t *testing.T) {
	var globalCalls, ownCalls int
	var built map[string]string

	r := byokRouter(
		map[string]provider.Adapter{"a": transcriber{name: "a", text: "gateway", calls: &globalCalls}},
		callerFactory(&built, map[string]provider.Adapter{
			"a": transcriber{name: "a", text: "caller", calls: &ownCalls},
		}),
	)
	ctx, rec := ctxWithKeys("", map[string]string{"a": "sk-mine"})
	out, err := r.Transcribe(ctx, "nabu-transcribe", provider.TranscriptionRequest{Audio: []byte("x")})
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if out.Text != "caller" {
		t.Errorf("text = %q, want the caller's key to have served", out.Text)
	}
	if globalCalls != 0 {
		t.Errorf("gateway credential was spent %d times despite the caller having one", globalCalls)
	}
	if built["a"] != "sk-mine" {
		t.Errorf("adapter built with %q, want the key from the request", built["a"])
	}
	// Billing keys on this: charging here would bill the caller twice.
	if !rec.Caller() {
		t.Error("key source not recorded as the caller's; the gateway would bill for spend it did not make")
	}
}

// When the caller's key fails, the gateway's is the safety net — and the fact
// that the gateway paid must be what billing sees.
func TestCallerKeyFallsBackToGateway(t *testing.T) {
	var built map[string]string
	r := byokRouter(
		map[string]provider.Adapter{"a": transcriber{name: "a", text: "gateway"}},
		callerFactory(&built, map[string]provider.Adapter{
			"a": transcriber{name: "a", err: errors.New("401 invalid key")},
		}),
	)
	ctx, rec := ctxWithKeys(ModeOwnFirst, map[string]string{"a": "sk-bad"})
	out, err := r.Transcribe(ctx, "nabu-transcribe", provider.TranscriptionRequest{Audio: []byte("x")})
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if out.Text != "gateway" {
		t.Errorf("text = %q, want the gateway rung to have rescued it", out.Text)
	}
	if rec.Caller() {
		t.Error("recorded as caller-served though the gateway paid; that under-bills silently")
	}
}

// global-first is for a caller whose own key is the backstop, not the default.
func TestGlobalFirstPrefersGateway(t *testing.T) {
	var ownCalls int
	var built map[string]string
	r := byokRouter(
		map[string]provider.Adapter{"a": transcriber{name: "a", text: "gateway"}},
		callerFactory(&built, map[string]provider.Adapter{
			"a": transcriber{name: "a", text: "caller", calls: &ownCalls},
		}),
	)
	ctx, _ := ctxWithKeys(ModeGlobalFirst, map[string]string{"a": "sk-mine"})
	out, err := r.Transcribe(ctx, "nabu-transcribe", provider.TranscriptionRequest{Audio: []byte("x")})
	if err != nil || out.Text != "gateway" {
		t.Fatalf("out = %q, err = %v", out.Text, err)
	}
	if ownCalls != 0 {
		t.Errorf("caller's key was spent %d times though the gateway's worked", ownCalls)
	}
}

// "own" means only mine. Falling back to the gateway there would defeat the
// reason a caller asks for it.
func TestOwnModeNeverUsesGateway(t *testing.T) {
	var globalCalls int
	var built map[string]string
	r := byokRouter(
		map[string]provider.Adapter{"a": transcriber{name: "a", text: "gateway", calls: &globalCalls}},
		callerFactory(&built, map[string]provider.Adapter{
			"a": transcriber{name: "a", err: errors.New("429 quota")},
		}),
	)
	ctx, _ := ctxWithKeys(ModeOwn, map[string]string{"a": "sk-mine"})
	_, err := r.Transcribe(ctx, "nabu-transcribe", provider.TranscriptionRequest{Audio: []byte("x")})
	if err == nil {
		t.Fatal("want an error, got a transcript")
	}
	if globalCalls != 0 {
		t.Errorf("gateway credential spent %d times in own mode", globalCalls)
	}
	// The error must say whose key failed. Two rungs named "a" with no
	// distinction is not a diagnosis.
	if !strings.Contains(err.Error(), "your key") {
		t.Errorf("error does not name the credential that failed: %v", err)
	}
}

// A key for a provider this deployment does not define is refused with a reason
// rather than silently ignored.
func TestOwnModeWithoutAKeyForThatProvider(t *testing.T) {
	var built map[string]string
	r := byokRouter(
		map[string]provider.Adapter{"a": transcriber{name: "a", text: "gateway"}},
		callerFactory(&built, nil),
	)
	ctx, _ := ctxWithKeys(ModeOwn, map[string]string{"b": "sk-other"})
	_, err := r.Transcribe(ctx, "nabu-transcribe", provider.TranscriptionRequest{Audio: []byte("x")})
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "no key of yours") {
		t.Errorf("error = %v, want it to say the caller sent no key for that provider", err)
	}
}

// With no keys on the request, nothing changes: the gateway's credential serves
// and the request is billed as it always was.
func TestNoCallerKeysBehavesAsBefore(t *testing.T) {
	var built map[string]string
	r := byokRouter(
		map[string]provider.Adapter{"a": transcriber{name: "a", text: "gateway"}},
		callerFactory(&built, nil),
	)
	ctx, rec := ctxWithKeys("", nil)
	out, err := r.Transcribe(ctx, "nabu-transcribe", provider.TranscriptionRequest{Audio: []byte("x")})
	if err != nil || out.Text != "gateway" {
		t.Fatalf("out = %q, err = %v", out.Text, err)
	}
	if rec.Caller() {
		t.Error("no caller key was sent, yet the request was marked caller-served")
	}
	if len(built) != 0 {
		t.Errorf("built a caller adapter with no key present: %v", built)
	}
}

// A gateway that never wired a factory refuses caller keys with a reason.
func TestCallerKeysRefusedWhenUnsupported(t *testing.T) {
	r := New(map[string]provider.Adapter{"a": transcriber{name: "a", text: "gateway"}},
		nil, nil, nil, nil,
		routeWith(config.Target{Provider: "a", Model: "whisper-1"}), nil, discardLogger())
	ctx, _ := ctxWithKeys(ModeOwn, map[string]string{"a": "sk-mine"})
	_, err := r.Transcribe(ctx, "nabu-transcribe", provider.TranscriptionRequest{Audio: []byte("x")})
	if err == nil || !strings.Contains(err.Error(), "does not accept caller-supplied keys") {
		t.Fatalf("err = %v", err)
	}
}
