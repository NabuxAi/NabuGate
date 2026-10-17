package server

import (
	"sync"

	"nabugate/internal/router"
)

// inflightReserveUSD is held against an owner's balance for each request of
// theirs still running. Debits land when a call finishes, so the balance gate
// alone let a small balance fan out into any number of concurrent calls, each
// admitted against money the others were about to spend.
//
// A request is admitted while the balance exceeds what is already held, so a
// last cent still buys one call at a time, and a $5 balance a hundred at
// once. What it cannot bound is one call costing more than the balance; that
// overdraft is one request deep.
const inflightReserveUSD = 0.05

// reservations is the money held per owner for their requests in flight.
type reservations struct {
	mu   sync.Mutex
	held map[string]float64
}

// hold admits one request for owner against balance, or refuses when what is
// already held would not leave anything for it.
func (r *reservations) hold(owner string, balance float64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.held == nil {
		r.held = map[string]float64{}
	}
	if balance <= r.held[owner] {
		return false
	}
	r.held[owner] += inflightReserveUSD
	return true
}

func (r *reservations) release(owner string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.held[owner] -= inflightReserveUSD; r.held[owner] < inflightReserveUSD/2 {
		delete(r.held, owner)
	}
}

// spendsOwnKeysOnly reports a request that can only be served on the
// caller's own vendor keys, which costs their balance nothing.
func spendsOwnKeysOnly(keys router.CallerKeys) bool {
	keys = keys.Normalize()
	return keys.Mode == router.ModeOwn && len(keys.Keys) > 0
}
