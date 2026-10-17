package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"nabugate/internal/adminstore"
	"nabugate/internal/config"
)

// verdictBridge is a NabuPay stub whose verify answer is set per invoice, and
// whose invoices are numbered so several checkouts can be told apart.
type verdictBridge struct {
	mu      sync.Mutex
	n       int
	answers map[string]map[string]any // invoice -> verify body
}

func (b *verdictBridge) set(invoice string, body map[string]any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.answers[invoice] = body
}

func newVerdictBridge(t *testing.T) (*verdictBridge, *httptest.Server) {
	t.Helper()
	b := &verdictBridge{answers: map[string]map[string]any{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		defer b.mu.Unlock()
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/pay/checkout":
			b.n++
			inv := fmt.Sprintf("INV-%d", b.n)
			json.NewEncoder(w).Encode(map[string]any{"success": true, "invoice_number": inv, "checkout_url": "https://pay.example/" + inv})
		case strings.HasPrefix(r.URL.Path, "/api/v1/pay/verify/"):
			inv := strings.TrimPrefix(r.URL.Path, "/api/v1/pay/verify/")
			body, ok := b.answers[inv]
			if !ok {
				body = map[string]any{"success": true, "status": "pending"}
			}
			json.NewEncoder(w).Encode(body)
		case r.URL.Path == "/api/v1/pay/gateways":
			json.NewEncoder(w).Encode(map[string]any{"success": true, "gateways": []map[string]string{{"slug": "zarinpal", "label": "Zarinpal"}}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	return b, srv
}

var monthPlan = config.Plan{ID: "gateway", Name: "Gateway keys", PriceUSD: 9, Days: 30, Providers: []string{"*"}, Markup: 1.4, IncludesCreditUSD: 2}

func call(t *testing.T, h http.HandlerFunc, method, path, body, email string) (int, map[string]any) {
	t.Helper()
	r := asUser(httptest.NewRequest(method, path, strings.NewReader(body)), email)
	w := httptest.NewRecorder()
	h(w, r)
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	return w.Code, got
}

func errCode(got map[string]any) string {
	e, _ := got["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}

func signUp(t *testing.T, store *adminstore.Store, email string) {
	t.Helper()
	if err := store.SignupUser(email, "a-long-password"); err != nil {
		t.Fatal(err)
	}
}

// A top-up outside the bounds never reaches the bridge as an invoice.
func TestTopUpBoundsAreEnforcedByTheServer(t *testing.T) {
	_, bridge := newVerdictBridge(t)
	defer bridge.Close()
	s, _ := payServer(t, bridge.URL)
	for _, amount := range []string{"0.0001", "0.5", "5000.01", "1e9", "-3"} {
		code, got := call(t, s.rechargeMe, http.MethodPost, "/api/me/recharge", `{"amount":`+amount+`}`, "buyer@example.com")
		if code != http.StatusBadRequest || errCode(got) != "amount_out_of_range" {
			t.Errorf("amount %s: %d %v", amount, code, got)
		}
	}
}

// The bridge saying fewer dollars arrived than were invoiced is not a payment
// of the invoice, whatever its status word is.
func TestAnUnderpaidInvoiceIsNotCredited(t *testing.T) {
	b, bridge := newVerdictBridge(t)
	defer bridge.Close()
	s, store := payServer(t, bridge.URL)

	call(t, s.rechargeMe, http.MethodPost, "/api/me/recharge", `{"amount":25}`, "buyer@example.com")
	b.set("INV-1", map[string]any{"success": true, "status": "paid", "amount_usd": 10})
	_, got := call(t, s.settleMyPayments, http.MethodPost, "/api/me/payments/settle", "", "buyer@example.com")

	if got["credited"] != false {
		t.Fatalf("an underpaid invoice was credited: %v", got)
	}
	if u := store.GetUser("buyer@example.com"); u.Balance != 0 || u.Payments[0].Status != "underpaid" {
		t.Fatalf("balance %v, row %+v", u.Balance, u.Payments[0])
	}
}

// A paid amount within a cent of the invoice is the bank's rounding, not a
// short payment.
func TestARoundedPaymentIsCredited(t *testing.T) {
	b, bridge := newVerdictBridge(t)
	defer bridge.Close()
	s, store := payServer(t, bridge.URL)

	call(t, s.rechargeMe, http.MethodPost, "/api/me/recharge", `{"amount":25}`, "buyer@example.com")
	b.set("INV-1", map[string]any{"success": true, "status": "paid", "amount_usd": 24.995, "gateway": "zarinpal"})
	call(t, s.settleMyPayments, http.MethodPost, "/api/me/payments/settle", "", "buyer@example.com")
	if u := store.GetUser("buyer@example.com"); u.Balance != 25 || u.Payments[0].Gateway != "zarinpal" {
		t.Fatalf("balance %v, row %+v", u.Balance, u.Payments[0])
	}
}

// Abandoned checkouts used to stay pending forever and, past ten of them, push
// the one real payment out of the list that is asked about.
func TestFailedInvoicesCloseAndDoNotHideARealPayment(t *testing.T) {
	b, bridge := newVerdictBridge(t)
	defer bridge.Close()
	s, store := payServer(t, bridge.URL)

	for i := 1; i <= 25; i++ {
		call(t, s.rechargeMe, http.MethodPost, "/api/me/recharge", `{"amount":5}`, "buyer@example.com")
		if i < 25 {
			b.set(fmt.Sprintf("INV-%d", i), map[string]any{"success": true, "status": "cancelled"})
		}
	}
	b.set("INV-25", map[string]any{"success": true, "status": "paid"})

	// Two visits: the first closes the first twenty, the second reaches the rest.
	call(t, s.settleMyPayments, http.MethodPost, "/api/me/payments/settle", "", "buyer@example.com")
	_, got := call(t, s.settleMyPayments, http.MethodPost, "/api/me/payments/settle", "", "buyer@example.com")
	if got["credited"] != true {
		t.Fatalf("the real payment was never credited: %v", got)
	}
	if u := store.GetUser("buyer@example.com"); u.Balance != 5 {
		t.Fatalf("balance %v, want 5", u.Balance)
	}
	if left := store.PendingPayments("buyer@example.com", 100); len(left) != 0 {
		t.Fatalf("%d invoices still pending", len(left))
	}
}

// "Pay the difference and activate" is one action: the top-up is sized to the
// plan and the plan is bought the moment the money lands.
func TestATopUpForAPlanBuysThePlanWhenPaid(t *testing.T) {
	b, bridge := newVerdictBridge(t)
	defer bridge.Close()
	s, store := payServer(t, bridge.URL)
	s.SetPlans([]config.Plan{monthPlan})
	signUp(t, store, "buyer@example.com")
	store.AddPayment("buyer@example.com", 3.5, "success", "manual-1")

	code, got := call(t, s.rechargeMe, http.MethodPost, "/api/me/recharge", `{"amount":1,"plan":"gateway"}`, "buyer@example.com")
	if code != http.StatusOK || got["amount"] != 5.5 {
		t.Fatalf("a plan top-up should charge the shortfall 5.5: %d %v", code, got)
	}
	b.set("INV-1", map[string]any{"success": true, "status": "paid", "amount_usd": 5.5})
	_, got = call(t, s.settleMyPayments, http.MethodPost, "/api/me/payments/settle", "", "buyer@example.com")
	if got["subscribed"] == nil {
		t.Fatalf("plan not activated: %v", got)
	}
	u := store.GetUser("buyer@example.com")
	// 3.5 + 5.5 paid − 9 plan + 2 bundled credit.
	if u.Balance != 2 || u.Subscription == nil || u.Subscription.PlanID != "gateway" {
		t.Fatalf("balance %v, sub %+v", u.Balance, u.Subscription)
	}
	// Settling again neither re-credits nor re-buys.
	call(t, s.settleMyPayments, http.MethodPost, "/api/me/payments/settle", "", "buyer@example.com")
	if u := store.GetUser("buyer@example.com"); u.Balance != 2 || u.Subscription.Renewals != 0 {
		t.Fatalf("second settle moved money: balance %v renewals %d", u.Balance, u.Subscription.Renewals)
	}
}

// A plan bought from the balance leaves a history row for the charge and one
// for the bundled credit, and a double click is one purchase.
func TestBuyingAPlanIsRecordedAndIdempotent(t *testing.T) {
	_, bridge := newVerdictBridge(t)
	defer bridge.Close()
	s, store := payServer(t, bridge.URL)
	s.SetPlans([]config.Plan{monthPlan})
	signUp(t, store, "buyer@example.com")
	store.AddPayment("buyer@example.com", 20, "success", "manual-1")

	sub := func() (int, map[string]any) {
		r := asUser(httptest.NewRequest(http.MethodPost, "/api/plans/gateway/subscribe", strings.NewReader(`{"idempotency_key":"click-1"}`)), "buyer@example.com")
		r.SetPathValue("id", "gateway")
		w := httptest.NewRecorder()
		s.subscribe(w, r)
		var got map[string]any
		json.Unmarshal(w.Body.Bytes(), &got)
		return w.Code, got
	}
	for i := 0; i < 3; i++ {
		if code, got := sub(); code != http.StatusOK {
			t.Fatalf("subscribe %d: %d %v", i, code, got)
		}
	}
	u := store.GetUser("buyer@example.com")
	if u.Balance != 13 {
		t.Fatalf("balance %v, want 20 − 9 + 2 = 13 (charged once)", u.Balance)
	}
	var plan, credit int
	for _, p := range u.Payments {
		switch p.Kind {
		case "plan":
			plan++
			if p.Amount != -9 {
				t.Errorf("plan row amount %v", p.Amount)
			}
		case "plan-credit":
			credit++
		}
	}
	if plan != 1 || credit != 1 {
		t.Fatalf("history rows: %d plan, %d credit; %+v", plan, credit, u.Payments)
	}
}

// Switching to a different plan ends the current term; that has to be the
// buyer's informed choice, not a side effect of a click.
func TestSwitchingPlansNeedsConfirmation(t *testing.T) {
	_, bridge := newVerdictBridge(t)
	defer bridge.Close()
	s, store := payServer(t, bridge.URL)
	other := monthPlan
	other.ID, other.Name, other.PriceUSD, other.IncludesCreditUSD = "pro", "Pro", 5, 0
	s.SetPlans([]config.Plan{monthPlan, other})
	signUp(t, store, "buyer@example.com")
	store.AddPayment("buyer@example.com", 50, "success", "manual-1")

	buy := func(id, body string) (int, map[string]any) {
		r := asUser(httptest.NewRequest(http.MethodPost, "/api/plans/"+id+"/subscribe", strings.NewReader(body)), "buyer@example.com")
		r.SetPathValue("id", id)
		w := httptest.NewRecorder()
		s.subscribe(w, r)
		var got map[string]any
		json.Unmarshal(w.Body.Bytes(), &got)
		return w.Code, got
	}
	if code, _ := buy("gateway", `{}`); code != http.StatusOK {
		t.Fatal(code)
	}
	code, got := buy("pro", `{}`)
	if code != http.StatusConflict || errCode(got) != "plan_switch_needs_confirm" {
		t.Fatalf("switch without confirmation: %d %v", code, got)
	}
	if e := got["error"].(map[string]any); e["days_left"] != float64(30) {
		t.Errorf("days_left = %v", e["days_left"])
	}
	if code, _ := buy("pro", `{"confirm_switch":true}`); code != http.StatusOK {
		t.Fatalf("confirmed switch refused: %d", code)
	}
}

// Short of money: the refusal says by how much, so the panel can offer to pay
// exactly that.
func TestInsufficientBalanceSaysTheShortfall(t *testing.T) {
	_, bridge := newVerdictBridge(t)
	defer bridge.Close()
	s, store := payServer(t, bridge.URL)
	s.SetPlans([]config.Plan{monthPlan})
	signUp(t, store, "buyer@example.com")
	store.AddPayment("buyer@example.com", 4, "success", "manual-1")

	r := asUser(httptest.NewRequest(http.MethodPost, "/api/plans/gateway/subscribe", nil), "buyer@example.com")
	r.SetPathValue("id", "gateway")
	w := httptest.NewRecorder()
	s.subscribe(w, r)
	var got map[string]any
	json.Unmarshal(w.Body.Bytes(), &got)
	e, _ := got["error"].(map[string]any)
	if w.Code != http.StatusPaymentRequired || e["code"] != "insufficient_balance" || e["shortfall_usd"] != float64(5) {
		t.Fatalf("%d %v", w.Code, got)
	}
}

// An operator's top-up to a mistyped address used to create an account that
// held the money.
func TestAdminTopUpNeedsARealAccountAndAPositiveAmount(t *testing.T) {
	_, bridge := newVerdictBridge(t)
	defer bridge.Close()
	s, store := payServer(t, bridge.URL)
	signUp(t, store, "buyer@example.com")

	if code, _ := call(t, s.adminRechargeUser, http.MethodPost, "/api/users/recharge", `{"email":"buyr@example.com","amount":10}`, "admin"); code != http.StatusNotFound {
		t.Errorf("unknown account: %d", code)
	}
	if store.GetUser("buyr@example.com") != nil {
		t.Error("a typo created an account")
	}
	if code, _ := call(t, s.adminRechargeUser, http.MethodPost, "/api/users/recharge", `{"email":"buyer@example.com","amount":-10}`, "admin"); code != http.StatusBadRequest {
		t.Errorf("negative amount: %d", code)
	}
	if code, got := call(t, s.adminRechargeUser, http.MethodPost, "/api/users/recharge", `{"email":" Buyer@Example.com ","amount":10}`, "admin"); code != http.StatusOK || got["balance"] != float64(10) {
		t.Errorf("valid top-up: %d %v", code, got)
	}
	if u := store.GetUser("buyer@example.com"); u.Payments[0].Kind != "admin" || u.Payments[0].Status != "success" {
		t.Errorf("row %+v", u.Payments[0])
	}
}
