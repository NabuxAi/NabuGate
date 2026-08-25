package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Console single sign-on with NabuAuth, the ecosystem's identity server.
//
// The console holds provider secrets and mints gateway tokens, so it is not an
// end-user surface: signing in with any Nabu account would be wrong. Access is
// limited to an explicit allow-list of email addresses
// (NABU_CONSOLE_NABUAUTH_ADMINS). Without that list the button stays hidden and
// the endpoints refuse, because an empty list would otherwise read as "everyone".
//
// The flow is authorization code with PKCE. The verifier stays server-side and
// only its SHA-256 hash goes in the redirect, so a code intercepted on the way
// back cannot be exchanged by anyone else.

const (
	consoleNabuFlowCookie = "nabu_console_flow"

	// consoleNabuFlowTTL bounds a started sign-in: long enough to type a
	// password, short enough that an abandoned flow cannot be resumed later.
	consoleNabuFlowTTL = 10 * time.Minute
)

// nabuAuthConfig is the console's view of NabuAuth, read from the environment
// so it needs no config-file change to enable.
type nabuAuthConfig struct {
	URL          string
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Scopes       string
	// Admins is the set of NabuAuth account emails allowed into the console.
	Admins map[string]bool
	// Primary means the console presents a Nabu account as the way in rather
	// than one option beside the password form. The form stays reachable, since
	// this console is also the tool for fixing a broken deployment — including
	// one where NabuAuth itself is down.
	Primary bool
}

