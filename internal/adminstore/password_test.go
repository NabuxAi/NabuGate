package adminstore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newUserStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "admin.json"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := st.SignupUser("Someone@Example.com", "first-password"); err != nil {
		t.Fatalf("signup: %v", err)
	}
	return st
}

func TestChangingAPasswordNeedsTheCurrentOne(t *testing.T) {
	st := newUserStore(t)

	if err := st.ChangeUserPassword("someone@example.com", "not-it", "second-password"); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("a wrong current password was accepted or misreported: %v", err)
	}

	// The old password must still work: a refused change that quietly took
	// effect anyway would lock the owner out of their own account.
	if _, _, err := st.AuthenticateUser("someone@example.com", "first-password"); err != nil {
		t.Fatalf("the refused change altered the stored password: %v", err)
	}
}

func TestAChangedPasswordReplacesTheOldOne(t *testing.T) {
	st := newUserStore(t)

	if err := st.ChangeUserPassword("someone@example.com", "first-password", "second-password"); err != nil {
		t.Fatalf("change: %v", err)
	}
	if _, _, err := st.AuthenticateUser("someone@example.com", "second-password"); err != nil {
		t.Fatalf("the new password does not sign in: %v", err)
	}
	if _, _, err := st.AuthenticateUser("someone@example.com", "first-password"); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("the old password still signs in after a change: %v", err)
	}
}

func TestAShortPasswordIsRefusedBeforeTheCurrentOneIsChecked(t *testing.T) {
	st := newUserStore(t)

	// Correct current password, too-short replacement. The point is that the
	// account is left alone rather than half-changed.
	if err := st.ChangeUserPassword("someone@example.com", "first-password", "short"); err == nil {
		t.Fatal("a five-character password was accepted")
	}
	if _, _, err := st.AuthenticateUser("someone@example.com", "first-password"); err != nil {
		t.Fatalf("the refused change altered the stored password: %v", err)
	}
}

func TestAnAccountWithNoPasswordIsNotGivenOne(t *testing.T) {
	st := newUserStore(t)

	// Somebody who has only ever signed in through single sign-on. Letting an
	// empty current password match would be a way to set a password on an
	// account whose owner never chose one.
	if err := st.ChangeUserPassword("nobody@example.com", "", "a-brand-new-password"); err == nil {
		t.Fatal("a password was set on an account that does not exist")
	}
	if _, _, err := st.AuthenticateUser("nobody@example.com", "a-brand-new-password"); err == nil {
		t.Fatal("the account was created by the attempt to change its password")
	}
}

// A session issued to a non-admin used to be stored under the raw token while
// ValidSession looked it up by hash, so signing in returned a cookie that was
// refused by the very next request — the user panel was unreachable to anyone
// who was not an administrator.
func TestAUserSessionIsAcceptedAfterSigningIn(t *testing.T) {
	st := newUserStore(t)

	token, _, err := st.AuthenticateUser("someone@example.com", "first-password")
	if err != nil {
		t.Fatalf("sign in: %v", err)
	}
	info, ok := st.ValidSession(token)
	if !ok {
		t.Fatal("the session issued at sign-in was refused")
	}
	if info.Email != "someone@example.com" {
		t.Errorf("session email = %q", info.Email)
	}
	if info.IsAdmin {
		t.Error("an ordinary sign-in produced an administrator session")
	}
}

// The other half of the same bug: the raw token was persisted. A copy of the
// state file was therefore a set of working sessions, not a set of hashes.
func TestTheStateFileHoldsNoUsableSessionToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "admin.json")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := st.SignupUser("someone@example.com", "first-password"); err != nil {
		t.Fatalf("signup: %v", err)
	}
	token, _, err := st.AuthenticateUser("someone@example.com", "first-password")
	if err != nil {
		t.Fatalf("sign in: %v", err)
	}

	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if strings.Contains(string(saved), token) {
		t.Error("the state file contains the session token in usable form")
	}
}

// Signing out must end the session, whichever path issued it.
func TestSigningOutEndsAUserSession(t *testing.T) {
	st := newUserStore(t)

	token, _, err := st.AuthenticateUser("someone@example.com", "first-password")
	if err != nil {
		t.Fatalf("sign in: %v", err)
	}
	if err := st.EndSession(token); err != nil {
		t.Fatalf("sign out: %v", err)
	}
	if _, ok := st.ValidSession(token); ok {
		t.Error("the session still works after signing out")
	}
}
