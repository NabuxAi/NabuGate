package nabupay

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestADeploymentWithNoGatewayConfiguredIsNotAnError(t *testing.T) {
	for _, c := range []*Client{
		New("", "gate", "secret"),
		New("https://desk.example", "gate", ""),
		New("   ", "gate", "secret"),
	} {
		if c.Configured() {
			t.Error("a client was returned for an unconfigured deployment")
		}
		if _, err := c.Start(context.Background(), StartOptions{AmountUSD: 10}); err != ErrNotConfigured {
			t.Errorf("Start on a nil client returned %v", err)
		}
		if _, err := c.Confirm(context.Background(), "INV-1"); err != ErrNotConfigured {
			t.Errorf("Confirm on a nil client returned %v", err)
		}
	}
}

// The bridge recomputes sha256("<app>:<timestamp>:<body>") keyed with the
// shared secret. A signature this side computes differently is refused there,
// and the failure looks like "the gateway is down" rather than a mismatch.
func TestRequestsAreSignedTheWayTheBridgeVerifiesThem(t *testing.T) {
	const secret = "a-shared-secret"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		appID := r.Header.Get("X-NabuGate-App-Id")
		ts := r.Header.Get("X-NabuGate-Timestamp")

		mac := hmac.New(sha256.New, []byte(secret))
		fmt.Fprintf(mac, "%s:%s:%s", appID, ts, body)
		want := hex.EncodeToString(mac.Sum(nil))

		if got := r.Header.Get("X-NabuGate-Signature"); got != want {
			t.Errorf("signature mismatch\n got %s\nwant %s", got, want)
		}
		if appID != "gate" {
			t.Errorf("app id = %q", appID)
		}
		// A timestamp outside five minutes is refused by the bridge as a replay.
		n, err := strconv.ParseInt(ts, 10, 64)
		if err != nil || time.Since(time.Unix(n, 0)) > time.Minute {
			t.Errorf("timestamp %q is not a usable unix time", ts)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"success": true, "invoice_number": "INV-1", "checkout_url": "https://pay.example/go",
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "gate", secret)
	if _, err := c.Start(context.Background(), StartOptions{AmountUSD: 10, Gateway: "zarinpal"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
}

// A bridge that cannot start a payment has no URL to send anyone to. Treating
// that as success is how a payer is bounced back with nothing paid.
func TestACheckoutWithNoUrlIsAFailure(t *testing.T) {
	for _, body := range []string{
		`{"success":false,"invoice_number":"INV-1","message":"gateway not configured"}`,
		`{"success":true,"invoice_number":"INV-1","checkout_url":""}`,
		`{"success":true,"checkout_url":"https://pay.example/go"}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			io.WriteString(w, body)
		}))

		c := New(srv.URL, "gate", "s")
		got, err := c.Start(context.Background(), StartOptions{AmountUSD: 10})
		if err == nil {
			t.Errorf("body %s was accepted as a checkout: %+v", body, got)
		}
		srv.Close()
	}
}

func TestOnlyAPaidInvoiceConfirms(t *testing.T) {
	cases := []struct {
		body string
		want bool
	}{
		{`{"success":true,"status":"paid"}`, true},
		{`{"success":true,"status":"PAID"}`, true},
		{`{"success":true,"status":"pending"}`, false},
		{`{"success":true,"status":"failed"}`, false},
		// Success without a status is not a receipt: an older or changed bridge
		// must not read as payment.
		{`{"success":true}`, false},
		{`{"success":false,"status":"paid"}`, false},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, tc.body)
		}))

		c := New(srv.URL, "gate", "s")
		got, err := c.Confirm(context.Background(), "INV-1")
		if err != nil {
			t.Errorf("%s: %v", tc.body, err)
		}
		if got != tc.want {
			t.Errorf("%s: confirmed = %v, want %v", tc.body, got, tc.want)
		}
		srv.Close()
	}
}

// An invoice number goes in the path, so one containing a slash or a query
// character must not be able to address a different endpoint.
func TestTheInvoiceNumberCannotRedirectTheRequest(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		io.WriteString(w, `{"success":true,"status":"pending"}`)
	}))
	defer srv.Close()

	c := New(srv.URL, "gate", "s")
	if _, err := c.Confirm(context.Background(), "../../admin/wipe"); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if want := "/api/v1/pay/verify/..%2F..%2Fadmin%2Fwipe"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
}

func TestABridgeThatIsDownIsReportedAsSuch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	c := New(srv.URL, "gate", "s")
	if _, err := c.Confirm(context.Background(), "INV-1"); err == nil {
		t.Error("a 502 from the bridge was reported as an answer")
	}
}

// A refused request is an ERROR, not a verdict of "unpaid".
//
// Confirm used to decode any 4xx into the zero value and return (false, nil),
// which is exactly what an unpaid invoice looks like. So a rotated secret or a
// moved route told every payer their payment had not been confirmed, while the
// caller — which only logs on a non-nil error — recorded nothing at all.
func TestARefusedConfirmIsAnErrorRatherThanAnUnpaidVerdict(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
		body string
		want string
	}{
		{"bad signature", 403, `{"success":false,"error":"Invalid NabuGate signature or expired request timestamp."}`, "Invalid NabuGate signature"},
		{"route moved", 404, `{"message":"Not Found"}`, "Not Found"},
		{"validation", 422, `{"message":"gateway not configured"}`, "gateway not configured"},
		{"no body", 401, ``, "401"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.code)
				io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			c := New(srv.URL, "gate", "s")
			paid, err := c.Confirm(context.Background(), "INV-1")
			if err == nil {
				t.Fatalf("a %d was reported as a verdict (paid=%v) instead of an error", tc.code, paid)
			}
			if paid {
				t.Error("a refused request reported the invoice as paid")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q, so an operator cannot tell what went wrong", err, tc.want)
			}
		})
	}
}

// The same applies to starting a payment: a refusal must not look like a
// checkout that merely lacked a URL.
func TestARefusedStartSurfacesTheBridgesReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `{"error":"Invalid NabuGate signature or expired request timestamp."}`)
	}))
	defer srv.Close()

	c := New(srv.URL, "gate", "s")
	if _, err := c.Start(context.Background(), StartOptions{AmountUSD: 10}); err == nil {
		t.Fatal("a 403 was accepted")
	} else if !strings.Contains(err.Error(), "signature") {
		t.Errorf("error %q hides the bridge's reason", err)
	}
}
