package server

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicBaseUsesTheMirrorHost(t *testing.T) {
	t.Setenv("NABU_PUBLIC_URL", "https://gate.nabuxai.com")
	r := httptest.NewRequest(http.MethodGet, "https://gate.nabuxai.com/api/me/recharge", nil)
	r.Host = "gate.nabuxai.com"
	r.Header.Set("X-Forwarded-Host", "gate.nabuxai.ir")
	r.Header.Set("X-Forwarded-Proto", "https")
	if got := publicBase(r); got != "https://gate.nabuxai.ir" {
		t.Fatalf("public base = %q, want the mirror the browser opened", got)
	}
}

func TestPublicBaseKeepsThePinnedURLWhenTheForwardedHostIsUntrusted(t *testing.T) {
	t.Setenv("NABU_PUBLIC_URL", "https://gate.nabuxai.com")
	r := httptest.NewRequest(http.MethodGet, "http://gate.nabuxai.com/api/me/recharge", nil)
	r.Header.Set("X-Forwarded-Host", "evil.example")
	r.Header.Set("X-Forwarded-Proto", "https")
	if got := publicBase(r); got != "https://gate.nabuxai.com" {
		t.Fatalf("public base = %q, a crafted X-Forwarded-Host must not replace the pin", got)
	}
}

func TestPublicBaseRejectsAForwardedHostWithAPath(t *testing.T) {
	t.Setenv("NABU_PUBLIC_URL", "")
	r := httptest.NewRequest(http.MethodGet, "http://internal:8080/x", nil)
	r.Host = "internal:8080"
	r.Header.Set("X-Forwarded-Host", "gate.nabuxai.ir/admin")
	if got := publicBase(r); got != "http://internal:8080" {
		t.Fatalf("public base = %q, a host with a path must be ignored", got)
	}
}

func TestPublicBaseUsesTheFirstForwardedHop(t *testing.T) {
	t.Setenv("NABU_PUBLIC_URL", "")
	r := httptest.NewRequest(http.MethodGet, "http://upstream/x", nil)
	r.Host = "upstream"
	r.Header.Set("X-Forwarded-Host", "gate.nabuxai.ir, proxy.internal")
	r.Header.Set("X-Forwarded-Proto", "https, http")
	if got := publicBase(r); got != "https://gate.nabuxai.ir" {
		t.Fatalf("public base = %q", got)
	}
}

func TestPublicBaseWithoutForwardingUsesThePin(t *testing.T) {
	t.Setenv("NABU_PUBLIC_URL", "https://gate.example")
	r := httptest.NewRequest(http.MethodGet, "http://10.0.0.5:8080/x", nil)
	r.Host = "10.0.0.5:8080"
	if got := publicBase(r); got != "https://gate.example" {
		t.Fatalf("public base = %q", got)
	}
}

func TestRedirectURIFollowsTheMirror(t *testing.T) {
	t.Setenv("NABU_PUBLIC_URL", "https://gate.nabuxai.com")
	cfg := testNabuConfig()
	r := httptest.NewRequest(http.MethodGet, "https://gate.nabuxai.com/api/nabu/start", nil)
	r.Host = "gate.nabuxai.com"
	r.Header.Set("X-Forwarded-Host", "gate.nabuxai.ir")
	r.Header.Set("X-Forwarded-Proto", "https")
	if got := cfg.redirectURI(r); got != "https://gate.nabuxai.ir/api/nabu/callback" {
		t.Fatalf("redirect uri = %q", got)
	}
	cfg.RedirectURI = "https://gate.nabuxai.com/api/nabu/callback"
	if got := cfg.redirectURI(r); got != "https://gate.nabuxai.com/api/nabu/callback" {
		t.Fatalf("an explicit NABUAUTH_REDIRECT_URI must still win, got %q", got)
	}
}

func TestCoolifyHopKeepsTheMirrorOrigin(t *testing.T) {
	// What the foreign gateway sees after Coolify's nginx: the TCP peer is
	// the Iran server, Host is the upstream vhost, and the browser identity
	// lives only in the forwarded headers.
	t.Setenv("NABU_PUBLIC_URL", "https://gate.nabuxai.com")
	r := httptest.NewRequest(http.MethodGet, "https://gate.nabuxai.com/api/nabu/start", nil)
	r.Host = "gate.nabuxai.com"
	r.RemoteAddr = "203.0.113.50:443"
	r.Header.Set("X-Forwarded-Host", "gate.nabuxai.ir")
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-For", "198.51.100.20, 172.16.0.2")

	if got := publicBase(r); got != "https://gate.nabuxai.ir" {
		t.Fatalf("public base = %q, want the mirror, not the upstream Host", got)
	}
	if got := testNabuConfig().redirectURI(r); got != "https://gate.nabuxai.ir/api/nabu/callback" {
		t.Fatalf("redirect uri = %q", got)
	}
	c := browserCookie(r, http.Cookie{Name: "nabugate_session", Value: "tok", HttpOnly: true})
	if c.Domain != "" || !c.Secure {
		t.Fatalf("cookie Domain=%q Secure=%v; the browser on .ir must store a Secure host-only cookie", c.Domain, c.Secure)
	}
	if got := clientIP(r); got != "198.51.100.20" {
		t.Fatalf("client ip = %q, want the browser, not the Iran proxy", got)
	}
}

func TestBrowserCookieIsHostOnlyAndSecureBehindTheProxy(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "http://gate.nabuxai.com/api/login", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	c := browserCookie(r, http.Cookie{Name: "nabugate_session", Value: "tok", HttpOnly: true})
	if c.Domain != "" {
		t.Fatalf("Domain = %q, a mirror host would not store a cookie pinned to another site", c.Domain)
	}
	if !c.Secure {
		t.Fatal("Secure should follow X-Forwarded-Proto, not the hop to the upstream")
	}
	if c.Path != "/" {
		t.Fatalf("Path = %q", c.Path)
	}

	plain := httptest.NewRequest(http.MethodPost, "http://localhost:8080/api/login", nil)
	if browserCookie(plain, http.Cookie{Name: "nabugate_session"}).Secure {
		t.Fatal("a plain local request must not be marked Secure")
	}
	tlsReq := httptest.NewRequest(http.MethodPost, "https://gate.nabuxai.com/api/login", nil)
	tlsReq.TLS = &tls.ConnectionState{}
	if !browserCookie(tlsReq, http.Cookie{Name: "nabugate_session"}).Secure {
		t.Fatal("a TLS request must be marked Secure")
	}
}

func TestStreamResponseDisablesProxyBuffering(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer up.Close()
	ts, _ := fidelityServer(t, up.URL, nil, nil)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", strings.NewReader(
		`{"model":"nabu-fast","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("X-Accel-Buffering"); got != "no" {
		t.Fatalf("X-Accel-Buffering = %q, an edge proxy would buffer the stream", got)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-cache") {
		t.Fatalf("Cache-Control = %q", cc)
	}
}
