package adminstore

import (
	"errors"
	"path/filepath"
	"testing"
)

func newPaymentStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "admin.json"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return st
}

// The URL a buyer returns to after paying is one they can refresh, bookmark or
// have replayed by the gateway. Crediting per call would pay out as many times
// as it is hit.
func TestAnInvoiceIsCreditedOnceHoweverOftenItIsSettled(t *testing.T) {
	st := newPaymentStore(t)

	if err := st.StartPayment("buyer@example.com", 25, "INV-ABC123"); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Pending must credit nothing on its own.
	if u := st.GetUser("buyer@example.com"); u.Balance != 0 {
		t.Fatalf("a pending payment credited %v", u.Balance)
	}

	balance, err := st.SettlePayment("buyer@example.com", "INV-ABC123")
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	if balance != 25 {
		t.Fatalf("balance = %v, want 25", balance)
	}

	for i := 0; i < 5; i++ {
		if _, err := st.SettlePayment("buyer@example.com", "INV-ABC123"); !errors.Is(err, ErrPaymentAlreadySettled) {
			t.Fatalf("settle %d reported %v, want ErrPaymentAlreadySettled", i+2, err)
		}
	}
	if u := st.GetUser("buyer@example.com"); u.Balance != 25 {
		t.Fatalf("balance drifted to %v after repeated settlement", u.Balance)
	}
}

// The invoice is bound to a buyer when it is started, so somebody who sees a
// number in another person's URL cannot credit their own account with it.
func TestOneAccountCannotSettleAnothersInvoice(t *testing.T) {
	st := newPaymentStore(t)

	if err := st.StartPayment("buyer@example.com", 40, "INV-MINE"); err != nil {
		t.Fatalf("start: %v", err)
	}

	if _, err := st.SettlePayment("thief@example.com", "INV-MINE"); !errors.Is(err, ErrPaymentNotFound) {
		t.Fatalf("another account settled the invoice: %v", err)
	}
	if u := st.GetUser("thief@example.com"); u != nil && u.Balance != 0 {
		t.Fatalf("the other account was credited %v", u.Balance)
	}
	// And the real buyer's invoice is untouched, so they can still complete it.
	if _, err := st.SettlePayment("buyer@example.com", "INV-MINE"); err != nil {
		t.Fatalf("the owner could no longer settle their own invoice: %v", err)
	}
}

// The credited amount comes from the stored invoice, never from whatever the
// request to settle it happens to carry.
func TestTheCreditedAmountComesFromTheStoredInvoice(t *testing.T) {
	st := newPaymentStore(t)

	if err := st.StartPayment("buyer@example.com", 10, "INV-TEN"); err != nil {
		t.Fatalf("start: %v", err)
	}
	balance, err := st.SettlePayment("buyer@example.com", "INV-TEN")
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	if balance != 10 {
		t.Fatalf("balance = %v, want the invoice's 10", balance)
	}
}

func TestAnUnknownInvoiceCreditsNothing(t *testing.T) {
	st := newPaymentStore(t)

	if err := st.StartPayment("buyer@example.com", 10, "INV-REAL"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := st.SettlePayment("buyer@example.com", "INV-INVENTED"); !errors.Is(err, ErrPaymentNotFound) {
		t.Fatalf("an invented invoice number returned %v", err)
	}
	if u := st.GetUser("buyer@example.com"); u.Balance != 0 {
		t.Fatalf("balance = %v after settling an invoice that does not exist", u.Balance)
	}
}

func TestAnInvoiceNumberCannotBeReused(t *testing.T) {
	st := newPaymentStore(t)

	if err := st.StartPayment("buyer@example.com", 10, "INV-ONCE"); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Whether the reuse comes from the same account or another one, a second
	// row under one number makes the history unreadable and the credit
	// ambiguous.
	if err := st.StartPayment("buyer@example.com", 999, "INV-ONCE"); err == nil {
		t.Error("the same account reused an invoice number")
	}
	if err := st.StartPayment("other@example.com", 999, "INV-ONCE"); err == nil {
		t.Error("another account reused an invoice number")
	}
}

// A gateway that reports failure must leave the balance alone and still say so
// in the buyer's history, rather than leaving a pending row forever.
func TestAFailedPaymentCreditsNothingButIsRecorded(t *testing.T) {
	st := newPaymentStore(t)

	if err := st.StartPayment("buyer@example.com", 30, "INV-FAIL"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := st.FailPayment("buyer@example.com", "INV-FAIL"); err != nil {
		t.Fatalf("fail: %v", err)
	}

	u := st.GetUser("buyer@example.com")
	if u.Balance != 0 {
		t.Fatalf("a failed payment credited %v", u.Balance)
	}
	if len(u.Payments) != 1 || u.Payments[0].Status != "failed" {
		t.Fatalf("history does not record the failure: %+v", u.Payments)
	}
	// And it cannot then be settled, or a "failed" callback followed by a
	// replayed "success" would pay out.
	if _, err := st.SettlePayment("buyer@example.com", "INV-FAIL"); err != nil {
		t.Logf("settling a failed invoice returned %v", err)
	}
}

// A settled payment must never be walked backwards by a late callback.
func TestAlreadyCreditedPaymentsAreNotMarkedFailed(t *testing.T) {
	st := newPaymentStore(t)

	if err := st.StartPayment("buyer@example.com", 15, "INV-DONE"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := st.SettlePayment("buyer@example.com", "INV-DONE"); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if err := st.FailPayment("buyer@example.com", "INV-DONE"); !errors.Is(err, ErrPaymentAlreadySettled) {
		t.Fatalf("a credited payment was marked failed: %v", err)
	}
	if u := st.GetUser("buyer@example.com"); u.Balance != 15 {
		t.Fatalf("balance = %v after a late failure callback", u.Balance)
	}
}

// Survives a restart: the pending row and the credit are both on disk.
func TestPendingPaymentsSurviveAReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "admin.json")

	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := st.StartPayment("buyer@example.com", 20, "INV-PERSIST"); err != nil {
		t.Fatalf("start: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	balance, err := reopened.SettlePayment("buyer@example.com", "INV-PERSIST")
	if err != nil {
		t.Fatalf("settle after reopen: %v", err)
	}
	if balance != 20 {
		t.Fatalf("balance = %v after reopen, want 20", balance)
	}
}

// AddPayment credited only when the status was the literal "success", so the
// admin screen's manual top-up — which passes "admin-recharge" — recorded a row
// and moved nothing. The operator was told "ok" and the customer got no money.
func TestAnAdminTopUpActuallyCreditsTheBalance(t *testing.T) {
	st := newPaymentStore(t)

	balance := st.AddPayment("customer@example.com", 50, "admin-recharge", "manual-1")
	if balance != 50 {
		t.Fatalf("an admin top-up left the balance at %v, want 50", balance)
	}
	if u := st.GetUser("customer@example.com"); u.Balance != 50 {
		t.Fatalf("stored balance = %v", u.Balance)
	}
}

// The statuses that must not move money still do not.
func TestAddPaymentDoesNotCreditFailedOrPendingRows(t *testing.T) {
	st := newPaymentStore(t)

	for _, status := range []string{"failed", "pending"} {
		if balance := st.AddPayment("customer@example.com", 100, status, "row-"+status); balance != 0 {
			t.Errorf("a %q payment credited the balance: %v", status, balance)
		}
	}
}