func loadNabuAuthConfig() nabuAuthConfig {
	admins := map[string]bool{}
	for _, entry := range strings.Split(os.Getenv("NABU_CONSOLE_NABUAUTH_ADMINS"), ",") {
		if email := strings.ToLower(strings.TrimSpace(entry)); email != "" {
			admins[email] = true
		}
	}
	return nabuAuthConfig{
		URL:          strings.TrimRight(envOrDefault("NABUAUTH_URL", "https://auth.nabuxai.com"), "/"),
		ClientID:     os.Getenv("NABUAUTH_CLIENT_ID"),
		ClientSecret: os.Getenv("NABUAUTH_CLIENT_SECRET"),
		RedirectURI:  os.Getenv("NABUAUTH_REDIRECT_URI"),
		Scopes:       envOrDefault("NABUAUTH_SCOPES", "openid profile email"),
		Admins:       admins,
		Primary:      os.Getenv("NABUAUTH_PRIMARY") != "0",
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// enabled reports whether console sign-on is usable. All three are required:
// the secret also signs the flow cookie, and an empty allow-list would let any
// Nabu account into a console that hands out gateway tokens.
func (c nabuAuthConfig) enabled() bool {
	return c.ClientID != "" && c.ClientSecret != "" && len(c.Admins) > 0
}

func (c nabuAuthConfig) redirectURI(r *http.Request) string {
	if c.RedirectURI != "" {
		return c.RedirectURI
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/api/nabu/callback", scheme, r.Host)
}

func b64u(raw []byte) string { return base64.RawURLEncoding.EncodeToString(raw) }

// consoleNabuFlow is the state carried across the redirect and back.
type consoleNabuFlow struct {
	State    string `json:"s"`
	Verifier string `json:"v"`
	Expires  int64  `json:"e"`
	ReturnTo string `json:"r"`
}

// signFlow authenticates the flow cookie. Without a signature a visitor could
// forge a state to match a code obtained elsewhere.
func (c nabuAuthConfig) signFlow(payload string) string {
	mac := hmac.New(sha256.New, []byte(c.ClientSecret))
	mac.Write([]byte(payload))
	return b64u(mac.Sum(nil))
}

func (c nabuAuthConfig) packFlow(f consoleNabuFlow) (string, error) {
	raw, err := json.Marshal(f)
	if err != nil {
		return "", err
	}
	payload := b64u(raw)
	return payload + "." + c.signFlow(payload), nil
}

// unpackFlow returns the flow only when the cookie is intact, correctly signed
// and unexpired.
func (c nabuAuthConfig) unpackFlow(value string) (consoleNabuFlow, bool) {
	payload, signature, ok := strings.Cut(value, ".")
	if !ok || payload == "" || signature == "" {
		return consoleNabuFlow{}, false
	}
	if !hmac.Equal([]byte(c.signFlow(payload)), []byte(signature)) {
		return consoleNabuFlow{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return consoleNabuFlow{}, false
	}
	var f consoleNabuFlow
	if err := json.Unmarshal(raw, &f); err != nil {
		return consoleNabuFlow{}, false
	}
	if f.State == "" || f.Verifier == "" || time.Now().Unix() > f.Expires {
		return consoleNabuFlow{}, false
	}
	return f, true
}

// consoleNabuStatus tells the console UI whether single sign-on exists and
// whether it leads.
func (s *Server) consoleNabuStatus(w http.ResponseWriter, _ *http.Request) {
	cfg := loadNabuAuthConfig()
	enabled := cfg.enabled()
	writeJSON(w, http.StatusOK, map[string]bool{
		"enabled": enabled,
		"primary": enabled && cfg.Primary,
	})
}

// consoleNabuStart sends the browser to NabuAuth.
func (s *Server) consoleNabuStart(w http.ResponseWriter, r *http.Request) {
	cfg := loadNabuAuthConfig()
	if !cfg.enabled() {
		writeError(w, http.StatusNotFound, "console single sign-on is not configured")
		return
	}

	buf := make([]byte, 64)
	if _, err := rand.Read(buf); err != nil {
		writeError(w, http.StatusInternalServerError, "could not start sign-in")
		return
	}
	state, verifier := b64u(buf[:32]), b64u(buf[32:])
	sum := sha256.Sum256([]byte(verifier))

	returnTo := "/panel/"
	if strings.Contains(r.Referer(), "/admin") {
		returnTo = "/admin/"
	}

	cookieValue, err := cfg.packFlow(consoleNabuFlow{
		State: state, Verifier: verifier, Expires: time.Now().Add(consoleNabuFlowTTL).Unix(), ReturnTo: returnTo,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start sign-in")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:  consoleNabuFlowCookie,
		Value: cookieValue,
		Path:  "/",
		// HttpOnly so no script on the page can read the verifier. SameSite=Lax
		// rather than Strict, because the cookie must survive the redirect back
		// from NabuAuth — Strict would drop it and every sign-in would fail.
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(consoleNabuFlowTTL.Seconds()),
	})

	q := url.Values{
		"client_id":             {cfg.ClientID},
		"redirect_uri":          {cfg.redirectURI(r)},
		"response_type":         {"code"},
		"scope":                 {cfg.Scopes},
		"state":                 {state},
		"code_challenge":        {b64u(sum[:])},
		"code_challenge_method": {"S256"},
	}
	if p := r.URL.Query().Get("provider"); p != "" {
		q.Set("provider", p)
	}
	http.Redirect(w, r, cfg.URL+"/oauth/authorize?"+q.Encode(), http.StatusFound)
}

// consoleNabuCallback handles the redirect back from NabuAuth.
func (s *Server) consoleNabuCallback(w http.ResponseWriter, r *http.Request) {
	cfg := loadNabuAuthConfig()
	if !cfg.enabled() {
		writeError(w, http.StatusNotFound, "console single sign-on is not configured")
		return
	}

	clearFlow := func() {
		http.SetCookie(w, &http.Cookie{
			Name: consoleNabuFlowCookie, Value: "", Path: "/",
			HttpOnly: true, Secure: requestIsHTTPS(r), SameSite: http.SameSiteLaxMode, MaxAge: -1,
		})
	}
	// The console is a single-page app, so a failure comes back as a flag it can
	// render rather than a JSON body the browser would show raw.
	fail := func(reason string) {
		clearFlow()
		http.Redirect(w, r, "/admin/?nabu_error="+url.QueryEscape(reason), http.StatusFound)
	}

	if e := r.URL.Query().Get("error"); e != "" {
		fail(e)
		return
	}
	cookie, err := r.Cookie(consoleNabuFlowCookie)
	if err != nil {
		fail("expired")
		return
	}
	flow, ok := cfg.unpackFlow(cookie.Value)
	// A callback whose state was not issued here did not start in this browser,
	// so the code in it is not ours to redeem.
	if !ok || !hmac.Equal([]byte(flow.State), []byte(r.URL.Query().Get("state"))) {
		fail("expired")
		return
	}

	profile, err := cfg.signIn(r.Context(), r.URL.Query().Get("code"), flow.Verifier, cfg.redirectURI(r))
	if err != nil {
		s.log.Warn("console nabuauth sign-in failed", "error", err)
		fail("failed")
		return
	}
	
	email := strings.ToLower(profile.Email)
	isAdmin := cfg.Admins[email]

	token, expiry, err := s.admin.NewSession(email, isAdmin)
	if err != nil {
		s.log.Error("could not issue console session", "error", err)
		fail("failed")
		return
	}
	clearFlow()
	http.SetCookie(w, &http.Cookie{
		Name:     consoleCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteStrictMode,
		Expires:  expiry,
		MaxAge:   int(time.Until(expiry).Seconds()),
	})

	dest := flow.ReturnTo
	if dest == "" {
		dest = "/panel/"
	}
	http.Redirect(w, r, dest, http.StatusFound)
}

func requestIsHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

type nabuAuthProfile struct {
	Subject string `json:"sub"`
	Name    string `json:"name"`
	Email   string `json:"email"`
}

// signIn exchanges the code and reads the profile behind it.
func (c nabuAuthConfig) signIn(ctx context.Context, code, verifier, redirectURI string) (nabuAuthProfile, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
		"redirect_uri":  {redirectURI},
		"code":          {code},
		"code_verifier": {verifier},
	}
	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.URL+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nabuAuthProfile{}, err
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var tokens struct {
		AccessToken string `json:"access_token"`
	}
	if err := nabuAuthJSON(tokenReq, &tokens); err != nil {
		return nabuAuthProfile{}, err
	}
	if tokens.AccessToken == "" {
		return nabuAuthProfile{}, errors.New("nabuauth returned no access token")
	}

	profileReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL+"/api/v1/user", nil)
	if err != nil {
		return nabuAuthProfile{}, err
	}
	profileReq.Header.Set("Authorization", "Bearer "+tokens.AccessToken)

	var profile nabuAuthProfile
	if err := nabuAuthJSON(profileReq, &profile); err != nil {
		return nabuAuthProfile{}, err
	}
	if profile.Email == "" || profile.Subject == "" {
		return nabuAuthProfile{}, errors.New("nabuauth returned no verified email")
	}
	return profile, nil
}

func nabuAuthJSON(req *http.Request, out any) error {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		// Bound the body: an error page from a misconfigured proxy does not
		// belong in a log line.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("nabuauth responded %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
