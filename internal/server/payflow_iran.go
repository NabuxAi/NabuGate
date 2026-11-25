package server

import (
	"context"
	"math"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"nabugate/internal/nabupay"
)

// Paying from Iran.
//
// A payer who opened a .ir host (gate.nabuxai.ir) is shown the whole payment
// in Persian and in toman:
//
//   - the panel gets the bridge's own toman rate in /api/status (usd_toman),
//     so every dollar figure on the buy screens carries its toman equivalent;
//   - the invoice is raised with amount_toman beside amount_usd and with
//     customer.country=IR / customer.locale=fa, so the bridge records it in
//     toman and its hosted checkout opens in Persian, quoting toman;
//   - when NABUPAY_IR_URL is set (the Iranian NabuPay, pay.nabuxai.ir) the
//     invoice is raised there instead of on the main bridge, so the checkout
//     page and the Iranian bank methods are that instance's.
//
// NabuGate holds no exchange rate of its own. The rate is whatever the bridge
// that will charge the payer answers on /api/v1/pay/rate; with no answer, no
// toman figure is shown or sent and the bridge converts as before.

// bridgeIran is the Payment.Bridge value of invoices raised on NABUPAY_IR_URL.
const bridgeIran = "ir"

const (
	rateCacheTTL     = 5 * time.Minute
	rateFailCacheTTL = time.Minute
)

type iranPayments struct {
	bridge   *nabupay.Client
	gateways gatewayCache

	mu    sync.Mutex
	rates map[*nabupay.Client]rateEntry
}

type rateEntry struct {
	at   time.Time
	rate int64
	ok   bool
}

// WithIranPayments attaches the Iranian NabuPay bridge. nil (NABUPAY_IR_URL
// unset) keeps every payer on the main bridge; .ir payers still see Persian
// and toman, priced by the main bridge's rate.
func (s *Server) WithIranPayments(c *nabupay.Client) *Server {
	s.iranPay.bridge = c
	return s
}

// isIranRequest reports whether the browser reached us on a .ir host.
func isIranRequest(r *http.Request) bool {
	u, err := url.Parse(publicBase(r))
	if err != nil {
		return false
	}
	return strings.HasSuffix(strings.ToLower(u.Hostname()), ".ir")
}

// bridgeFor picks the bridge a new invoice from this request is raised on,
// and the name recorded against it.
func (s *Server) bridgeFor(r *http.Request) (*nabupay.Client, string) {
	if isIranRequest(r) && s.iranPay.bridge.Configured() {
		return s.iranPay.bridge, bridgeIran
	}
	return s.pay, ""
}

// bridgeNamed is the bridge that issued an invoice, for settling it. nil when
// that bridge is no longer configured: its invoices cannot be asked about
// anywhere else, because each NabuPay instance keeps its own.
func (s *Server) bridgeNamed(name string) *nabupay.Client {
	if name == bridgeIran {
		if s.iranPay.bridge.Configured() {
			return s.iranPay.bridge
		}
		return nil
	}
	return s.pay
}

// tomanRate is c's toman-per-dollar rate, cached for a few minutes: /api/status
// asks on every page load and the rate does not move that fast. A failure is
// cached briefly too, so a bridge that is down is not asked on every load.
func (s *Server) tomanRate(ctx context.Context, c *nabupay.Client) (int64, bool) {
	if !c.Configured() {
		return 0, false
	}
	p := &s.iranPay
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, hit := p.rates[c]; hit {
		ttl := rateCacheTTL
		if !e.ok {
			ttl = rateFailCacheTTL
		}
		if time.Since(e.at) < ttl {
			return e.rate, e.ok
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	rate, err := c.Rate(ctx)
	if p.rates == nil {
		p.rates = make(map[*nabupay.Client]rateEntry)
	}
	if err != nil {
		if s.log != nil {
			s.log.Warn("payment bridge gave no toman rate", "error", err)
		}
		p.rates[c] = rateEntry{at: time.Now()}
		return 0, false
	}
	p.rates[c] = rateEntry{at: time.Now(), rate: rate, ok: true}
	return rate, true
}

// tomanFor is usd at rate, to the whole toman (toman has nothing smaller).
func tomanFor(usd float64, rate int64) int64 {
	if usd <= 0 || rate <= 0 {
		return 0
	}
	return int64(math.Round(usd * float64(rate)))
}

// iranCheckout fills the Persian/toman parts of a checkout started from a
// .ir host. Nothing changes for any other host.
func (s *Server) iranCheckout(r *http.Request, c *nabupay.Client, email string, opts *nabupay.StartOptions) {
	if !isIranRequest(r) {
		return
	}
	opts.Country = "IR"
	opts.Locale = "fa"
	opts.Email = email
	if rate, ok := s.tomanRate(r.Context(), c); ok {
		opts.AmountToman = tomanFor(opts.AmountUSD, rate)
	}
	// No method named: go straight to the AccountCity bank relay when the
	// bridge offers it to Iran. The payer is redirected to the bank with no
	// chooser page in between; NabuPay's hosted page stays the fallback for
	// a bridge that does not list it.
	if opts.Gateway == "" {
		if gw := iranDefaultGateway(); gw != "" && s.bridgeOffers(r.Context(), c, gw) {
			opts.Gateway = gw
		}
	}
}

// iranDefaultGateway is the method a .ir payer is sent to when none is named:
// NABUPAY_IR_GATEWAY, "bank_payment" (AccountCity's bank relay) by default.
// "auto" or "none" keeps NabuPay's own chooser page.
func iranDefaultGateway() string {
	v := strings.TrimSpace(os.Getenv("NABUPAY_IR_GATEWAY"))
	switch strings.ToLower(v) {
	case "":
		return "bank_payment"
	case "auto", "none":
		return ""
	}
	return v
}

// bridgeOffers reports whether c lists slug for Iranian payers right now.
func (s *Server) bridgeOffers(ctx context.Context, c *nabupay.Client, slug string) bool {
	cache := &s.gateways
	if c == s.iranPay.bridge && c != nil {
		cache = &s.iranPay.gateways
	}
	ctx = context.WithValue(ctx, paymentCountryCtxKey{}, "IR")
	list, _ := s.paymentGatewaysFrom(ctx, c, cache)
	for _, g := range list {
		if strings.EqualFold(g.Slug, slug) {
			return true
		}
	}
	return false
}
