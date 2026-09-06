package router

import (
	"errors"
	"strings"
	"testing"

	"nabugate/internal/config"
	"nabugate/internal/provider"
)

// keyedFactory builds a different adapter per key, which is what a multi-key
// chain needs: the point is that the second credential behaves differently from
// the first, not that the provider does.
func keyedFactory(byKey map[string]provider.Adapter, order *[]string) CallerAdapterFunc {
	return func(name, apiKey string) (provider.Adapter, bool) {
		if order != nil {
			*order = append(*order, apiKey)
		}
		a, ok := byKey[apiKey]
		return a, ok
	}
}

// The whole reason for a list: one of the caller's keys is dead — revoked,
// over its cap, on a deleted project — and the request should reach the next
// one rather than falling to the gateway or failing.
func TestSecondCallerKeyServesWhenTheFirstFails(t *testing.T) {
	var gatewayCalls, deadCalls, liveCalls int
	var built []string

	r := byokRouter(
		map[string]provider.Adapter{"a": transcriber{name: "a", text: "gateway", calls: &gatewayCalls}},
		keyedFactory(map[string]provider.Adapter{
			"sk-dead": transcriber{name: "a", calls: &deadCalls, err: errors.New("401 invalid api key")},
			"sk-live": transcriber{name: "a", text: "second key", calls: &liveCalls},
		}, &built),
	)

	ctx, rec := ctxWithKeyLists("", map[string][]string{"a": {"sk-dead", "sk-live"}})
	out, err := r.Transcribe(ctx, "nabu-transcribe", provider.TranscriptionRequest{Audio: []byte("x")})
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if out.Text != "second key" {
		t.Errorf("text = %q, want the second key to have served", out.Text)
	}
	if deadCalls != 1 || liveCalls != 1 {
		t.Errorf("calls: dead=%d live=%d, want each tried once", deadCalls, liveCalls)
	}
	if gatewayCalls != 0 {
		t.Errorf("gateway credential spent %d times; the caller's second key should have covered it", gatewayCalls)
	}
	// Order matters and is the order the keys were given in. A chain that
	// reordered on its own would make "which key is my main one" unanswerable.
	if len(built) != 2 || built[0] != "sk-dead" || built[1] != "sk-live" {
		t.Errorf("built in order %v, want the keys tried as given", built)
	}
	if !rec.Caller() {
		t.Error("served by a caller key but not recorded as such; the gateway would bill for it")
	}
}

// Every key failing must fall through to the gateway, not end the chain: a
// user with three dead keys and an approved provider still has a working
// request, which is the whole promise of own-first.
func TestGatewayStillCatchesAfterEveryCallerKeyFails(t *testing.T) {
	var gatewayCalls int
	r := byokRouter(
		map[string]provider.Adapter{"a": transcriber{name: "a", text: "gateway", calls: &gatewayCalls}},
		keyedFactory(map[string]provider.Adapter{
			"k1": transcriber{name: "a", err: errors.New("401")},
			"k2": transcriber{name: "a", err: errors.New("429")},
		}, nil),
	)
	ctx, rec := ctxWithKeyLists("", map[string][]string{"a": {"k1", "k2"}})
	out, err := r.Transcribe(ctx, "nabu-transcribe", provider.TranscriptionRequest{Audio: []byte("x")})
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if out.Text != "gateway" || gatewayCalls != 1 {
		t.Errorf("text = %q, gateway calls = %d", out.Text, gatewayCalls)
	}
	if rec.Caller() {
		t.Error("recorded as caller-served though the gateway's key did the work; this would bill nothing")
	}
}

// A failure report has to distinguish the rungs. Three lines all saying "a
// failed" would describe a provider outage, when what happened is three
// different credentials being refused — a different problem with a different
// fix.
func TestEachCallerKeyIsNamedSeparatelyInFailures(t *testing.T) {
	r := New(nil, nil, nil, nil, nil,
		routeWith(config.Target{Provider: "a", Model: "whisper-1"}), nil, discardLogger())
	r.SetCallerAdapter(keyedFactory(map[string]provider.Adapter{
		"k1": transcriber{name: "a", err: errors.New("401 first")},
		"k2": transcriber{name: "a", err: errors.New("403 second")},
	}, nil))

	ctx, _ := ctxWithKeyLists(ModeOwn, map[string][]string{"a": {"k1", "k2"}})
	_, err := r.Transcribe(ctx, "nabu-transcribe", provider.TranscriptionRequest{Audio: []byte("x")})
	if err == nil {
		t.Fatal("want an error when every key fails")
	}
	msg := err.Error()
	for _, want := range []string{"your key #1", "your key #2", "401 first", "403 second"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not mention %q", msg, want)
		}
	}
}

// One key keeps the plain label. A caller with a single credential should not
// read "#1" and wonder which other key the gateway thinks it has.
func TestOneKeyIsNotNumbered(t *testing.T) {
	r := New(nil, nil, nil, nil, nil,
		routeWith(config.Target{Provider: "a", Model: "whisper-1"}), nil, discardLogger())
	r.SetCallerAdapter(keyedFactory(map[string]provider.Adapter{
		"k1": transcriber{name: "a", err: errors.New("401 only")},
	}, nil))
	ctx, _ := ctxWithKeyLists(ModeOwn, map[string][]string{"a": {"k1"}})
	_, err := r.Transcribe(ctx, "nabu-transcribe", provider.TranscriptionRequest{Audio: []byte("x")})
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), "#") {
		t.Errorf("error %q numbers a lone key", err)
	}
}
