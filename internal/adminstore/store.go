// Package adminstore holds the gateway's mutable state: admin accounts, the
// project tokens minted from the console, and usage counters that survive a
// restart.
//
// A JSON file on a volume, not a database. The gateway is otherwise a static
// binary reading a baked config, and everything here fits in memory and is
// written a few times a minute — a database would be a second thing to deploy,
// back up and fail. Writes are atomic (temp file + rename) so a crash mid-write
// leaves the previous state rather than a truncated one.
//
// Tokens declared in config.yaml keep working exactly as before; these are
// merged on top, so the console is additive rather than a migration.
package adminstore

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrBadCredentials = errors.New("invalid username or password")
	ErrNoSession      = errors.New("no valid session")
	ErrTokenNotFound  = errors.New("token not found")
	ErrNameTaken      = errors.New("a token with that name already exists")
)

// pbkdf2 cost. 600k SHA-256 iterations is the OWASP figure; it costs a few
// hundred milliseconds per login, which is unnoticeable for a console and
// expensive at scale for anyone who steals the file.
const (
	pbkdfIterations = 600_000
	pbkdfKeyLen     = 32
	saltLen         = 16
)

// SessionTTL is how long a console login lasts.
const SessionTTL = 12 * time.Hour

// Admin is a console account. The password is never stored, only its PBKDF2
// derivation and the salt it used.
type Admin struct {
	Username  string    `json:"username"`
	Salt      string    `json:"salt"`
	Hash      string    `json:"hash"`
	CreatedAt time.Time `json:"created_at"`
}

// Token is a project key minted from the console.
type Token struct {
	Name      string   `json:"name"`   // the project name usage is attributed to
	Prefix    string   `json:"prefix"` // first characters, so the console can identify it
	Hash      string   `json:"hash"`   // SHA-256 of the token; the token itself is shown once
	Allow     []string `json:"allow"`  // alias globs
	RateLimit int      `json:"rate_limit"`

	// AllowedOrigins restricts where a request may come from. Empty means
	// anywhere. Matched against the request's Origin header, falling back to
	// Referer — a browser sets those and cannot forge them cross-site, which is
	// what makes this useful for a key embedded in a web app.
	AllowedOrigins []string `json:"allowed_origins"`

	Disabled  bool      `json:"disabled"`
	CreatedAt time.Time `json:"created_at"`
	LastUsed  time.Time `json:"last_used,omitempty"`
}

