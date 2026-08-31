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
	})
	return s.save()
}

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
	email = strings.TrimSpace(strings.ToLower(email))

	s.mu.Lock()
	defer s.mu.Unlock()

	u := s.st.Users[email]
	if u == nil {
		return 0, ErrPaymentNotFound
	}

	for i := range u.Payments {
		p := &u.Payments[i]
		if p.ID != invoice {
			continue
		}
		if p.Status == "success" {
			return u.Balance, ErrPaymentAlreadySettled
		}
		p.Status = "success"
		u.Balance += p.Amount
		return u.Balance, s.save()
	}
	return 0, ErrPaymentNotFound
}

// FailPayment marks a pending payment as failed, crediting nothing. Used when
// the gateway says the money did not move, so the buyer's history shows the
// attempt rather than silently dropping it.
func (s *Store) FailPayment(email, invoice string) error {
	email = strings.TrimSpace(strings.ToLower(email))

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
		p.Status = "failed"
		return s.save()
	}
	return ErrPaymentNotFound
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
