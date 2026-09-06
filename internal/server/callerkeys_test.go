package server

import (
	"net/http/httptest"
	"testing"

	"nabugate/internal/router"
)

func TestCallerKeysFromHeaders(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/audio/transcriptions", nil)
	r.Header.Set("X-Nabu-Key-Gemini", "  AIza-secret  ")
	r.Header.Set("X-Nabu-Key-SPEECHMATICS", "sm-secret")
	r.Header.Set("X-Nabu-Key-Empty", "   ")
	r.Header.Set("X-Nabu-Key-Mode", "  Own-First ")
	r.Header.Set("Authorization", "Bearer nabu-token")

	got := callerKeysFrom(r)
	if got.Mode != router.ModeOwnFirst {
		t.Errorf("mode = %q", got.Mode)
	}
	// Provider names are matched case-insensitively against the config, so the
	// header casing a client happens to send must not decide whether its key
	// is found.
	if got.Keys["gemini"] != "AIza-secret" {
		t.Errorf("gemini key = %q", got.Keys["gemini"])
	}
	if got.Keys["speechmatics"] != "sm-secret" {
		t.Errorf("speechmatics key = %q", got.Keys["speechmatics"])
	}
	if _, ok := got.Keys["empty"]; ok {
		t.Error("a blank header became a key; it would be tried and fail")
	}
	if _, ok := got.Keys["mode"]; ok {
		t.Error("the mode header was read as a provider key")
	}
}

// No caller headers must leave behaviour exactly as it was.
func TestCallerKeysAbsent(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	got := callerKeysFrom(r)
	if len(got.Keys) != 0 {
		t.Errorf("keys = %v", got.Keys)
	}
	if got.Mode != router.ModeGlobal {
		t.Errorf("mode = %q, want the gateway's own credentials", got.Mode)
	}
}

// An unrecognised mode must not fail the request; it degrades to the default
// for whether keys were sent.
func TestCallerKeysUnknownMode(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	r.Header.Set("X-Nabu-Key-Mode", "sideways")
	r.Header.Set("X-Nabu-Key-Openai", "sk-1")
	if got := callerKeysFrom(r); got.Mode != router.ModeOwnFirst {
		t.Errorf("mode = %q, want own-first when keys are present", got.Mode)
	}

	r2 := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	r2.Header.Set("X-Nabu-Key-Mode", "sideways")
	if got := callerKeysFrom(r2); got.Mode != router.ModeGlobal {
		t.Errorf("mode = %q, want global with no keys", got.Mode)
	}
}

// servedByCaller is what zeroes the bill; a request with no recorder installed
// must never look caller-served.
func TestServedByCaller(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	if servedByCaller(r.Context()) {
		t.Error("a request with no recorder reported as caller-served")
	}

	ctx, rec := withCallerKeys(r.Context(), r)
	if servedByCaller(ctx) {
		t.Error("caller-served before anything ran")
	}
	rec.Set("gateway")
	if servedByCaller(ctx) {
		t.Error("the gateway paid, yet the request is exempt from billing")
	}

	ctx2, rec2 := withCallerKeys(r.Context(), r)
	rec2.Set("caller")
	if !servedByCaller(ctx2) {
		t.Error("caller-served not reported; the gateway would bill for the caller's own spend")
	}
}
