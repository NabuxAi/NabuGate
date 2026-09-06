package adminstore

import (
	"testing"
	"time"
)

func subStore(t *testing.T, balance float64) *Store {
	t.Helper()
	s, err := Open(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SignupUser("me@example.com", "hunter2hunter2"); err != nil {
		t.Fatal(err)
	}
	s.AddPayment("me@example.com", balance, "success", "seed")
	return s
}

func plan(price float64) Subscription {
	return Subscription{PlanID: "gateway", Name: "Gateway keys", PriceUSD: price,
		Markup: 1.4, MinChargeUSD: 0.0002, Providers: []string{"*"}}
}

// The subscription fee comes out of the same balance the usage does. That is
// the design, not an accident: one wallet, and a user can see exactly what the
// month cost them.
func TestSubscribeDebitsTheBalance(t *testing.T) {
	s := subStore(t, 20)
	sub, err := s.Subscribe("me@example.com", plan(9), 30)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if got := s.GetUser("me@example.com").Balance; got != 11 {
		t.Errorf("balance = %v, want 11 after a $9 plan", got)
	}
	if !sub.Active(time.Now().UTC()) {
		t.Error("a subscription bought now is not active")
	}
	if got, ok := s.ActiveSubscription("me@example.com"); !ok || got.Markup != 1.4 {
		t.Errorf("stored subscription = %+v, ok=%v", got, ok)
	}
}

// The plan's terms are copied in at purchase. Editing config afterwards must
// change what new buyers get and nothing about what someone is already paying.
func TestPlanTermsAreFrozenAtPurchase(t *testing.T) {
	s := subStore(t, 20)
	if _, err := s.Subscribe("me@example.com", plan(9), 30); err != nil {
		t.Fatal(err)
	}
	got, _ := s.ActiveSubscription("me@example.com")
	if got.Markup != 1.4 || got.MinChargeUSD != 0.0002 || len(got.Providers) != 1 {
		t.Errorf("subscription did not carry its own terms: %+v", got)
	}
}

// Renewing early must not cost the buyer the days they had left.
func TestRenewalExtendsRatherThanResets(t *testing.T) {
	s := subStore(t, 40)
	first, err := s.Subscribe("me@example.com", plan(9), 30)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Subscribe("me@example.com", plan(9), 30)
	if err != nil {
		t.Fatal(err)
	}
	if !second.ExpiresAt.After(first.ExpiresAt.AddDate(0, 0, 29)) {
		t.Errorf("expiry %v did not extend %v by a term", second.ExpiresAt, first.ExpiresAt)
	}
	if second.Renewals != 1 {
		t.Errorf("renewals = %d, want 1", second.Renewals)
	}
	if got := s.GetUser("me@example.com").Balance; got != 22 {
		t.Errorf("balance = %v, want both terms charged", got)
	}
}

// Selling access the buyer cannot pay for would leave them holding a plan and
// no credit to use it with.
func TestSubscribeRefusesWhenTheBalanceIsShort(t *testing.T) {
	s := subStore(t, 3)
	if _, err := s.Subscribe("me@example.com", plan(9), 30); err != ErrInsufficientBalance {
		t.Fatalf("err = %v, want ErrInsufficientBalance", err)
	}
	if got := s.GetUser("me@example.com").Balance; got != 3 {
		t.Errorf("balance = %v; a refused purchase must not charge", got)
	}
	if _, ok := s.ActiveSubscription("me@example.com"); ok {
		t.Error("a refused purchase left a subscription behind")
	}
}

// Expiry is read at request time. Nothing sweeps, and a lapsed term must stop
// unlocking the moment it passes.
func TestExpiredSubscriptionStopsCounting(t *testing.T) {
	s := subStore(t, 20)
	if _, err := s.Subscribe("me@example.com", plan(9), 1); err != nil {
		t.Fatal(err)
	}
	// Reach in and age it, which is what a day passing looks like from here.
	s.mu.Lock()
	s.st.Users["me@example.com"].Subscription.ExpiresAt = time.Now().UTC().Add(-time.Minute)
	s.mu.Unlock()

	if _, ok := s.ActiveSubscription("me@example.com"); ok {
		t.Error("an expired subscription still reports active")
	}
	// Still visible, so the console can say when it lapsed instead of showing
	// a user who paid an empty screen.
	if _, ok := s.SubscriptionOf("me@example.com"); !ok {
		t.Error("the expired subscription vanished from the account")
	}
}

func TestCoversMatchesGlobs(t *testing.T) {
	for _, c := range []struct {
		globs []string
		prov  string
		want  bool
	}{
		{[]string{"*"}, "gemini", true},
		{[]string{"gem*"}, "gemini", true},
		{[]string{"gem*"}, "openai", false},
		{[]string{"openai"}, "OpenAI", true},
		{nil, "gemini", false},
		{[]string{""}, "gemini", false},
	} {
		got := Subscription{PlanID: "p", Providers: c.globs}.Covers(c.prov)
		if got != c.want {
			t.Errorf("Covers(%q) with %v = %v, want %v", c.prov, c.globs, got, c.want)
		}
	}
}

// Coming back after a lapse is still a renewal. Counting only while the term
// runs would make a returning customer a first-time buyer.
func TestRenewalAfterALapseStillCounts(t *testing.T) {
	s := subStore(t, 40)
	if _, err := s.Subscribe("me@example.com", plan(9), 30); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.st.Users["me@example.com"].Subscription.ExpiresAt = time.Now().UTC().Add(-time.Hour)
	s.mu.Unlock()

	again, err := s.Subscribe("me@example.com", plan(9), 30)
	if err != nil {
		t.Fatal(err)
	}
	if again.Renewals != 1 {
		t.Errorf("renewals = %d, want the lapsed term counted", again.Renewals)
	}
	// A fresh term, not an extension of one that already ended.
	if !again.ExpiresAt.After(time.Now().UTC().AddDate(0, 0, 29)) {
		t.Errorf("expiry %v; a lapsed renewal must start from now", again.ExpiresAt)
	}
}

// The grant path exists for the customer an outage left in the red. Checking a
// balance against a zero price would refuse exactly them.
func TestGrantIgnoresTheBalance(t *testing.T) {
	s := subStore(t, 0)
	s.mu.Lock()
	s.st.Users["me@example.com"].Balance = -2
	s.mu.Unlock()

	free := plan(0)
	free.PriceUSD = 0
	if _, err := s.Subscribe("me@example.com", free, 30); err != nil {
		t.Fatalf("granting a term to an overdrawn account: %v", err)
	}
	if got := s.GetUser("me@example.com").Balance; got != -2 {
		t.Errorf("balance = %v; a grant must not move it", got)
	}
	if _, ok := s.ActiveSubscription("me@example.com"); !ok {
		t.Error("the granted term did not take")
	}
}
