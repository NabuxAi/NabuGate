package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"log/slog"

	"nabugate/internal/adminstore"
	"nabugate/internal/agent"
	"nabugate/internal/policy"
	"nabugate/internal/router"
)

// /api/me used to answer with the stored adminstore.User, whose Salt and Hash
// carry json tags. Every signed-in page therefore received the account's
// password hash, and so did anything that could read one response body.
func TestTheAccountEndpointDoesNotPublishThePasswordHash(t *testing.T) {
	store, err := adminstore.Open(filepath.Join(t.TempDir(), "admin.json"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := store.SignupUser("someone@example.com", "a-real-password"); err != nil {
		t.Fatalf("signup: %v", err)
	}

	s := &Server{admin: store}
	r := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	r = r.WithContext(context.WithValue(r.Context(), consoleEmailCtxKey{}, "someone@example.com"))
	w := httptest.NewRecorder()
	s.getMe(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	body := w.Body.String()
	for _, secret := range []string{"salt", "hash"} {
		if strings.Contains(strings.ToLower(body), `"`+secret+`"`) {
			t.Errorf("the account response carries a %q field: %s", secret, body)
		}
	}

	// The fields the panel actually renders must still be there, or this would
	// pass by returning nothing at all.
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["email"] != "someone@example.com" {
		t.Errorf("email = %v, want the signed-in address", got["email"])
	}
	if _, ok := got["balance"]; !ok {
		t.Error("the response carries no balance")
	}
}

// A caller with no stored account still gets an answer rather than an error:
// the panel renders a balance of zero, which is what a new account has.
func TestTheAccountEndpointAnswersForAnAccountThatWasNeverStored(t *testing.T) {
	store, err := adminstore.Open(filepath.Join(t.TempDir(), "admin.json"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	s := &Server{admin: store}
	r := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	r = r.WithContext(context.WithValue(r.Context(), consoleEmailCtxKey{}, "nobody@example.com"))
	w := httptest.NewRecorder()
	s.getMe(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["email"] != "nobody@example.com" || got["balance"] != float64(0) {
		t.Errorf("unexpected response: %v", got)
	}
}

// /api/users had the same shape of bug as /api/me: it answered with the stored
// records, so the admin user screen received every account's password hash.
func TestTheUserListDoesNotPublishPasswordHashes(t *testing.T) {
	store, err := adminstore.Open(filepath.Join(t.TempDir(), "admin.json"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	for _, email := range []string{"one@example.com", "two@example.com"} {
		if err := store.SignupUser(email, "a-real-password"); err != nil {
			t.Fatalf("signup %s: %v", email, err)
		}
	}

	s := &Server{admin: store}
	w := httptest.NewRecorder()
	s.listUsers(w, httptest.NewRequest(http.MethodGet, "/api/users", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	body := strings.ToLower(w.Body.String())
	for _, secret := range []string{`"salt"`, `"hash"`} {
		if strings.Contains(body, secret) {
			t.Errorf("the user list carries a %s field: %s", secret, w.Body.String())
		}
	}

	var got struct {
		Users []map[string]any `json:"users"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Users) != 2 {
		t.Fatalf("got %d users, want 2", len(got.Users))
	}
}

// consoleUsage answered with the whole deployment to every signed-in caller,
// so anyone who signed up through the public form could read every other
// customer's project names, request counts and spend.
func TestUsageIsScopedToTheCallerUnlessTheyAreAnAdmin(t *testing.T) {
	store, err := adminstore.Open(filepath.Join(t.TempDir(), "admin.json"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if _, _, err := store.NewToken("mine", []string{"*"}, 0, nil, "me@example.com", nil); err != nil {
		t.Fatalf("mint key: %v", err)
	}
	if _, _, err := store.NewToken("theirs", []string{"*"}, 0, nil, "them@example.com", nil); err != nil {
		t.Fatalf("mint key: %v", err)
	}
	store.RecordUsage("mine", "openai", "gpt", 10, 10, 0.01)
	store.RecordUsage("theirs", "openai", "gpt", 99, 99, 9.99)

	s := &Server{admin: store}

	ask := func(email string, admin bool) map[string]any {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/api/usage", nil)
		ctx := context.WithValue(r.Context(), consoleEmailCtxKey{}, email)
		ctx = context.WithValue(ctx, consoleAdminCtxKey{}, admin)
		w := httptest.NewRecorder()
		s.consoleUsage(w, r.WithContext(ctx))
		if w.Code != http.StatusOK {
			t.Fatalf("status %d", w.Code)
		}
		var got map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return got
	}

	mine, _ := ask("me@example.com", false)["by_project"].(map[string]any)
	if _, leaked := mine["theirs"]; leaked {
		t.Error("a non-admin was shown another owner's project")
	}
	if _, ok := mine["mine"]; !ok {
		t.Error("a non-admin was not shown their own project")
	}

	// The administrator must still see both, or the check above would pass by
	// showing nobody anything.
	all, _ := ask("admin@example.com", true)["by_project"].(map[string]any)
	if len(all) != 2 {
		t.Errorf("an administrator saw %d projects, want 2", len(all))
	}
}

// Closing the deployment-wide usage leak on /api/usage while leaving it open on
// /api/overview would have moved the door rather than shut it: the same
// per-project spend is served from both, and the panel's dashboard reads it
// from overview.
func TestOverviewDoesNotLeakAnotherOwnersSpend(t *testing.T) {
	store, err := adminstore.Open(filepath.Join(t.TempDir(), "admin.json"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if _, _, err := store.NewToken("mine", []string{"*"}, 0, nil, "me@example.com", nil); err != nil {
		t.Fatalf("mint key: %v", err)
	}
	store.RecordUsage("mine", "openai", "gpt", 1, 1, 0.01)
	store.RecordUsage("theirs", "openai", "gpt", 99, 99, 9.99)

	s := newOverviewServer(store)

	ask := func(email string, admin bool) map[string]any {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/api/overview", nil)
		ctx := context.WithValue(r.Context(), consoleEmailCtxKey{}, email)
		ctx = context.WithValue(ctx, consoleAdminCtxKey{}, admin)
		w := httptest.NewRecorder()
		s.consoleOverview(w, r.WithContext(ctx))
		if w.Code != http.StatusOK {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		var got map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return got
	}

	mine := ask("me@example.com", false)
	usage, _ := mine["usage"].(map[string]any)
	if _, leaked := usage["theirs"]; leaked {
		t.Error("a non-admin was shown another owner's spend through overview")
	}
	if _, ok := usage["mine"]; !ok {
		t.Error("a non-admin was not shown their own spend")
	}
	// The deployment's declared project names are the operator's inventory.
	if keys, _ := mine["config_keys"].([]any); len(keys) != 0 {
		t.Errorf("a non-admin was shown the deployment's config keys: %v", keys)
	}

	all := ask("admin@example.com", true)
	if usage, _ := all["usage"].(map[string]any); len(usage) != 2 {
		t.Errorf("an administrator saw %d projects, want 2", len(usage))
	}
}

// newOverviewServer builds the minimum a consoleOverview call touches: the
// router for provider and alias names, the policy enforcer for the declared
// project list, and the agent registry for agent names.
func newOverviewServer(store *adminstore.Store) *Server {
	return &Server{
		admin:  store,
		router: router.New(nil, nil, nil, nil, nil, nil, nil, slog.New(slog.DiscardHandler)),
		policy: policy.New([]string{"a-config-key"}, nil),
		agents: agent.NewRegistry(),
	}
}
