package adminstore

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"unicode/utf8"
)

// Registration is a new password account: what the email-first sign-up form
// collects once it has established that no account holds the address.
type Registration struct {
	Email     string
	Password  string
	FirstName string
	LastName  string
	Mobile    string
}

var (
	ErrAccountExists = errors.New("an account with this email already exists")
	ErrInvalidName   = errors.New("enter a first and last name")
	ErrInvalidMobile = errors.New("enter a valid mobile number")
)

const maxNameRunes = 60

// RegisterUser creates a password account. Names and mobile are optional for
// the legacy single-step form (SignupUser), but when either name is given
// both must be, and a given mobile must be a valid number.
func (s *Store) RegisterUser(reg Registration) error {
	email := strings.TrimSpace(strings.ToLower(reg.Email))
	if !ValidEmail(email) {
		return ErrInvalidEmail
	}
	if utf8.RuneCountInString(reg.Password) < 10 {
		return ErrShortPassword
	}
	first, last := cleanName(reg.FirstName), cleanName(reg.LastName)
	if (first == "") != (last == "") || utf8.RuneCountInString(first) > maxNameRunes || utf8.RuneCountInString(last) > maxNameRunes {
		return ErrInvalidName
	}
	mobile := ""
	if strings.TrimSpace(reg.Mobile) != "" {
		var ok bool
		if mobile, ok = NormalizeMobile(reg.Mobile); !ok {
			return ErrInvalidMobile
		}
	}

	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	hash, err := pbkdf2.Key(sha256.New, reg.Password, salt, pbkdfIterations, pbkdfKeyLen)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.st.Users == nil {
		s.st.Users = make(map[string]*User)
	}
	if _, exists := s.st.Users[email]; exists {
		return ErrAccountExists
	}
	s.st.Users[email] = &User{
		Email:     email,
		Salt:      base64.RawStdEncoding.EncodeToString(salt),
		Hash:      base64.RawStdEncoding.EncodeToString(hash),
		Name:      strings.TrimSpace(first + " " + last),
		FirstName: first,
		LastName:  last,
		Mobile:    mobile,
	}
	return s.save()
}

// cleanName trims a name and collapses inner whitespace; control characters
// make it invalid (returned as a name too long to pass).
func cleanName(name string) string {
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return strings.Repeat("x", maxNameRunes+1)
		}
	}
	return strings.Join(strings.Fields(name), " ")
}

// NormalizeMobile accepts a mobile number as people type it — Persian or
// Arabic digits, spaces, dashes, a leading 0, 0098 or +98 — and returns it in
// international form. An Iranian mobile (9xxxxxxxxx after the country code)
// becomes +989xxxxxxxxx; any other number must already carry its country
// code (+ or 00) and have 8 to 15 digits.
func NormalizeMobile(raw string) (string, bool) {
	var b strings.Builder
	for i, r := range strings.TrimSpace(raw) {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= '۰' && r <= '۹':
			b.WriteRune('0' + (r - '۰'))
		case r >= '٠' && r <= '٩':
			b.WriteRune('0' + (r - '٠'))
		case r == '+' && i == 0:
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '(' || r == ')' || r == '.':
		default:
			return "", false
		}
	}
	n := b.String()
	if strings.HasPrefix(n, "00") {
		n = "+" + n[2:]
	}
	switch {
	case len(n) == 11 && strings.HasPrefix(n, "09"):
		return "+98" + n[1:], true
	case len(n) == 10 && strings.HasPrefix(n, "9"):
		return "+98" + n, true
	case strings.HasPrefix(n, "+98"):
		rest := n[3:]
		if strings.HasPrefix(rest, "0") {
			rest = rest[1:]
		}
		if len(rest) == 10 && rest[0] == '9' {
			return "+98" + rest, true
		}
		return "", false
	case strings.HasPrefix(n, "+") && len(n)-1 >= 8 && len(n)-1 <= 15 && n[1] != '0':
		return n, true
	}
	return "", false
}
