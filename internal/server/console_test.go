package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"nabugate/internal/nabupay"
)

// The console's origin filter is what makes a key safe to embed in a web app:
// the key alone cannot be kept secret there, so the gateway also checks where
// the request came from.

func requestFrom(origin, referer string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if referer != "" {
		r.Header.Set("Referer", referer)
	}
	return r
}

func TestOriginEmptyListAllowsAnything(t *testing.T) {
	if !originAllowed(nil, requestFrom("https://anywhere.example", "")) {
		t.Error("a token with no origin list must not be restricted")
	}
	if !originAllowed(nil, requestFrom("", "")) {
		t.Error("a token with no origin list must still work for non-browser clients")
	}
}

func TestOriginExactMatch(t *testing.T) {
	allowed := []string{"app.nabuxai.com"}

	if !originAllowed(allowed, requestFrom("https://app.nabuxai.com", "")) {
		t.Error("the listed origin was refused")
	}
	if originAllowed(allowed, requestFrom("https://evil.example", "")) {
		t.Error("an unlisted origin was allowed")
	}
}

func TestOriginWildcardDoesNotMatchLookalikes(t *testing.T) {
	allowed := []string{"*.nabuxai.com"}

	for _, host := range []string{"https://app.nabuxai.com", "https://a.b.nabuxai.com", "https://nabuxai.com"} {
		if !originAllowed(allowed, requestFrom(host, "")) {
			t.Errorf("%s should match *.nabuxai.com", host)
		}
	}
	// A substring match would let this through, which is the whole reason the
	// wildcard is anchored to a dot boundary.
	if originAllowed(allowed, requestFrom("https://evil-nabuxai.com", "")) {
		t.Error("a lookalike domain matched the wildcard")
	}
	if originAllowed(allowed, requestFrom("https://nabuxai.com.evil.example", "")) {
		t.Error("a suffix-appended domain matched the wildcard")
	}
}

func TestOriginFallsBackToReferer(t *testing.T) {
	allowed := []string{"app.nabuxai.com"}
	// Some browsers omit Origin on same-site GETs but still send Referer.
	if !originAllowed(allowed, requestFrom("", "https://app.nabuxai.com/editor")) {
		t.Error("Referer should be used when Origin is absent")
	}
}

func TestOriginRestrictedTokenRefusesNonBrowser(t *testing.T) {
	allowed := []string{"app.nabuxai.com"}
	// No Origin and no Referer means no browser. A token that named specific
	// origins was meant for one, so this must refuse rather than assume.
	if originAllowed(allowed, requestFrom("", "")) {
		t.Error("a request with no origin was allowed against a restricted token")
	}
	if originAllowed(allowed, requestFrom("null", "")) {
		t.Error(`an Origin of "null" was treated as a real origin`)
	}
}

// TestRechargeDoesNotPreselectAGateway pins the behaviour change.
//
// The panel used to fall back to s.payGateway, so every top-up went straight to
// one gateway. When Aqayepardakht answered -15 the payer met that error instead
// of a choice, and payment was down for everyone even though other gateways were
// working. Sending no gateway makes the bridge return NabuPay's own checkout.
func TestRechargeDoesNotPreselectAGateway(t *testing.T) {
	var sent map[string]any

	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&sent)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"invoice_number":"inv-1","checkout_url":"https://desk.test/pay/inv-1"}`))
	}))
	defer bridge.Close()

	client := nabupay.New(bridge.URL, "gate", "shared-secret")
	if client == nil {
		t.Fatal("client should be configured")
	}

	_, err := client.Start(context.Background(), nabupay.StartOptions{
		AmountUSD:   5,
		Description: "top-up",
		CallbackURL: "https://gate.test/panel/account",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if got, ok := sent["gateway"]; ok && got != "" {
		t.Fatalf("a gateway was named (%v); the bridge must be left to offer the choice", got)
	}
}
