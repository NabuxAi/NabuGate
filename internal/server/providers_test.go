package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"nabugate/internal/adminstore"
	"nabugate/internal/config"
	"nabugate/internal/provider"
	"nabugate/internal/router"
)

// A stored credential must never come back out of an API response. The store
// tags it for JSON, so the only thing keeping it in is that every handler
// projects rather than marshalling the record.
func TestStoredKeyNeverLeavesTheStore(t *testing.T) {
	t.Setenv("NABUGATE_SECRET_KEY", "master")
	st, err := adminstore.Open(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	const secret = "sk-do-not-print-me"
	if err := st.SetProviderKey("me@example.com", "openai", secret); err != nil {
		t.Fatal(err)
	}

	s := &Server{
		admin:  st,
		router: router.New(nil, nil, nil, nil, nil, nil, nil, discardLogger()),
		providers: map[string]config.ProviderMeta{
			"openai": {Name: "openai", Enabled: true, Access: "auto", BYOK: true, KeyEnv: "OPENAI_API_KEY"},
		},
	}

	// Every place a user record is rendered.
	for _, probe := range []struct {
		name string
		call func() *httptest.ResponseRecorder
	}{
		{"/api/me", func() *httptest.ResponseRecorder {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/api/me", nil)
			r = r.WithContext(context.WithValue(r.Context(), consoleEmailCtxKey{}, "me@example.com"))
			s.getMe(w, r)
			return w
		}},
		{"/api/users", func() *httptest.ResponseRecorder {
			w := httptest.NewRecorder()
			s.listUsers(w, httptest.NewRequest("GET", "/api/users", nil))
			return w
		}},
		{"/api/providers", func() *httptest.ResponseRecorder {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/api/providers", nil)
			r = r.WithContext(context.WithValue(r.Context(), consoleEmailCtxKey{}, "me@example.com"))
			s.listProviders(w, r)
			return w
		}},
	} {
		body := probe.call().Body.String()
		if strings.Contains(body, secret) {
			t.Errorf("%s leaked the stored key", probe.name)
		}
		// The sealed form must not escape either: it is one env var away from
		// being the key.
		if strings.Contains(body, "\"blob\"") {
			t.Errorf("%s exposed the ciphertext", probe.name)
		}
	}

	// The catalogue must still say a key is saved, and identify which.
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/providers", nil)
	r = r.WithContext(context.WithValue(r.Context(), consoleEmailCtxKey{}, "me@example.com"))
	s.listProviders(w, r)
	if !strings.Contains(w.Body.String(), `"have_key":true`) {
		t.Errorf("catalogue does not show the saved key: %s", w.Body.String())
	}
}

// An explicit header beats a stored key, and asking for the gateway's key still
// works — otherwise saving a credential once would trap a user with it forever.
func TestStoredKeysMergeUnderHeaders(t *testing.T) {
	t.Setenv("NABUGATE_SECRET_KEY", "master")
	st, err := adminstore.Open(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetProviderKey("me@example.com", "openai", "sk-stored"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetProviderKey("me@example.com", "gemini", "AIza-stored"); err != nil {
		t.Fatal(err)
	}
	s := &Server{admin: st}

	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Set("X-Nabu-Key-Openai", "sk-from-header")
	ctx, _ := withCallerKeys(req.Context(), req)
	ctx = s.withOwnerCredentials(ctx, "me@example.com")

	creds, _ := ctx.Value(router.CallerKeysCtxKey{}).(router.CallerKeys)
	if creds.Keys["openai"] != "sk-from-header" {
		t.Errorf("openai = %q; an explicit header must beat the stored key", creds.Keys["openai"])
	}
	if creds.Keys["gemini"] != "AIza-stored" {
		t.Errorf("gemini = %q; the stored key should fill in where no header was sent", creds.Keys["gemini"])
	}

	// With nothing but stored keys, the mode must not stay "global" — that is
	// what Normalize picks from an empty header set, and it would ignore every
	// saved credential.
	bare := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	bareCtx, _ := withCallerKeys(bare.Context(), bare)
	bareCtx = s.withOwnerCredentials(bareCtx, "me@example.com")
	if got := bareCtx.Value(router.CallerKeysCtxKey{}).(router.CallerKeys).Mode; got != router.ModeOwnFirst {
		t.Errorf("mode = %q, want own-first once stored keys are in play", got)
	}

	// And an explicit "global" still routes through the gateway.
	forced := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	forced.Header.Set("X-Nabu-Key-Mode", "global")
	forced.Header.Set("X-Nabu-Key-Openai", "sk-x")
	fctx, _ := withCallerKeys(forced.Context(), forced)
	fctx = s.withOwnerCredentials(fctx, "me@example.com")
	if got := fctx.Value(router.CallerKeysCtxKey{}).(router.CallerKeys).Mode; got != router.ModeGlobal {
		t.Errorf("mode = %q; an explicit global must survive stored keys", got)
	}
}

// A grant gates the gateway's credential only, and only for a console user's
// token. A deployment's own baked key has no owner and is never gated.
func TestGatewayGateIsOwnerScopedAndAdditive(t *testing.T) {
	st, err := adminstore.Open(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{admin: st, providers: map[string]config.ProviderMeta{
		"openai": {Name: "openai", Enabled: true, Access: "request"},
		"groq":   {Name: "groq", Enabled: true, Access: "auto"},
	}}

	// No owner: a baked config key. Nothing is gated.
	if ctx := s.withOwnerCredentials(context.Background(), ""); ctx.Value(router.GatewayDeniedCtxKey{}) != nil {
		t.Error("a keyless-owner token was gated; every internal integration would break")
	}

	ctx := s.withOwnerCredentials(context.Background(), "me@example.com")
	denied, _ := ctx.Value(router.GatewayDeniedCtxKey{}).(map[string]bool)
	if !denied["openai"] {
		t.Error("a request-gated provider was not held back")
	}
	if denied["groq"] {
		t.Error("an auto provider was gated")
	}

	// Approval opens it, and never closes anything.
	if _, err := st.RequestProvider("me@example.com", "openai", "", true, 0); err != nil {
		t.Fatal(err)
	}
	ctx = s.withOwnerCredentials(context.Background(), "me@example.com")
	denied, _ = ctx.Value(router.GatewayDeniedCtxKey{}).(map[string]bool)
	if denied["openai"] {
		t.Error("approval did not open the gateway rung")
	}
}

// The catalogue must merge two sources that disagree: what this deployment
// configures, and what exists in the world. Neither alone answers the screen's
// question.
func TestCatalogueMergesConfigAndWorld(t *testing.T) {
	st, err := adminstore.Open(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		admin:  st,
		router: router.New(map[string]provider.Adapter{"whisper": nil}, nil, nil, nil, nil, nil, nil, discardLogger()),
		providers: map[string]config.ProviderMeta{
			"whisper":      {Name: "whisper", Enabled: true, Access: "auto"},
			"speechmatics": {Name: "speechmatics", Enabled: true, Access: "auto", BYOK: true, KeyEnv: "SPEECHMATICS_API_KEY"},
			// Configured but unknown to the vendor catalogue: it must still
			// render, or adding a provider would mean a frontend change.
			"somenewthing": {Name: "somenewthing", Enabled: true, Access: "auto", BYOK: true},
		},
	}

	rows := map[string]providerView{}
	for _, row := range s.catalogueFor("me@example.com") {
		rows[row.Name] = row
	}

	// Live: configured and its adapter came up.
	if w := rows["whisper"]; !w.Live || !w.Configured || !w.UsesGatewayKey {
		t.Errorf("whisper = %+v; it is up and needs no approval", w)
	}
	// Configured but keyless: on the list, not usable, and the row names the
	// env var an operator has to set.
	if sm := rows["speechmatics"]; sm.Live || !sm.Configured || sm.KeyEnv != "SPEECHMATICS_API_KEY" {
		t.Errorf("speechmatics = %+v", sm)
	}
	// Known to the world, not wired here: this is the row that exists so it can
	// be asked for.
	dg, ok := rows["deepgram"]
	if !ok {
		t.Fatal("a vendor this gateway does not route to is missing; nobody can ask for it")
	}
	if dg.Configured || dg.Access != "request" || dg.Label == "" {
		t.Errorf("deepgram = %+v", dg)
	}
	// Unknown to the catalogue: renders from its own name rather than vanishing.
	if n := rows["somenewthing"]; !n.Configured || n.Label != "somenewthing" {
		t.Errorf("uncatalogued provider = %+v", n)
	}
	// The key itself is never part of a row, catalogued or not.
	for name, row := range rows {
		if row.KeyPrefix != "" && !row.HaveKey {
			t.Errorf("%s shows a key prefix with no stored key", name)
		}
	}
}
