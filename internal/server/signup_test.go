package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func signupCall(t *testing.T, h http.HandlerFunc, path, body string) (int, map[string]any, bool) {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.RemoteAddr = "203.0.113.7:5555"
	h(w, r)
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	return w.Code, got, strings.Contains(w.Header().Get("Set-Cookie"), consoleCookie+"=")
}

// Step one tells the form which step two to show.
func TestCheckEmailSaysWhatAnAddressCanDo(t *testing.T) {
	s, store := continueServer(t)
	s.probes = newProbeThrottle()
	if err := store.SignupUser("has@example.com", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsureUser("sso@example.com", "Sara"); err != nil {
		t.Fatal(err)
	}
	for email, want := range map[string]string{"New@Example.com": "none", "has@example.com": "password", "sso@example.com": "sso"} {
		code, got, _ := signupCall(t, s.consoleCheckEmail, "/api/auth/check", `{"email":"`+email+`"}`)
		if code != http.StatusOK || got["state"] != want {
			t.Errorf("%s: %d %v, want %s", email, code, got, want)
		}
	}
	if code, got, _ := signupCall(t, s.consoleCheckEmail, "/api/auth/check", `{"email":"nope"}`); code != http.StatusBadRequest || errCode(got) != "invalid_email" {
		t.Errorf("invalid email: %d %v", code, got)
	}
}

// The address check is an account oracle, so it is budgeted per source.
func TestCheckEmailIsRateLimited(t *testing.T) {
	s, _ := continueServer(t)
	s.probes = newThrottleMax(3)
	for i := 0; i < 3; i++ {
		if code, _, _ := signupCall(t, s.consoleCheckEmail, "/api/auth/check", `{"email":"a@example.com"}`); code != http.StatusOK {
			t.Fatalf("call %d: %d", i, code)
		}
	}
	if code, got, _ := signupCall(t, s.consoleCheckEmail, "/api/auth/check", `{"email":"a@example.com"}`); code != http.StatusTooManyRequests || errCode(got) != "throttled" {
		t.Fatalf("over budget: %d %v", code, got)
	}
	if code, _, _ := signupCall(t, s.consoleRegister, "/api/auth/register", `{"email":"b@example.com","first_name":"a","last_name":"b","mobile":"09121234567","password":"a-long-password"}`); code != http.StatusTooManyRequests {
		t.Fatalf("register shares the budget: %d", code)
	}
}

func TestRegisterCreatesTheAccountAndSignsIn(t *testing.T) {
	s, store := continueServer(t)
	s.probes = newProbeThrottle()
	code, got, cookie := signupCall(t, s.consoleRegister, "/api/auth/register",
		`{"email":"Neda@Example.com","first_name":" ندا ","last_name":"رضایی","mobile":"۰۹۱۲ ۱۲۳ ۴۵۶۷","password":"a-long-password"}`)
	if code != http.StatusOK || !cookie {
		t.Fatalf("register: %d %v cookie=%v", code, got, cookie)
	}
	u := store.GetUser("neda@example.com")
	if u == nil || u.FirstName != "ندا" || u.LastName != "رضایی" || u.Name != "ندا رضایی" || u.Mobile != "+989121234567" {
		t.Fatalf("stored %+v", u)
	}
	// The same address cannot be registered twice; the form sends it to sign-in.
	if code, got, _ = signupCall(t, s.consoleRegister, "/api/auth/register",
		`{"email":"neda@example.com","first_name":"x","last_name":"y","mobile":"09121234567","password":"another-long-one"}`); code != http.StatusConflict || errCode(got) != "account_exists" {
		t.Fatalf("second registration: %d %v", code, got)
	}
	// And the password it set signs in.
	if code, _, cookie = cont(t, s, `{"username":"neda@example.com","password":"a-long-password"}`); code != http.StatusOK || !cookie {
		t.Fatalf("sign-in after register: %d", code)
	}
}

func TestRegisterRequiresEveryField(t *testing.T) {
	s, store := continueServer(t)
	s.probes = newProbeThrottle()
	cases := map[string]string{
		`{"email":"a@example.com","first_name":"","last_name":"b","mobile":"09121234567","password":"a-long-password"}`: "invalid_name",
		`{"email":"a@example.com","first_name":"a","last_name":"b","mobile":"","password":"a-long-password"}`:           "invalid_mobile",
		`{"email":"a@example.com","first_name":"a","last_name":"b","mobile":"12345","password":"a-long-password"}`:      "invalid_mobile",
		`{"email":"a@example.com","first_name":"a","last_name":"b","mobile":"09121234567","password":"short"}`:          "short_password",
		`{"email":"a@b","first_name":"a","last_name":"b","mobile":"09121234567","password":"a-long-password"}`:          "invalid_email",
	}
	for body, want := range cases {
		if code, got, cookie := signupCall(t, s.consoleRegister, "/api/auth/register", body); code != http.StatusBadRequest || errCode(got) != want || cookie {
			t.Errorf("%s: %d %v, want %s", body, code, got, want)
		}
	}
	if store.GetUser("a@example.com") != nil {
		t.Fatal("a refused registration left an account behind")
	}
}

// An account made through NabuAuth has no password; registering its address
// must not set one.
func TestRegisterRefusesAnSSOAccount(t *testing.T) {
	s, store := continueServer(t)
	s.probes = newProbeThrottle()
	if _, err := store.EnsureUser("sso@example.com", "Sara"); err != nil {
		t.Fatal(err)
	}
	code, got, cookie := signupCall(t, s.consoleRegister, "/api/auth/register",
		`{"email":"sso@example.com","first_name":"a","last_name":"b","mobile":"09121234567","password":"a-long-password"}`)
	if code != http.StatusConflict || errCode(got) != "use_sso" || cookie {
		t.Fatalf("%d %v cookie=%v", code, got, cookie)
	}
}
