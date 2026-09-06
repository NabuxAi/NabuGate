package adminstore

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// --- Subscriptions: paying to spend the gateway's keys ---
//
// Two separate things a user pays for, and keeping them separate is the whole
// design:
//
//   - The subscription. A recurring fee that unlocks the gateway's own vendor
//     credentials. It buys permission, not usage.
//   - The usage. Every call served on those credentials is metered against the
//     same balance the user tops up, at the rate their plan names.
//
// A user running on their own keys pays neither: their vendor bills them
// directly, and this gateway charges zero for passing the request along. That
// asymmetry is the point — "our keys cost extra" is not a surcharge, it is the
// only thing there is to charge for.
//
// Expiry is read at request time and never swept: a lapsed subscription simply
// stops satisfying the gate on the next call. There is no background job here,
// and adding one would only introduce a window where the two disagree.

// Subscription is what a user bought, as they bought it. The plan's terms are
// copied in rather than looked up later, so editing a plan in config changes
// what new subscribers get and never what an existing one is already paying.
type Subscription struct {
	PlanID string `json:"plan_id"`
	Name   string `json:"name,omitempty"`

	StartedAt time.Time `json:"started_at"`
	ExpiresAt time.Time `json:"expires_at"`

	// PriceUSD is what was charged for this term.
	PriceUSD float64 `json:"price_usd"`
	// Markup multiplies the metered cost of calls served on the gateway's key.
	// 1 means at cost.
	Markup float64 `json:"markup,omitempty"`
	// MinChargeUSD is the floor for one such call. It exists because a model
	// with no price entry meters at zero, and a plan that bills a multiple of
	// zero would hand out the gateway's credentials for nothing.
	MinChargeUSD float64 `json:"min_charge_usd,omitempty"`
	// Providers are the globs whose gateway credential this plan unlocks.
	// Empty unlocks none, "*" unlocks all.
	Providers []string `json:"providers,omitempty"`

	// Renewals counts terms bought, so the console can say "since March"
	// without keeping every past term.
	Renewals int `json:"renewals,omitempty"`
}

// Active reports whether the subscription is in force at t.
func (s Subscription) Active(t time.Time) bool {
	return s.PlanID != "" && t.Before(s.ExpiresAt)
}

// Covers reports whether this subscription unlocks a provider's gateway key.
func (s Subscription) Covers(provider string) bool {
	provider = normProv(provider)
	for _, glob := range s.Providers {
		if matchGlob(strings.ToLower(strings.TrimSpace(glob)), provider) {
			return true
		}
	}
	return false
}

// matchGlob is the same trailing-* match the alias allowlists use. Kept local
// and deliberately simple: a provider name has no separators to be clever about.
func matchGlob(pattern, s string) bool {
	switch {
	case pattern == "":
		return false
	case pattern == "*":
		return true
	case strings.HasSuffix(pattern, "*"):
		return strings.HasPrefix(s, strings.TrimSuffix(pattern, "*"))
	default:
		return pattern == s
	}
}

// ErrInsufficientBalance is returned when a user cannot afford a term. It is a
// named error because the console distinguishes "top up first" from "that plan
// does not exist", and a string compare on the message would not survive
// translation.
var ErrInsufficientBalance = errors.New("adminstore: balance too low for this plan")

// Subscribe charges a term to the user's balance and starts (or extends) their
// subscription. Buying the same plan again adds a term to the end of the
// current one rather than resetting it, so an early renewal never costs the
// buyer the days they had left.
//
// Switching plans replaces the terms outright from now: a user moving up should
// get the new plan's rate immediately, and a user moving down has said they
// want the cheaper one.
func (s *Store) Subscribe(owner string, p Subscription, days int) (Subscription, error) {
	owner = normEmail(owner)
	if owner == "" || strings.TrimSpace(p.PlanID) == "" {
		return Subscription{}, errors.New("adminstore: need an owner and a plan")
	}
	if days <= 0 {
		return Subscription{}, errors.New("adminstore: a plan term must be at least a day")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	u := s.st.Users[owner]
	if u == nil {
		return Subscription{}, fmt.Errorf("adminstore: no such user %q", owner)
	}
	// Only a purchase checks the balance. A grant — an admin making good on an
	// outage, or a customer who paid another way — passes a zero price, and the
	// customer owed a term is exactly the one whose balance went negative on
	// the way in. Checking regardless would refuse the support path in the only
	// case it exists for.
	if p.PriceUSD > 0 && u.Balance < p.PriceUSD {
		return Subscription{}, ErrInsufficientBalance
	}

	now := time.Now().UTC()
	cur := u.Subscription
	next := p
	next.StartedAt = now
	next.ExpiresAt = now.AddDate(0, 0, days)
	if cur != nil && cur.PlanID == p.PlanID {
		// Buying the same plan again is a renewal whether or not the last term
		// is still running. Counting it only while active would make someone
		// who came back after a lapsed month a first-time buyer.
		next.Renewals = cur.Renewals + 1
		if cur.Active(now) {
			// Still running: extend rather than restart, so an early renewal
			// never costs the buyer the days they had left.
			next.StartedAt = cur.StartedAt
			next.ExpiresAt = cur.ExpiresAt.AddDate(0, 0, days)
		}
	}

	u.Balance -= p.PriceUSD
	u.Subscription = &next
	s.dirty = true
	if err := s.save(); err != nil {
		return Subscription{}, err
	}
	return next, nil
}

// Unsubscribe ends a subscription now. No refund is computed: the term was
// bought, and a partial refund is a decision for a person, not a handler.
func (s *Store) Unsubscribe(owner string) error {
	owner = normEmail(owner)
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.st.Users[owner]
	if u == nil || u.Subscription == nil {
		return nil
	}
	u.Subscription = nil
	s.dirty = true
	return s.save()
}

// ActiveSubscription returns a user's subscription if it is in force. Expiry is
// evaluated here, on read, which is what makes a lapse need no sweeper.
func (s *Store) ActiveSubscription(owner string) (Subscription, bool) {
	owner = normEmail(owner)
	s.mu.RLock()
	defer s.mu.RUnlock()
	u := s.st.Users[owner]
	if u == nil || u.Subscription == nil {
		return Subscription{}, false
	}
	sub := *u.Subscription
	if !sub.Active(time.Now().UTC()) {
		return sub, false
	}
	return sub, true
}

// SubscriptionOf returns what a user holds whether or not it is still in force,
// for a console that must show "expired on the 3rd" and not an empty screen.
func (s *Store) SubscriptionOf(owner string) (Subscription, bool) {
	owner = normEmail(owner)
	s.mu.RLock()
	defer s.mu.RUnlock()
	u := s.st.Users[owner]
	if u == nil || u.Subscription == nil {
		return Subscription{}, false
	}
	return *u.Subscription, true
}
