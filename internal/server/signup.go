package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"nabugate/internal/adminstore"
)

// Email-first sign-in and sign-up for the user panel.
//
// Step one asks for the address alone and POSTs it to /api/auth/check, which
// says what that address can do: "password" (ask for the password and sign
// in), "none" (ask for first and last name, mobile and a password, and
// register), or "sso" (the account was made through NabuAuth or Google and is
// reached only that way). Step two is the existing sign-in, or
// /api/auth/register.
//
// Telling a visitor whether an address holds an account is what makes the
// form clearer, and also what an enumerator wants, so both endpoints share a
// per-address budget: maxProbes calls per loginLockFor window. A person signs
// up in two or three calls; a list of thousands of addresses does not fit.

const maxProbes = 30

func newProbeThrottle() *throttle { return newThrottleMax(maxProbes) }

// probe counts one call against the caller's budget and reports whether it may
// proceed; a refused call has been answered with 429.
func (s *Server) probe(w http.ResponseWriter, r *http.Request) bool {
	key := "probe|" + clientIP(r)
	if s.probes.blocked(key) {
		writeErrorCode(w, http.StatusTooManyRequests, "throttled", "too many attempts; try again in a few minutes", nil)
		return false
	}
	s.probes.fail(key)
	return true
}

// consoleCheckEmail is step one: what can this address do?
func (s *Server) consoleCheckEmail(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	if !adminstore.ValidEmail(email) {
		writeErrorCode(w, http.StatusBadRequest, "invalid_email", adminstore.ErrInvalidEmail.Error(), nil)
		return
	}
	if !s.probe(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"email": email, "state": s.admin.AccountState(email)})
}

// consoleRegister is step two for a new address: create the account with the
// details the form collected and sign it in.
func (s *Server) consoleRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email     string `json:"email"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Mobile    string `json:"mobile"`
		Password  string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	// Every field is required here, unlike the legacy one-step form.
	switch {
	case !adminstore.ValidEmail(email):
		writeErrorCode(w, http.StatusBadRequest, "invalid_email", adminstore.ErrInvalidEmail.Error(), nil)
		return
	case strings.TrimSpace(body.FirstName) == "" || strings.TrimSpace(body.LastName) == "":
		writeErrorCode(w, http.StatusBadRequest, "invalid_name", adminstore.ErrInvalidName.Error(), nil)
		return
	case strings.TrimSpace(body.Mobile) == "":
		writeErrorCode(w, http.StatusBadRequest, "invalid_mobile", adminstore.ErrInvalidMobile.Error(), nil)
		return
	}
	if !s.probe(w, r) {
		return
	}
	// An administrator's username that happens to be this address, or an
	// account made meanwhile, is not registered over.
	if state := s.admin.AccountState(email); state != "none" {
		code := "account_exists"
		if state == "sso" {
			code = "use_sso"
		}
		writeErrorCode(w, http.StatusConflict, code, "an account with this email already exists", nil)
		return
	}
	err := s.admin.RegisterUser(adminstore.Registration{
		Email: email, Password: body.Password,
		FirstName: body.FirstName, LastName: body.LastName, Mobile: body.Mobile,
	})
	if err != nil {
		code, status := "signup_failed", http.StatusBadRequest
		switch {
		case errors.Is(err, adminstore.ErrInvalidEmail):
			code = "invalid_email"
		case errors.Is(err, adminstore.ErrShortPassword):
			code = "short_password"
		case errors.Is(err, adminstore.ErrInvalidName):
			code = "invalid_name"
		case errors.Is(err, adminstore.ErrInvalidMobile):
			code = "invalid_mobile"
		case errors.Is(err, adminstore.ErrAccountExists):
			code, status = "account_exists", http.StatusConflict
		default:
			s.log.Error("could not register account", "error", err)
			status = http.StatusInternalServerError
		}
		writeErrorCode(w, status, code, err.Error(), nil)
		return
	}
	s.log.Info("account created", "account", email, "via", "signup")
	s.startConsoleSession(w, r, email, body.Password)
}
