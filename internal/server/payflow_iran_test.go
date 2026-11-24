package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"nabugate/internal/nabupay"
)

// rateBridge is a NabuPay stub that answers a rate and records what checkout
// bodies and verify calls reached it.
type rateBridge struct {
	*httptest.Server
	mu       sync.Mutex
	rate     int    // 0 = no rate (503)
	invoice  string // number handed back on checkout
	bodies   []map[string]any
	verifies int
	paid     bool
}

func newRateBridge(t *testing.T, rate int, invoice string) *rateBridge {
	t.Helper()
	b := &rateBridge{rate: rate, invoice: invoice}
	b.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.mu.Lock()
		defer b.mu.Unlock()
		switch {
		case r.URL.Path == "/api/v1/pay/rate":
			if b.rate == 0 {
				w.WriteHeader(http.StatusServiceUnavailable)
				json.NewEncoder(w).Encode(map[string]any{"success": false})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "usd_toman": b.rate})
		case r.URL.Path == "/api/v1/pay/gateways":
			json.NewEncoder(w).Encode(map[string]any{"success": true, "gateways": []map[string]string{{"slug": "bank_payment", "label": "درگاه پرداخت بانکی"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/pay/checkout":
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			json.Unmarshal(raw, &body)
			b.bodies = append(b.bodies, body)
			json.NewEncoder(w).Encode(map[string]any{"success": true, "invoice_number": b.invoice, "checkout_url": "https://pay.example/" + b.invoice})
		case strings.HasPrefix(r.URL.Path, "/api/v1/pay/verify/"):
			b.verifies++
			status := "pending"
			if b.paid {
				status = "paid"
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "status": status, "amount_usd": 5})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(b.Close)
	return b
}

func (b *rateBridge) lastBody(t *testing.T) map[string]any {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.bodies) == 0 {
		t.Fatal("no checkout reached this bridge")
	}
	return b.bodies[len(b.bodies)-1]
}

func iranRecharge(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/me/recharge", strings.NewReader(body))
	r.Header.Set(mirrorHostHeader, "gate.nabuxai.ir")
	return asUser(r, "buyer@example.ir")
}

func TestIranCheckoutIsPersianAndInToman(t *testing.T) {
	main := newRateBridge(t, 289698, "INV-MAIN")
	s, _ := payServer(t, main.URL)

	w := httptest.NewRecorder()
	s.rechargeMe(w, iranRecharge(`{"amount":5}`))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	body := main.lastBody(t)
	if got := body["amount_toman"]; got != float64(1448490) {
		t.Errorf("amount_toman = %v, want 5 × 289698 = 1448490", got)
	}
	if got := body["amount_usd"]; got != float64(5) {
		t.Errorf("amount_usd = %v; the dollars credited must still be sent", got)
	}
	cust, _ := body["customer"].(map[string]any)
	if cust["country"] != "IR" || cust["locale"] != "fa" || cust["email"] != "buyer@example.ir" {
		t.Errorf("customer = %v, want country IR, locale fa and the payer's email", cust)
	}
	var res map[string]any
	json.Unmarshal(w.Body.Bytes(), &res)
	if res["amount_toman"] != float64(1448490) {
		t.Errorf("response amount_toman = %v", res["amount_toman"])
	}
}

func TestComCheckoutIsUnchanged(t *testing.T) {
	main := newRateBridge(t, 289698, "INV-MAIN")
	s, _ := payServer(t, main.URL)

	r := asUser(httptest.NewRequest(http.MethodPost, "/api/me/recharge", strings.NewReader(`{"amount":5}`)), "buyer@example.com")
	r.Header.Set(mirrorHostHeader, "gate.nabuxai.com")
	s.rechargeMe(httptest.NewRecorder(), r)

	body := main.lastBody(t)
	if _, ok := body["amount_toman"]; ok {
		t.Errorf("a .com checkout carried amount_toman: %v", body)
	}
	if _, ok := body["customer"]; ok {
		t.Errorf("a .com checkout carried customer: %v", body)
	}
}

// No rate from the bridge: no toman figure is invented, and the payment still
// starts (the bridge converts as it always has).
func TestIranCheckoutWithoutRateSendsNoToman(t *testing.T) {
	main := newRateBridge(t, 0, "INV-MAIN")
	s, _ := payServer(t, main.URL)

	w := httptest.NewRecorder()
	s.rechargeMe(w, iranRecharge(`{"amount":5}`))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	if v, ok := main.lastBody(t)["amount_toman"]; ok {
		t.Errorf("amount_toman = %v with no rate available", v)
	}
}

// With NABUPAY_IR_URL set, a .ir invoice is raised on the Iranian bridge and
// settled there; the main bridge is never asked about it.
func TestIranInvoiceIsRaisedAndSettledOnTheIranianBridge(t *testing.T) {
	main := newRateBridge(t, 289698, "INV-MAIN")
	ir := newRateBridge(t, 290000, "INV-IR")
	s, store := payServer(t, main.URL)
	s.WithIranPayments(nabupay.New(ir.URL, "gate", "a-secret"))

	s.rechargeMe(httptest.NewRecorder(), iranRecharge(`{"amount":5}`))
	if len(main.bodies) != 0 {
		t.Fatalf("the main bridge got the .ir checkout: %v", main.bodies)
	}
	if got := ir.lastBody(t)["amount_toman"]; got != float64(1450000) {
		t.Errorf("amount_toman = %v, want the Iranian bridge's own rate: 1450000", got)
	}
	u := store.GetUser("buyer@example.ir")
	if len(u.Payments) != 1 || u.Payments[0].Bridge != bridgeIran || u.Payments[0].ID != "INV-IR" {
		t.Fatalf("payment rows = %+v, want one INV-IR on bridge ir", u.Payments)
	}

	ir.paid = true
	// Settled from any host: the row says which bridge to ask.
	w := httptest.NewRecorder()
	s.settleMyPayments(w, asUser(httptest.NewRequest(http.MethodPost, "/api/me/payments/settle", nil), "buyer@example.ir"))
	if main.verifies != 0 {
		t.Errorf("the main bridge was asked about an Iranian invoice")
	}
	if ir.verifies != 1 {
		t.Errorf("Iranian bridge verifies = %d, want 1", ir.verifies)
	}
	if got := store.GetUser("buyer@example.ir").Balance; got != 5 {
		t.Errorf("balance = %v, want 5", got)
	}
}

// A .com payer keeps using the main bridge even when the Iranian one is set.
func TestComInvoiceStaysOnTheMainBridge(t *testing.T) {
	main := newRateBridge(t, 289698, "INV-MAIN")
	ir := newRateBridge(t, 290000, "INV-IR")
	s, store := payServer(t, main.URL)
	s.WithIranPayments(nabupay.New(ir.URL, "gate", "a-secret"))

	s.rechargeMe(httptest.NewRecorder(), asUser(httptest.NewRequest(http.MethodPost, "/api/me/recharge", strings.NewReader(`{"amount":5}`)), "buyer@example.com"))
	if len(ir.bodies) != 0 {
		t.Fatalf("a .com checkout went to the Iranian bridge")
	}
	if p := store.GetUser("buyer@example.com").Payments[0]; p.Bridge != "" {
		t.Errorf("bridge = %q, want main", p.Bridge)
	}
}

func TestStatusQuotesTomanOnlyOnIranHost(t *testing.T) {
	main := newRateBridge(t, 289698, "INV-MAIN")
	s, store := payServer(t, main.URL)
	token, _, err := store.NewSession("buyer@example.ir", false)
	if err != nil {
		t.Fatal(err)
	}
	get := func(host string) map[string]any {
		r := httptest.NewRequest(http.MethodGet, "/api/status", nil)
		r.Header.Set(mirrorHostHeader, host)
		r.AddCookie(&http.Cookie{Name: consoleCookie, Value: token})
		w := httptest.NewRecorder()
		s.consoleStatus(w, r)
		var out map[string]any
		json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}
	ir := get("gate.nabuxai.ir")
	if ir["usd_toman"] != float64(289698) || ir["payment_locale"] != "fa" || ir["payment_currency"] != "IRT" {
		t.Errorf(".ir status = %v, want usd_toman 289698, locale fa, currency IRT", ir)
	}
	com := get("gate.nabuxai.com")
	if _, ok := com["usd_toman"]; ok {
		t.Errorf(".com status carried a toman rate: %v", com)
	}
}

func TestTomanFor(t *testing.T) {
	for _, c := range []struct {
		usd  float64
		rate int64
		want int64
	}{{5, 289698, 1448490}, {1.5, 289698, 434547}, {0, 289698, 0}, {5, 0, 0}} {
		if got := tomanFor(c.usd, c.rate); got != c.want {
			t.Errorf("tomanFor(%v, %v) = %v, want %v", c.usd, c.rate, got, c.want)
		}
	}
}
