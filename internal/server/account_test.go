package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"nabugate/internal/adminstore"
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
