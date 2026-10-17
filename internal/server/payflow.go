package server

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"time"

	"nabugate/internal/adminstore"
	"nabugate/internal/config"
	"nabugate/internal/nabupay"
)

// Top-up bounds, enforced here and not only in the panel: a request can be
// sent without the panel, and the bridge turns whatever arrives into a toman
// invoice. A cent-fraction converts unpredictably; a billion is a typo.
const (
	minTopUpUSD = 1.0
	maxTopUpUSD = 5000.0
)

// roundCents is the precision money is invoiced at.
func roundCents(v float64) float64 { return math.Round(v*100) / 100 }

// planByID finds a plan this deployment sells.
func (s *Server) planByID(id string) (config.Plan, bool) {
	id = strings.TrimSpace(id)
	for _, p := range s.plans {
		if strings.EqualFold(strings.TrimSpace(p.ID), id) {
			return p, true
		}
	}
	return config.Plan{}, false
}

// errPlanSwitch is returned by buyPlan when the buyer holds a different plan
// with days left and has not said they accept losing them.
var errPlanSwitch = errors.New("buying a different plan ends the current one now")

// buyPlan charges one term of plan to email's balance. It is the one place a
// plan is bought, whether from the buy button or from a top-up raised to pay
// for it, so both paths charge, credit and record the same way.
func (s *Server) buyPlan(email string, plan config.Plan, confirmSwitch bool, key string) (adminstore.Subscription, error) {
	if !confirmSwitch {
		if cur, ok := s.admin.ActiveSubscription(email); ok && !strings.EqualFold(cur.PlanID, plan.ID) {
			return cur, errPlanSwitch
		}
	}
	return s.admin.Subscribe(email, adminstore.Subscription{
		PlanID:            plan.ID,
		Name:              plan.Name,
		PriceUSD:          plan.PriceUSD,
		Markup:            plan.Rate(),
		MinChargeUSD:      plan.MinChargeUSD,
		Providers:         plan.Providers,
		IncludesCreditUSD: plan.IncludesCreditUSD,
		PurchaseKey:       strings.TrimSpace(key),
	}, plan.Term())
}

// daysLeft is how much of a term remains, rounded up, for the switch warning.
func daysLeft(sub adminstore.Subscription, now time.Time) int {
	d := sub.ExpiresAt.Sub(now).Hours() / 24
	if d <= 0 {
		return 0
	}
	return int(math.Ceil(d))
}

// gatewayCache holds the bridge's method list for the status endpoint, which
// every page load asks: the list changes when someone enables a merchant in
// NabuDesk, not per request.
type gatewayCache struct {
	mu      sync.Mutex
	at      time.Time
	list    []nabupay.Gateway
	healthy bool
}

const gatewayCacheTTL = 5 * time.Minute

// paymentGateways returns the bridge's current methods and whether the bridge
// answered at all. A bridge that does not answer, or answers with no methods,
// is a deployment where pressing Pay cannot work, and the panel should say so
// before the payer tries.
func (s *Server) paymentGateways(ctx context.Context) ([]nabupay.Gateway, bool) {
	if !s.pay.Configured() {
		return nil, false
	}
	c := &s.gateways
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.at.IsZero() && time.Since(c.at) < gatewayCacheTTL {
		return c.list, c.healthy
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	list, err := s.pay.Gateways(ctx)
	c.at = time.Now()
	if err != nil {
		s.log.Warn("payment bridge did not list its gateways", "error", err)
		c.list, c.healthy = nil, false
		return nil, false
	}
	c.list, c.healthy = list, len(list) > 0
	return c.list, c.healthy
}