// Counters is the persisted usage for one project.
type Counters struct {
	Requests         int64   `json:"requests"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	Denied           int64   `json:"denied"`
}

type state struct {
	Admins   []Admin              `json:"admins"`
	Tokens   []Token              `json:"tokens"`
	Usage    map[string]Counters  `json:"usage"`
	Sessions map[string]time.Time `json:"sessions"` // token hash -> expiry
}

// Store is the persisted gateway state.
type Store struct {
	path string

	mu    sync.RWMutex
	st    state
	dirty bool
}

// Open loads the state file, creating an empty one if it does not exist.
func Open(path string) (*Store, error) {
	s := &Store{path: path, st: state{
		Usage:    map[string]Counters{},
		Sessions: map[string]time.Time{},
	}}

	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("adminstore: create dir: %w", err)
		}
		return s, s.flush()
	case err != nil:
		return nil, fmt.Errorf("adminstore: read %s: %w", path, err)
	}

	if err := json.Unmarshal(raw, &s.st); err != nil {
		return nil, fmt.Errorf("adminstore: parse %s: %w", path, err)
	}
	if s.st.Usage == nil {
		s.st.Usage = map[string]Counters{}
	}
	if s.st.Sessions == nil {
		s.st.Sessions = map[string]time.Time{}
	}
	s.purgeExpiredLocked()
	return s, nil
}

// flush writes the state atomically: a temp file in the same directory, then a
// rename. A crash mid-write leaves the previous file intact rather than a
// half-written one.
func (s *Store) flush() error {
	raw, err := json.MarshalIndent(s.st, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) save() error {
	s.dirty = false
	return s.flush()
}

// ─────────────────────────── admin accounts ────────────────────────────────

// NeedsSetup reports whether no admin exists yet, so the console can offer to
// create the first one instead of a login it can never satisfy.
func (s *Store) NeedsSetup() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.st.Admins) == 0
}

// CreateAdmin adds a console account. The first one may be created without
// authentication (there is nobody to authorise it); the handler enforces that.
func (s *Store) CreateAdmin(username, password string) error {
	username = strings.TrimSpace(strings.ToLower(username))
	if username == "" {
		return errors.New("username is required")
	}
	if len([]rune(password)) < 10 {
		return errors.New("password must be at least 10 characters")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.st.Admins {
		if a.Username == username {
			return errors.New("that username already exists")
		}
	}

	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, pbkdfIterations, pbkdfKeyLen)
	if err != nil {
		return err
	}

	s.st.Admins = append(s.st.Admins, Admin{
		Username:  username,
		Salt:      base64.RawStdEncoding.EncodeToString(salt),
		Hash:      base64.RawStdEncoding.EncodeToString(key),
		CreatedAt: time.Now().UTC(),
	})
	return s.save()
}

// Authenticate verifies a console login and returns a session token.
//
// A missing account and a wrong password return the same error, and the missing
// case still runs a derivation, so neither the message nor the response time
// distinguishes them.
func (s *Store) Authenticate(username, password string) (string, time.Time, error) {
	username = strings.TrimSpace(strings.ToLower(username))

	s.mu.Lock()
	defer s.mu.Unlock()

	var found *Admin
	for i := range s.st.Admins {
		if s.st.Admins[i].Username == username {
			found = &s.st.Admins[i]
			break
		}
	}

	salt, hash := make([]byte, saltLen), make([]byte, pbkdfKeyLen)
	if found != nil {
		salt, _ = base64.RawStdEncoding.DecodeString(found.Salt)
		hash, _ = base64.RawStdEncoding.DecodeString(found.Hash)
	}

	key, err := pbkdf2.Key(sha256.New, password, salt, pbkdfIterations, pbkdfKeyLen)
	if err != nil {
		return "", time.Time{}, err
	}
	if found == nil || subtle.ConstantTimeCompare(key, hash) != 1 {
		return "", time.Time{}, ErrBadCredentials
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expiry := time.Now().Add(SessionTTL)

	s.purgeExpiredLocked()
	s.st.Sessions[hashString(token)] = expiry
	return token, expiry, s.save()
}

// ValidSession reports whether a console session token is live.
func (s *Store) ValidSession(token string) bool {
	if token == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	exp, ok := s.st.Sessions[hashString(token)]
	return ok && time.Now().Before(exp)
}

func (s *Store) EndSession(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.st.Sessions, hashString(token))
	return s.save()
}

func (s *Store) purgeExpiredLocked() {
	now := time.Now()
	for h, exp := range s.st.Sessions {
		if now.After(exp) {
			delete(s.st.Sessions, h)
		}
	}
}

// ─────────────────────────── project tokens ────────────────────────────────

// NewToken mints a project key. The secret is returned once and never stored;
// only its SHA-256 and a short prefix are kept, so the file cannot be used to
// impersonate a project.
func (s *Store) NewToken(name string, allow []string, rateLimit int, origins []string) (Token, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Token{}, "", errors.New("a project name is required")
	}
	if len(allow) == 0 {
		return Token{}, "", errors.New("an allow-list is required — a key that reaches everything is an admin key")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.st.Tokens {
		if strings.EqualFold(t.Name, name) {
			return Token{}, "", ErrNameTaken
		}
	}

	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return Token{}, "", err
	}
	secret := "ng-" + hex.EncodeToString(raw)

	t := Token{
		Name:           name,
		Prefix:         secret[:9],
		Hash:           hashString(secret),
		Allow:          allow,
		RateLimit:      rateLimit,
		AllowedOrigins: origins,
		CreatedAt:      time.Now().UTC(),
	}
	s.st.Tokens = append(s.st.Tokens, t)
	return t, secret, s.save()
}

// Tokens returns the stored tokens, newest first. Hashes are cleared: nothing
// outside this package needs them, and shipping them to a browser would be a
// needless way to leak an offline-crackable value.
func (s *Store) Tokens() []Token {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Token, len(s.st.Tokens))
	copy(out, s.st.Tokens)
	for i := range out {
		out[i].Hash = ""
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// Lookup resolves a presented secret to its token.
func (s *Store) Lookup(secret string) (Token, bool) {
	if secret == "" {
		return Token{}, false
	}
	h := hashString(secret)

	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.st.Tokens {
		if subtle.ConstantTimeCompare([]byte(t.Hash), []byte(h)) == 1 {
			if t.Disabled {
				return Token{}, false
			}
			return t, true
		}
	}
	return Token{}, false
}

func (s *Store) SetTokenDisabled(name string, disabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.st.Tokens {
		if strings.EqualFold(s.st.Tokens[i].Name, name) {
			s.st.Tokens[i].Disabled = disabled
			return s.save()
		}
	}
	return ErrTokenNotFound
}

func (s *Store) DeleteToken(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.st.Tokens {
		if strings.EqualFold(s.st.Tokens[i].Name, name) {
			s.st.Tokens = append(s.st.Tokens[:i], s.st.Tokens[i+1:]...)
			return s.save()
		}
	}
	return ErrTokenNotFound
}

// SetOrigins replaces a token's origin allow-list.
func (s *Store) SetOrigins(name string, origins []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.st.Tokens {
		if strings.EqualFold(s.st.Tokens[i].Name, name) {
			s.st.Tokens[i].AllowedOrigins = origins
			return s.save()
		}
	}
	return ErrTokenNotFound
}

// ─────────────────────────── usage ─────────────────────────────────────────

// RecordUsage accumulates one call against a project. Kept in memory and
// flushed by Persist, because a disk write per request would dominate the cost
// of a cheap completion.
func (s *Store) RecordUsage(project string, prompt, completion int64, cost float64) {
	if project == "" {
		project = "(admin)"
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	c := s.st.Usage[project]
	c.Requests++
	c.PromptTokens += prompt
	c.CompletionTokens += completion
	c.CostUSD += cost
	s.st.Usage[project] = c

	for i := range s.st.Tokens {
		if strings.EqualFold(s.st.Tokens[i].Name, project) {
			s.st.Tokens[i].LastUsed = time.Now().UTC()
			break
		}
	}
	s.dirty = true
}

// RecordDenied counts a request refused by policy or origin, which is what
// makes a misconfigured caller visible instead of merely absent.
func (s *Store) RecordDenied(project string) {
	if project == "" {
		project = "(unknown)"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.st.Usage[project]
	c.Denied++
	s.st.Usage[project] = c
	s.dirty = true
}

// Usage returns the persisted counters per project.
func (s *Store) Usage() map[string]Counters {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]Counters, len(s.st.Usage))
	for k, v := range s.st.Usage {
		out[k] = v
	}
	return out
}

// ResetUsage clears the counters for one project, or all of them when name is
// empty.
func (s *Store) ResetUsage(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if name == "" {
		s.st.Usage = map[string]Counters{}
	} else {
		delete(s.st.Usage, name)
	}
	return s.save()
}

// Persist writes accumulated usage if anything changed. Call it periodically.
func (s *Store) Persist() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.dirty {
		return nil
	}
	return s.save()
}

func hashString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
