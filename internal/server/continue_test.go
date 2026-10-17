package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"nabugate/internal/adminstore"
)

func continueServer(t *testing.T) (*Server, *adminstore.Store) {
	t.Helper()
	store, err := adminstore.Open(filepath.Join(t.TempDir(), "admin.json"))
	if err != nil {
		t.Fatal(err)
	}
	return &Server{admin: store, log: slog.New(slog.DiscardHandler), logins: newThrottle()}, store
}

func cont(t *testing.T, s *Server, body string) (int, map[string]any, bool) {
	t.Helper()
	w := httptest.NewRecorder()
	s.consoleContinue(w, httptest.NewRequest(http.MethodPost, "/api/auth/continue", strings.NewReader(body)))
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	return w.Code, got, strings.Contains(w.Header().Get("Set-Cookie"), consoleCookie+"=")
}

// One form signs in and signs up: an unknown address is asked to confirm
// before an account is made, then the same form signs it in next time.
func TestContinueSignsUpThenSignsIn(t *testing.T) {
	s, store := continueServer(t)

	code, got, cookie := cont(t, s, `{"username":"New@Example.com","password":"a-long-password"}`)
	if code != http.StatusOK || got["new_account"] != true || cookie {
		t.Fatalf("first press on an unknown address: %d %v cookie=%v", code, got, cookie)
	}
	if store.GetUser("new@example.com") != nil {
		t.Fatal("an account was made before the buyer confirmed")
	}
	if code, got, cookie = cont(t, s, `{"username":"New@Example.com","password":"a-long-password","create":true}`); code != http.StatusOK || !cookie {
		t.Fatalf("create: %d %v", code, got)
	}
	if code, _, cookie = cont(t, s, `{"username":"new@example.com","password":"a-long-password"}`); code != http.StatusOK || !cookie {
		t.Fatalf("sign-in after sign-up: %d", code)
	}
	if code, _, cookie = cont(t, s, `{"username":"new@example.com","password":"wrong-password!"}`); code != http.StatusUnauthorized || cookie {
		t.Fatalf("wrong password: %d", code)
	}
}

// The account rules the form states are the server's rules too.
func TestContinueRefusesWeakSignups(t *testing.T) {
	s, _ := continueServer(t)
	if code, got, _ := cont(t, s, `{"username":"a@example.com","password":"short","create":true}`); code != http.StatusBadRequest || errCode(got) != "short_password" {
		t.Errorf("short password: %d %v", code, got)
	}
	if code, got, _ := cont(t, s, `{"username":"a@b","password":"a-long-password","create":true}`); code != http.StatusBadRequest || errCode(got) != "invalid_email" {
		t.Errorf("invalid email: %d %v", code, got)
	}
}

// An account made through NabuAuth has no password; typing its address must
// not let anyone set one.
func TestContinueSendsPasswordlessAccountsToSSO(t *testing.T) {
	s, store := continueServer(t)
	if _, err := store.EnsureUser("sso@example.com", "Sara"); err != nil {
		t.Fatal(err)
	}
	code, got, cookie := cont(t, s, `{"username":"sso@example.com","password":"a-long-password","create":true}`)
	if code != http.StatusConflict || errCode(got) != "use_sso" || cookie {
		t.Fatalf("%d %v cookie=%v", code, got, cookie)
	}
}

// An administrator whose username is an email signs in, not up.
func TestContinueSignsInAnAdminWithAnEmailUsername(t *testing.T) {
	s, store := continueServer(t)
	if err := store.CreateAdmin("ops@example.com", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	if code, got, cookie := cont(t, s, `{"username":"ops@example.com","password":"admin-password-123"}`); code != http.StatusOK || !cookie {
		t.Fatalf("%d %v", code, got)
	}
}
