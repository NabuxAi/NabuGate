package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"nabugate/internal/adminstore"
	"nabugate/internal/nabupay"
)

// bridgeStub stands in for NabuDesk's NabuPay bridge. paid decides what the
// gateway is said to have done.
func bridgeStub(t *testing.T, paid *bool, calls *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/pay/checkout":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"service_name":"gate"`) {
				t.Errorf("checkout body does not identify the service: %s", body)
			}
			json.NewEncoder(w).Encode(map[string]any{
				"success": true, "invoice_number": "INV-TEST", "checkout_url": "https://pay.example/go",
			})
		case strings.HasPrefix(r.URL.Path, "/api/v1/pay/verify/"):
			*calls++
			status := "pending"
			if *paid {
				status = "paid"
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "status": status})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func payServer(t *testing.T, bridge string) (*Server, *adminstore.Store) {
	t.Helper()
	store, err := adminstore.Open(filepath.Join(t.TempDir(), "admin.json"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	s := &Server{admin: store, log: slog.New(slog.DiscardHandler)}
	s.pay = nabupay.New(bridge, "gate", "a-secret")
	s.payGateway = "zarinpal"
	return s, store
}

func asUser(r *http.Request, email string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), consoleEmailCtxKey{}, email))
}

// The whole point of the change: asking to top up must not move the balance.
// It used to add the requested number and answer "paid", which made the button
// a way to give yourself credit.
func TestStartingATopUpCreditsNothing(t *testing.T) {
	paid, calls := false, 0
	bridge := bridgeStub(t, &paid, &calls)
	defer bridge.Close()
	s, store := payServer(t, bridge.URL)

	r := asUser(httptest.NewRequest(http.MethodPost, "/api/me/recharge",
		strings.NewReader(`{"amount":25}`)), "buyer@example.com")
	w := httptest.NewRecorder()
	s.rechargeMe(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	var got map[string]any
	json.Unmarshal(w.Body.Bytes(), &got)
	if got["checkout_url"] != "https://pay.example/go" {
		t.Errorf("no checkout url in %v", got)
	}
	// The response must not carry a balance at all — a balance here is what a
	// panel would render as "topped up".
	if _, leaked := got["balance"]; leaked {
		t.Errorf("starting a payment answered with a balance: %v", got)
	}
	if u := store.GetUser("buyer@example.com"); u.Balance != 0 {
		t.Fatalf("starting a payment credited %v", u.Balance)
	}
}

func TestAnUnpaidInvoiceIsNotCredited(t *testing.T) {
	paid, calls := false, 0
	bridge := bridgeStub(t, &paid, &calls)
	defer bridge.Close()
	s, store := payServer(t, bridge.URL)

	s.rechargeMe(httptest.NewRecorder(), asUser(
		httptest.NewRequest(http.MethodPost, "/api/me/recharge", strings.NewReader(`{"amount":25}`)),
		"buyer@example.com"))

	w := httptest.NewRecorder()
	s.settleMyPayments(w, asUser(httptest.NewRequest(http.MethodPost, "/api/me/payments/settle", nil), "buyer@example.com"))

	var got map[string]any
	json.Unmarshal(w.Body.Bytes(), &got)
	if got["credited"] != false {
		t.Errorf("an unpaid invoice reported credited: %v", got)
	}
	if u := store.GetUser("buyer@example.com"); u.Balance != 0 {
		t.Fatalf("balance = %v for an invoice the gateway calls pending", u.Balance)
	}
}

// The return page is one a payer can refresh, bookmark, or have replayed.
func TestAConfirmedPaymentIsCreditedExactlyOnce(t *testing.T) {
	paid, calls := false, 0
	bridge := bridgeStub(t, &paid, &calls)
	defer bridge.Close()
	s, store := payServer(t, bridge.URL)

	s.rechargeMe(httptest.NewRecorder(), asUser(
		httptest.NewRequest(http.MethodPost, "/api/me/recharge", strings.NewReader(`{"amount":25}`)),
		"buyer@example.com"))

	paid = true
	for i := 0; i < 4; i++ {
		w := httptest.NewRecorder()
		s.settleMyPayments(w, asUser(httptest.NewRequest(http.MethodPost, "/api/me/payments/settle", nil), "buyer@example.com"))
		if w.Code != http.StatusOK {
			t.Fatalf("settle %d: status %d", i, w.Code)
		}
	}

	if u := store.GetUser("buyer@example.com"); u.Balance != 25 {
		t.Fatalf("balance = %v after four settles of one 25 payment", u.Balance)
	}
}

// Settling looks only at what this account started, so one signed-in user can
// never finish — or be credited for — another's payment.
func TestSettlingOnlyEverTouchesTheCallersOwnInvoices(t *testing.T) {
	paid, calls := true, 0
	bridge := bridgeStub(t, &paid, &calls)
	defer bridge.Close()
	s, store := payServer(t, bridge.URL)

	s.rechargeMe(httptest.NewRecorder(), asUser(
		httptest.NewRequest(http.MethodPost, "/api/me/recharge", strings.NewReader(`{"amount":100}`)),
		"buyer@example.com"))

	w := httptest.NewRecorder()
	s.settleMyPayments(w, asUser(httptest.NewRequest(http.MethodPost, "/api/me/payments/settle", nil), "thief@example.com"))

	var got map[string]any
	json.Unmarshal(w.Body.Bytes(), &got)
	if got["credited"] != false {
		t.Errorf("another account was credited: %v", got)
	}
	if u := store.GetUser("thief@example.com"); u != nil && u.Balance != 0 {
		t.Fatalf("the other account holds %v", u.Balance)
	}
	// And the buyer can still complete their own.
	w = httptest.NewRecorder()
	s.settleMyPayments(w, asUser(httptest.NewRequest(http.MethodPost, "/api/me/payments/settle", nil), "buyer@example.com"))
	if u := store.GetUser("buyer@example.com"); u.Balance != 100 {
		t.Fatalf("the buyer's own balance is %v, want 100", u.Balance)
	}
}

// A deployment with no gateway says so rather than falling back to crediting.
func TestWithNoGatewayConfiguredTopUpIsRefusedRatherThanFaked(t *testing.T) {
	store, err := adminstore.Open(filepath.Join(t.TempDir(), "admin.json"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	s := &Server{admin: store, log: slog.New(slog.DiscardHandler)} // s.pay is nil

	w := httptest.NewRecorder()
	s.rechargeMe(w, asUser(httptest.NewRequest(http.MethodPost, "/api/me/recharge",
		strings.NewReader(`{"amount":25}`)), "buyer@example.com"))

	if w.Code != http.StatusNotImplemented {
		t.Errorf("status %d, want 501", w.Code)
	}
	if u := store.GetUser("buyer@example.com"); u != nil && u.Balance != 0 {
		t.Fatalf("an unconfigured deployment credited %v", u.Balance)
	}
}

func TestATopUpForNothingIsRefused(t *testing.T) {
	paid, calls := true, 0
	bridge := bridgeStub(t, &paid, &calls)
	defer bridge.Close()
	s, store := payServer(t, bridge.URL)

	for _, body := range []string{`{"amount":0}`, `{"amount":-50}`} {
		w := httptest.NewRecorder()
		s.rechargeMe(w, asUser(httptest.NewRequest(http.MethodPost, "/api/me/recharge",
			strings.NewReader(body)), "buyer@example.com"))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", body, w.Code)
		}
	}
	if u := store.GetUser("buyer@example.com"); u != nil && u.Balance != 0 {
		t.Fatalf("balance = %v", u.Balance)
	}
}
