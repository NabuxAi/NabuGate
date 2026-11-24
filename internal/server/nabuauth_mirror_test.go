package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeNabuAuth is one NabuAuth instance: it redeems only the codes it issued
// and only with the secret registered on it.
func fakeNabuAuth(t *testing.T, code, secret, email string, redeemed *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			_ = r.ParseForm()
			if r.PostForm.Get("client_secret") != secret {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
				return
			}
			if r.PostForm.Get("code") != code {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"the code is unknown, expired or already used"}`))
				return
			}
			redeemed.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "at-" + code})
		case "/api/v1/user":
			if r.Header.Get("Authorization") != "Bearer at-"+code {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"sub": "u1", "email": email, "name": "Sara"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func nabuRoundTrip(t *testing.T, s *Server, mirror bool, code string) *httptest.ResponseRecorder {
	t.Helper()
	start := httptest.NewRequest(http.MethodGet, "http://gate.nabuxai.com/api/nabu", nil)
	start.Header.Set("Referer", "https://gate.nabuxai.com/panel/")
	if mirror {
		start.Header.Set(mirrorHostHeader, "gate.nabuxai.ir")
	}
	w := httptest.NewRecorder()
	s.consoleNabuStart(w, start)
	if w.Code != http.StatusFound {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}
	loc, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	state := loc.Query().Get("state")

	cb := httptest.NewRequest(http.MethodGet, "http://gate.nabuxai.com/api/nabu/callback?code="+code+"&state="+url.QueryEscape(state), nil)
	if mirror {
		cb.Header.Set(mirrorHostHeader, "gate.nabuxai.ir")
	}
	for _, c := range w.Result().Cookies() {
		cb.AddCookie(c)
	}
	out := httptest.NewRecorder()
	s.consoleNabuCallback(out, cb)
	out.Header().Set("X-Test-Authorize", loc.Scheme+"://"+loc.Host)
	return out
}

// auth.nabuxai.ir is its own NabuAuth with its own database. A sign-in that
// started on gate.nabuxai.ir got its code there, so it must be redeemed there:
// redeeming it at auth.nabuxai.com is the "code is unknown" failure that sent
// every Iran visitor back to /panel/?nabu_error=failed.
func TestNabuSignInOnTheMirrorRedeemsAtTheMirror(t *testing.T) {
	var comRedeemed, irRedeemed atomic.Int32
	com := fakeNabuAuth(t, "code-com", "com-secret", "sara@example.com", &comRedeemed)
	ir := fakeNabuAuth(t, "code-ir", "ir-secret", "sara@example.com", &irRedeemed)

	t.Setenv("NABUAUTH_URL", com.URL)
	t.Setenv("NABUAUTH_MIRROR_URL", ir.URL)
	t.Setenv("NABUAUTH_CLIENT_ID", "nabugate")
	t.Setenv("NABUAUTH_CLIENT_SECRET", "com-secret")
	t.Setenv("NABUAUTH_MIRROR_CLIENT_SECRET", "ir-secret")
	t.Setenv("NABU_CONSOLE_NABUAUTH_ADMINS", "owner@nabuxai.com")
	t.Setenv("NABUAUTH_REDIRECT_URI", "")

	s, store := continueServer(t)

	out := nabuRoundTrip(t, s, true, "code-ir")
	if got := out.Header().Get("X-Test-Authorize"); got != ir.URL {
		t.Fatalf("an Iran visitor was sent to %s, want the mirror %s", got, ir.URL)
	}
	if out.Code != http.StatusFound || out.Header().Get("Location") != "/panel/" {
		t.Fatalf("mirror callback: %d -> %s", out.Code, out.Header().Get("Location"))
	}
	if irRedeemed.Load() != 1 || comRedeemed.Load() != 0 {
		t.Fatalf("code redeemed ir=%d com=%d; want it at the instance that issued it", irRedeemed.Load(), comRedeemed.Load())
	}
	if !strings.Contains(strings.Join(out.Header().Values("Set-Cookie"), "\n"), consoleCookie+"=") {
		t.Fatal("no console session after a successful mirror sign-in")
	}
	if store.GetUser("sara@example.com") == nil {
		t.Fatal("signing in through the mirror did not make the account")
	}

	// The international host is unchanged: auth.nabuxai.com, its own secret.
	out = nabuRoundTrip(t, s, false, "code-com")
	if got := out.Header().Get("X-Test-Authorize"); got != com.URL {
		t.Fatalf("a .com visitor was sent to %s", got)
	}
	if out.Header().Get("Location") != "/panel/" || comRedeemed.Load() != 1 {
		t.Fatalf(".com callback -> %s, com redeemed %d", out.Header().Get("Location"), comRedeemed.Load())
	}
}

// Without a mirror-specific secret the main one is used on both instances —
// the expected setup, one secret registered on both.
func TestNabuMirrorCredentialsDefaultToTheMainOnes(t *testing.T) {
	cfg := testNabuConfig()
	cfg.MirrorURL = "https://auth.mirror"
	in := cfg.instance(true)
	if in.URL != "https://auth.mirror" || in.ClientID != cfg.ClientID || in.ClientSecret != cfg.ClientSecret {
		t.Fatalf("mirror instance = %+v", in)
	}
	cfg.MirrorClientSecret = "other"
	if cfg.instance(true).ClientSecret != "other" || cfg.instance(false).ClientSecret != cfg.ClientSecret {
		t.Fatal("mirror secret override not applied to the mirror only")
	}
	cfg.MirrorURL = ""
	if cfg.instance(true).URL != cfg.URL {
		t.Fatal("with no mirror configured every code is redeemed at URL")
	}
}
