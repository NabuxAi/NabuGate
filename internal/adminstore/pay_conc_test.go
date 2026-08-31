package adminstore

import (
	"path/filepath"
	"sync"
	"testing"
)

// The settle endpoint is reachable from a page the payer can refresh, and a
// gateway may replay its callback. Two settles arriving at once must still
// credit once — a check-then-act that is not atomic would credit twice, and
// that is real money.
func TestConcurrentSettlementCreditsExactlyOnce(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "admin.json"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := st.StartPayment("buyer@example.com", 25, "INV-RACE"); err != nil {
		t.Fatalf("start: %v", err)
	}

	const racers = 24
	var wg sync.WaitGroup
	var mu sync.Mutex
	credited := 0

	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // release them all together
			if _, err := st.SettlePayment("buyer@example.com", "INV-RACE"); err == nil {
				mu.Lock()
				credited++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	if credited != 1 {
		t.Errorf("%d of %d concurrent settlements reported success, want exactly 1", credited, racers)
	}
	if u := st.GetUser("buyer@example.com"); u.Balance != 25 {
		t.Errorf("balance = %v after %d concurrent settlements, want 25", u.Balance, racers)
	}
}

// Two different invoices for the same account settled at once must both land.
func TestConcurrentSettlementOfDistinctInvoicesBothCredit(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "admin.json"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	for _, inv := range []string{"INV-A", "INV-B"} {
		if err := st.StartPayment("buyer@example.com", 10, inv); err != nil {
			t.Fatalf("start %s: %v", inv, err)
		}
	}

	var wg sync.WaitGroup
	for _, inv := range []string{"INV-A", "INV-B"} {
		wg.Add(1)
		go func(inv string) {
			defer wg.Done()
			st.SettlePayment("buyer@example.com", inv)
		}(inv)
	}
	wg.Wait()

	if u := st.GetUser("buyer@example.com"); u.Balance != 20 {
		t.Errorf("balance = %v, want 20 (both invoices credited)", u.Balance)
	}
}
