package router

import (
	"context"
	"sort"
	"strings"
	"time"
)

// --- Out of credit ---
//
// A provider whose prepaid balance ran out does not come back on the next
// request. Fallback already rescues each call, but only after paying for a
// round trip to a vendor that is certain to refuse: on 2026-10-23 the OpenAI
// key had no credit left and every request on an OpenAI-first alias waited for
// that refusal before reaching a rung that could answer.
//
// So when the gateway's own key is refused for money, that provider is moved to
// the back of every chain for a while. Moved, not removed: if everything else
// fails too, it is still tried, so this can only change which vendor answers,
// never whether one does. A success on it clears the mark at once, which is how
// a top-up is noticed without anyone telling the gateway.

// creditCooldown is how long a provider stays at the back after a refusal for
// money. Long enough that a busy alias stops paying the wasted round trip; short
// enough that a top-up takes effect without a restart.
const creditCooldown = 10 * time.Minute

// creditSignals are the phrases vendors use when the account, not the request,
// is the problem. Rate limits are deliberately absent: a 429 for requests per
// minute clears by itself in seconds and says nothing about the balance.
var creditSignals = []string{
	"status 402",
	"payment required",
	"insufficient_quota",
	"credit_balance_exhausted",
	"no credits remaining",
	"insufficient credit",
	"insufficient balance",
	"insufficient funds",
	"credit balance is too low",
	"exceeded your current quota",
	"billing_hard_limit",
}

// outOfCredit reports whether an upstream error is a refusal for money.
func outOfCredit(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, s := range creditSignals {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// failed records a rung that the upstream refused, and marks the provider out
// of credit when that is why. Only the gateway's key is marked: a caller's key
// is their wallet, and their balance says nothing about ours.
func (r *Router) failed(failures *targetErrors, t attempt, err error) {
	failures.add(t.label, t.Model, err)
	if t.caller || !outOfCredit(err) {
		return
	}
	r.creditMu.Lock()
	if r.drained == nil {
		r.drained = make(map[string]time.Time)
	}
	_, already := r.drained[t.Provider]
	r.drained[t.Provider] = r.clock()
	r.creditMu.Unlock()
	if !already {
		r.log.Warn("provider out of credit — moved to the back of every chain",
			"provider", t.Provider, "for", creditCooldown.String(), "error", err.Error())
	}
}

// served records the credential that answered, and clears an out-of-credit
// mark: a provider that just served on the gateway's key has money again.
func (r *Router) served(ctx context.Context, t attempt) {
	recordKeySource(ctx, t.caller)
	if t.caller {
		return
	}
	r.creditMu.Lock()
	_, was := r.drained[t.Provider]
	delete(r.drained, t.Provider)
	r.creditMu.Unlock()
	if was {
		r.log.Info("provider has credit again", "provider", t.Provider)
	}
}

// outOfCreditNow reports whether the gateway's key for this provider was
// refused for money within the cooldown.
func (r *Router) outOfCreditNow(providerName string) bool {
	r.creditMu.Lock()
	defer r.creditMu.Unlock()
	at, ok := r.drained[providerName]
	if !ok {
		return false
	}
	if r.clock().Sub(at) >= creditCooldown {
		delete(r.drained, providerName)
		return false
	}
	return true
}

// OutOfCredit lists the providers currently at the back of every chain.
func (r *Router) OutOfCredit() []string {
	r.creditMu.Lock()
	names := make([]string, 0, len(r.drained))
	for name := range r.drained {
		names = append(names, name)
	}
	r.creditMu.Unlock()

	out := names[:0]
	for _, name := range names {
		if r.outOfCreditNow(name) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// clock is r.now, or the wall clock for a Router built without New.
func (r *Router) clock() time.Time {
	if r.now != nil {
		return r.now()
	}
	return time.Now()
}

// creditLast moves the gateway rungs of out-of-credit providers behind every
// other rung, keeping the configured order within each group.
func (r *Router) creditLast(rungs []attempt) []attempt {
	var funded, drained []attempt
	for _, a := range rungs {
		if !a.caller && a.adapter != nil && r.outOfCreditNow(a.Provider) {
			drained = append(drained, a)
			continue
		}
		funded = append(funded, a)
	}
	if len(drained) == 0 {
		return rungs
	}
	return append(funded, drained...)
}
