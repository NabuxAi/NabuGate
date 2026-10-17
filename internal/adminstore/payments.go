package adminstore

import (
	"errors"
	"strings"
	"time"
)

var (
	// ErrPaymentNotFound is returned when no pending payment matches the
	// invoice, for the caller who owns it.
	ErrPaymentNotFound = errors.New("no pending payment with that invoice number")

	// ErrPaymentAlreadySettled means the invoice was credited before. Not an
	// error the buyer caused: refreshing the page they return from after paying
	// is the ordinary way to reach it.
	ErrPaymentAlreadySettled = errors.New("this payment was already credited")
)

// StartPayment records an invoice as pending against the buyer, crediting
// nothing.
//
// Written before the buyer leaves for the gateway, which is what binds the
// invoice number to an account. Without that binding, settling would have to
// take the buyer's word for whose invoice it is, and an invoice number seen in
// somebody else's URL would credit the person who typed it.
func (s *Store) StartPayment(email string, amount float64, invoice string) error {
	return s.StartPaymentFor(email, amount, invoice, "")
}

// StartPaymentFor is StartPayment carrying a plan to buy once the money lands.
func (s *Store) StartPaymentFor(email string, amount float64, invoice, planID string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || invoice == "" {
		return errors.New("a payment needs both an account and an invoice number")
	}
	if amount <= 0 {
		return errors.New("a payment must be for more than nothing")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.st.Users == nil {
		s.st.Users = make(map[string]*User)
	}
	// An invoice number is unique across the deployment, so a repeat is either
	// a retry or somebody reusing a number, and neither should add a second row.
	for _, u := range s.st.Users {
		for _, p := range u.Payments {
			if p.ID == invoice {
				return errors.New("that invoice number is already recorded")
			}
		}
	}

	u := s.st.Users[email]
	if u == nil {
		u = &User{Email: email}
		s.st.Users[email] = u
	}
	u.Payments = append(u.Payments, Payment{
		ID:        invoice,
		Amount:    amount,
		Status:    "pending",
		CreatedAt: time.Now(),
		Kind:      "topup",
		PlanID:    planID,
	})
	return s.save()
}

// ErrPaymentUnderpaid means the bridge reported less money collected than the
// invoice asked for. Nothing is credited; the row is marked "underpaid" for a
// person to look at, because the right answer (credit what arrived, chase the
// rest, refund) is a decision about money, not a rule.
var ErrPaymentUnderpaid = errors.New("the gateway collected less than this invoice asked for")

// underpaidTolerance absorbs the rounding of a toman charge converted back to
// dollars: a cent either way is the bank's arithmetic, not a short payment.
const underpaidTolerance = 0.01

// SettlePayment credits a pending payment, once.
//
// The amount comes from the stored record rather than from the caller: the
// balance must reflect the invoice that was actually raised, not a number that
// arrives with the request to settle it.
//
// email must be the account the invoice was started against. A signed-in
// caller settling somebody else's invoice gets ErrPaymentNotFound, which is
// also what they get for an invoice that does not exist — the two are the same
// answer on purpose.
func (s *Store) SettlePayment(email, invoice string) (balance float64, err error) {
	b, _, err := s.SettleVerified(email, invoice, 0, "")
	return b, err
}

// SettleVerified credits a pending payment once the bridge has said it is
// paid, checking what the bridge says was collected (paidUSD, zero when it did
// not say) against what the invoice asked for. It returns the payment as it
// now stands, so the caller can act on a plan the top-up was raised for.
//
// Only a pending row is credited. A row already marked failed, expired or
// underpaid is not asked about again, and a late "paid" for one is a support
// case rather than something to credit automatically.
func (s *Store) SettleVerified(email, invoice string, paidUSD float64, gateway string) (float64, Payment, error) {
	email = strings.TrimSpace(strings.ToLower(email))

	s.mu.Lock()
	defer s.mu.Unlock()

	u := s.st.Users[email]
	if u == nil {
		return 0, Payment{}, ErrPaymentNotFound
	}

	for i := range u.Payments {
		p := &u.Payments[i]
		if p.ID != invoice {
			continue
		}
		switch p.Status {
		case "success":
			return u.Balance, *p, ErrPaymentAlreadySettled
		case "pending":
		default:
			return u.Balance, *p, ErrPaymentNotFound
		}
		before := *p
		if gateway != "" {
			p.Gateway = gateway
		}
		if paidUSD > 0 {
			p.PaidUSD = paidUSD
			if paidUSD+underpaidTolerance < p.Amount {
				p.Status = "underpaid"
				if err := s.save(); err != nil {
					*p = before
					return u.Balance, before, err
				}
				return u.Balance, *p, ErrPaymentUnderpaid
			}
		}
		p.Status = "success"
		u.Balance += p.Amount
		if err := s.save(); err != nil {
			// Undo in memory too: the handler reports "not credited", and a
			// balance that is credited here but not on disk would say otherwise
			// until the next flush decided which was true.
			u.Balance -= p.Amount
			*p = before
			return u.Balance, before, err
		}
		return u.Balance, *p, nil
	}
	return 0, Payment{}, ErrPaymentNotFound
}

// FailPayment marks a pending payment as failed, crediting nothing. Used when
// the gateway says the money did not move, so the buyer's history shows the
// attempt rather than silently dropping it.
func (s *Store) FailPayment(email, invoice string) error {
	return s.ClosePayment(email, invoice, "failed")
}

// ClosePayment ends a payment that can no longer be paid — "failed" when the
// bridge said so, "expired" when it was abandoned past the point of waiting.
// A credited payment is never walked backwards.
func (s *Store) ClosePayment(email, invoice, status string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	if status != "failed" && status != "expired" {
		return errors.New("a payment closes as failed or expired")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	u := s.st.Users[email]
	if u == nil {
		return ErrPaymentNotFound
	}
	for i := range u.Payments {
		p := &u.Payments[i]
		if p.ID != invoice {
			continue
		}
		if p.Status == "success" {
			// Never walk a credited payment backwards from a later callback.
			return ErrPaymentAlreadySettled
		}
		p.Status = status
		return s.save()
	}
	return ErrPaymentNotFound
}

// PendingMaxAge is how long an unconfirmed invoice is still asked about. A
// bank reverses an unconfirmed charge within 72 hours and a crypto payment
// confirms within hours; a week covers both with room, and past it the row is
// marked expired so abandoned checkouts stop crowding out real ones.
const PendingMaxAge = 7 * 24 * time.Hour

// ExpireStalePayments marks this account's pending invoices older than
// PendingMaxAge as expired and returns how many it closed.
func (s *Store) ExpireStalePayments(email string, now time.Time) int {
	email = strings.TrimSpace(strings.ToLower(email))
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.st.Users[email]
	if u == nil {
		return 0
	}
	n := 0
	for i := range u.Payments {
		p := &u.Payments[i]
		if p.Status == "pending" && now.Sub(p.CreatedAt) > PendingMaxAge {
			p.Status = "expired"
			n++
		}
	}
	if n > 0 {
		if err := s.save(); err != nil {
			s.dirty = true
		}
	}
	return n
}

// PendingPayments lists the invoices this account started and has not
// completed, newest first.
//
// This is what lets the panel finish a payment without being told which one:
// the payer comes back from the gateway with a query string we do not trust
// and no invoice number we could trust either, so the answer to "what was I
// paying for" has to come from what we recorded before they left.
func (s *Store) PendingPayments(email string, limit int) []Payment {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || limit <= 0 {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	u := s.st.Users[email]
	if u == nil {
		return nil
	}

	// OLDEST first. Walking newest-first meant a genuinely paid invoice could be
	// pushed out of the caller's limit by later abandoned attempts and never
	// settled again — the one invoice with money behind it is the one that gets
	// dropped, because every new top-up click raises a fresh pending row.
	out := make([]Payment, 0, limit)
	for i := 0; i < len(u.Payments) && len(out) < limit; i++ {
		if u.Payments[i].Status == "pending" {
			out = append(out, u.Payments[i])
		}
	}
	return out
}
