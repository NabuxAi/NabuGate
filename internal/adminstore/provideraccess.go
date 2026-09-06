package adminstore

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// --- Provider access: your key, or ours ---
//
// Two ways a user reaches an upstream through this gateway:
//
//   - Their own credential. They save it here (sealed) or send it per request.
//     Nothing to approve: it is their account and their bill.
//   - The gateway's credential. That spends the deployment's money, so it is a
//     request. Some providers grant it on the spot — the cheap ones, the local
//     ones, the ones already paid for — and the rest wait for a human.
//
// A grant is additive and only ever concerns the gateway's own credential. It
// can never take away access someone already has, which is what lets this ship
// without auditing every existing caller.

// Request statuses.
const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusDenied   = "denied"
)

// ProviderRequest is one user's ask to spend the gateway's credential on one
// provider.
type ProviderRequest struct {
	ID       string `json:"id"`
	Owner    string `json:"owner"`
	Provider string `json:"provider"`
	Status   string `json:"status"`
	// Note is what the user said when asking; Decision is what the admin said
	// when answering. Keeping both means a denial can explain itself.
	Note     string `json:"note,omitempty"`
	Decision string `json:"decision,omitempty"`
	// CreditUSD is credit added to the user's balance on approval. Zero is
	// normal — approval and credit are separate gifts.
	CreditUSD float64   `json:"credit_usd,omitempty"`
	Auto      bool      `json:"auto,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	DecidedAt time.Time `json:"decided_at,omitempty"`
	DecidedBy string    `json:"decided_by,omitempty"`
}

func normEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
func normProv(s string) string  { return strings.ToLower(strings.TrimSpace(s)) }

// RequestProvider records a request, or returns the existing one. Asking twice
// is not an error and does not reset a decision — a user clicking again should
// see the same pending request, not lose their place in the queue.
//
// auto grants it immediately, for providers the deployment marked as needing no
// approval.
func (s *Store) RequestProvider(owner, provider, note string, auto bool, credit float64) (ProviderRequest, error) {
	owner, provider = normEmail(owner), normProv(provider)
	if owner == "" || provider == "" {
		return ProviderRequest{}, errors.New("adminstore: request needs an owner and a provider")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.st.ProviderRequests {
		r := &s.st.ProviderRequests[i]
		if r.Owner == owner && r.Provider == provider {
			// A denial is not a wall: asking again reopens it, so a user whose
			// circumstances changed does not need an admin to find the old row.
			if r.Status == StatusDenied {
				r.Status = StatusPending
				r.Note = note
				r.Decision = ""
				r.CreatedAt = time.Now().UTC()
				r.DecidedAt = time.Time{}
				r.DecidedBy = ""
				s.dirty = true
				if err := s.save(); err != nil {
					return ProviderRequest{}, err
				}
			}
			return *r, nil
		}
	}

	req := ProviderRequest{
		ID:        fmt.Sprintf("%s|%s", owner, provider),
		Owner:     owner,
		Provider:  provider,
		Status:    StatusPending,
		Note:      note,
		CreatedAt: time.Now().UTC(),
	}
	if auto {
		req.Status = StatusApproved
		req.Auto = true
		req.DecidedAt = req.CreatedAt
		req.DecidedBy = "auto"
		req.CreditUSD = credit
		if credit > 0 {
			s.creditLocked(owner, credit)
		}
	}
	s.st.ProviderRequests = append(s.st.ProviderRequests, req)
	s.dirty = true
	if err := s.save(); err != nil {
		return ProviderRequest{}, err
	}
	return req, nil
}

// creditLocked tops up a balance. Callers hold the lock.
func (s *Store) creditLocked(owner string, amount float64) {
	if s.st.Users == nil {
		s.st.Users = map[string]*User{}
	}
	u := s.st.Users[owner]
	if u == nil {
		u = &User{Email: owner}
		s.st.Users[owner] = u
	}
	u.Balance += amount
}

// DecideProviderRequest approves or denies a pending request. Credit is added
// to the user's balance on approval, so granting access and funding it are one
// action for the admin.
func (s *Store) DecideProviderRequest(id string, approve bool, credit float64, decision, by string) (ProviderRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.st.ProviderRequests {
		r := &s.st.ProviderRequests[i]
		if r.ID != id {
			continue
		}
		r.Status = StatusDenied
		if approve {
			r.Status = StatusApproved
		}
		r.Decision = decision
		r.DecidedAt = time.Now().UTC()
		r.DecidedBy = by
		if approve && credit > 0 {
			r.CreditUSD += credit
			s.creditLocked(r.Owner, credit)
		}
		s.dirty = true
		if err := s.save(); err != nil {
			return ProviderRequest{}, err
		}
		return *r, nil
	}
	return ProviderRequest{}, fmt.Errorf("adminstore: no request %q", id)
}

// ProviderRequestsFor returns one user's requests.
func (s *Store) ProviderRequestsFor(owner string) []ProviderRequest {
	owner = normEmail(owner)
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []ProviderRequest
	for _, r := range s.st.ProviderRequests {
		if r.Owner == owner {
			out = append(out, r)
		}
	}
	return out
}

// AllProviderRequests returns every request, newest first, for the admin queue.
func (s *Store) AllProviderRequests() []ProviderRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ProviderRequest, len(s.st.ProviderRequests))
	copy(out, s.st.ProviderRequests)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// ApprovedProviders is the set of providers a user may spend the gateway's
// credential on.
func (s *Store) ApprovedProviders(owner string) map[string]bool {
	owner = normEmail(owner)
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[string]bool{}
	for _, r := range s.st.ProviderRequests {
		if r.Owner == owner && r.Status == StatusApproved {
			out[r.Provider] = true
		}
	}
	return out
}

// --- stored credentials ---

// SetProviderKey seals a user's own upstream credential. It refuses rather than
// storing plaintext when the deployment has no secret.
func (s *Store) SetProviderKey(owner, provider, key string) error {
	owner, provider = normEmail(owner), normProv(provider)
	key = strings.TrimSpace(key)
	if owner == "" || provider == "" || key == "" {
		return errors.New("adminstore: need an owner, a provider and a key")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	blob, err := seal(s.secret, key)
	if err != nil {
		return err
	}
	if s.st.Users == nil {
		s.st.Users = map[string]*User{}
	}
	u := s.st.Users[owner]
	if u == nil {
		u = &User{Email: owner}
		s.st.Users[owner] = u
	}
	if u.ProviderKeys == nil {
		u.ProviderKeys = map[string]StoredKey{}
	}
	u.ProviderKeys[provider] = StoredKey{Prefix: keyPrefix(key), Blob: blob}
	s.dirty = true
	return s.save()
}

// DeleteProviderKey forgets a stored credential.
func (s *Store) DeleteProviderKey(owner, provider string) error {
	owner, provider = normEmail(owner), normProv(provider)
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.st.Users[owner]
	if u == nil || u.ProviderKeys == nil {
		return nil
	}
	delete(u.ProviderKeys, provider)
	s.dirty = true
	return s.save()
}

// ProviderKeyPrefixes reports which providers a user has saved a key for, and
// the identifying head of each. It never decrypts, so it is what a console
// response is built from.
func (s *Store) ProviderKeyPrefixes(owner string) map[string]string {
	owner = normEmail(owner)
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[string]string{}
	if u := s.st.Users[owner]; u != nil {
		for name, k := range u.ProviderKeys {
			out[name] = k.Prefix
		}
	}
	return out
}

// ProviderKeys decrypts a user's stored credentials for use on one request. A
// key that will not decrypt — the usual cause is a rotated secret — is left out
// rather than failing the lot, so the request falls through to the gateway's
// own credential instead of erroring.
func (s *Store) ProviderKeys(owner string) map[string]string {
	owner = normEmail(owner)
	s.mu.RLock()
	defer s.mu.RUnlock()
	u := s.st.Users[owner]
	if u == nil || len(u.ProviderKeys) == 0 {
		return nil
	}
	out := map[string]string{}
	for name, k := range u.ProviderKeys {
		plain, err := unseal(s.secret, k.Blob)
		if err != nil {
			continue
		}
		out[name] = plain
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// SecretConfigured reports whether this deployment can store provider keys at
// all, so the console can say so instead of offering a form that will fail.
func (s *Store) SecretConfigured() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.secret) > 0
}
