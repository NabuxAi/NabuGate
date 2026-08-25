package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testNabuConfig() nabuAuthConfig {
	return nabuAuthConfig{
		URL:          "https://auth.test",
		ClientID:     "nabugate",
		ClientSecret: "a-client-secret",
		Scopes:       "openid profile email",
		Admins:       map[string]bool{"owner@nabuxai.com": true},
	}
}

// The console mints gateway tokens and holds provider secrets, so an empty
// allow-list must read as "nobody", never as "everyone".
func TestConsoleSSOStaysOffWithoutAnAllowList(t *testing.T) {
	cfg := testNabuConfig()
	if !cfg.enabled() {
		t.Fatal("a fully configured console should offer single sign-on")
	}

	noAdmins := cfg
	noAdmins.Admins = map[string]bool{}
	if noAdmins.enabled() {
		t.Fatal("an empty admin allow-list must disable console single sign-on")
	}

	noSecret := cfg
	noSecret.ClientSecret = ""
	if noSecret.enabled() {
		t.Fatal("a missing client secret must disable console single sign-on")
	}

	noID := cfg
	noID.ClientID = ""
	if noID.enabled() {
		t.Fatal("a missing client id must disable console single sign-on")
	}
}

func TestConsoleFlowCookieRoundTrip(t *testing.T) {
	cfg := testNabuConfig()
	packed, err := cfg.packFlow(consoleNabuFlow{
		State: "st", Verifier: "vf", Expires: time.Now().Add(time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	flow, ok := cfg.unpackFlow(packed)
	if !ok || flow.State != "st" || flow.Verifier != "vf" {
		t.Fatalf("round trip lost data: %+v ok=%v", flow, ok)
	}
}

func TestConsoleFlowCookieRejectsTampering(t *testing.T) {
	cfg := testNabuConfig()
	packed, err := cfg.packFlow(consoleNabuFlow{
		State: "st", Verifier: "vf", Expires: time.Now().Add(time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	payload, signature, _ := strings.Cut(packed, ".")

	// A rewritten payload keeping the old signature must not verify; otherwise a
	// visitor could set a state matching a code obtained elsewhere.
	forged, _ := json.Marshal(consoleNabuFlow{
		State: "attacker", Verifier: "vf", Expires: time.Now().Add(time.Minute).Unix(),
	})
	if _, ok := cfg.unpackFlow(base64.RawURLEncoding.EncodeToString(forged) + "." + signature); ok {
		t.Fatal("a re-written payload was accepted")
	}
	if _, ok := cfg.unpackFlow(payload + "." + strings.Repeat("A", len(signature))); ok {
		t.Fatal("a bad signature was accepted")
	}
	if _, ok := cfg.unpackFlow(payload); ok {
		t.Fatal("an unsigned cookie was accepted")
	}

	other := cfg
	other.ClientSecret = "different-secret"
	otherPacked, _ := other.packFlow(consoleNabuFlow{
		State: "st", Verifier: "vf", Expires: time.Now().Add(time.Minute).Unix(),
	})
	if _, ok := cfg.unpackFlow(otherPacked); ok {
		t.Fatal("a cookie signed with another secret was accepted")
	}
}

func TestConsoleFlowCookieExpires(t *testing.T) {
	cfg := testNabuConfig()
	packed, err := cfg.packFlow(consoleNabuFlow{
		State: "st", Verifier: "vf", Expires: time.Now().Add(-time.Second).Unix(),
	})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	if _, ok := cfg.unpackFlow(packed); ok {
		t.Fatal("an expired flow was accepted")
	}
}

func TestConsoleRedirectURIDerivedFromRequest(t *testing.T) {
	cfg := testNabuConfig()
	r := httptest.NewRequest(http.MethodGet, "http://gate.test/api/nabu", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	if got := cfg.redirectURI(r); got != "https://gate.test/api/nabu/callback" {
		t.Fatalf("redirect uri = %q", got)
	}
	cfg.RedirectURI = "https://gate.test/custom"
	if got := cfg.redirectURI(r); got != "https://gate.test/custom" {
		t.Fatalf("an explicit redirect uri must win, got %q", got)
	}
}

func TestLoadNabuAuthConfigNormalisesAdmins(t *testing.T) {
	t.Setenv("NABUAUTH_CLIENT_ID", "nabugate")
	t.Setenv("NABUAUTH_CLIENT_SECRET", "secret")
	t.Setenv("NABU_CONSOLE_NABUAUTH_ADMINS", " Owner@Nabuxai.com , , second@nabuxai.com ")

	cfg := loadNabuAuthConfig()
	// Addresses are compared lower-cased, and blank entries from a trailing
	// comma must not become an allow-list member.
	if !cfg.Admins["owner@nabuxai.com"] || !cfg.Admins["second@nabuxai.com"] {
		t.Fatalf("admins not parsed: %#v", cfg.Admins)
	}
	if len(cfg.Admins) != 2 {
		t.Fatalf("expected exactly two admins, got %#v", cfg.Admins)
	}
	if !cfg.enabled() {
		t.Fatal("config with id, secret and admins should be enabled")
	}
}

func TestConsolePrimaryDefaultsOnAndIsOptOut(t *testing.T) {
	t.Setenv("NABUAUTH_CLIENT_ID", "nabugate")
	t.Setenv("NABUAUTH_CLIENT_SECRET", "secret")
	t.Setenv("NABU_CONSOLE_NABUAUTH_ADMINS", "owner@nabuxai.com")

	// A Nabu account is the way in wherever sign-on is configured, without
	// needing another variable set to opt in.
	if cfg := loadNabuAuthConfig(); !cfg.Primary {
		t.Fatal("single sign-on should lead by default once it is configured")
	}

	// The password form stays reachable: this console is the tool for fixing a
	// broken deployment, including one where NabuAuth itself is down.
	t.Setenv("NABUAUTH_PRIMARY", "0")
	if cfg := loadNabuAuthConfig(); cfg.Primary {
		t.Fatal("NABUAUTH_PRIMARY=0 must put the password form back in front")
	}
}
