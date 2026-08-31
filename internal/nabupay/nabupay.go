// Package nabupay starts and confirms real payments through NabuPay, the
// payment bridge NabuDesk exposes.
//
// The gateways themselves — Zarinpal, Aqayepardakht, Larapay, Stripe, PayPal,
// Polar, NowPayments — live in NabuDesk and are configured there. Reimplementing
// any of them here would mean a second place for merchant credentials to live
// and a second implementation to keep correct, for a gateway the organisation
// already integrates once.
//
// What stays here is the money: NabuGate owns the wallet, so NabuGate decides
// when a balance moves. The bridge is asked whether the gateway confirmed the
// payment, and the answer is all this package returns.
package nabupay

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client talks to the NabuPay bridge.
type Client struct {
	BaseURL string // e.g. https://desk.nabuxai.com
	AppID   string // identifies this caller to the bridge
	Secret  string // shared secret the bridge verifies the signature with
	HTTP    *http.Client
}

// New returns a client, or nil when the deployment has not configured one.
//
// nil rather than an error, and a nil client reports NotConfigured: a gateway
// running without payment configured is an ordinary deployment, and the panel
// says recharging is unavailable rather than failing to start.
func New(baseURL, appID, secret string) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" || strings.TrimSpace(secret) == "" {
		return nil
	}
	if appID = strings.TrimSpace(appID); appID == "" {
		appID = "gate"
	}
	return &Client{
		BaseURL: baseURL,
		AppID:   appID,
		Secret:  secret,
		HTTP:    &http.Client{Timeout: 20 * time.Second},
	}
}

// Configured reports whether payments can be started at all.
func (c *Client) Configured() bool { return c != nil }

// ErrNotConfigured is returned by every call on a nil client.
var ErrNotConfigured = errors.New("no payment gateway is configured on this deployment")

// Checkout is a started payment: an invoice to remember and a URL to send the
// payer to.
type Checkout struct {
	Invoice string
	URL     string
}

// Start raises an invoice on the bridge and returns where to send the payer.
//
// amountUSD is what the wallet will be credited if the payment completes, so it
// is also what the payer is charged; the bridge converts to toman using the
// rate NabuDesk is configured with.
func (c *Client) Start(ctx context.Context, opts StartOptions) (Checkout, error) {
	if c == nil {
		return Checkout{}, ErrNotConfigured
	}
	if opts.AmountUSD <= 0 {
		return Checkout{}, errors.New("a payment must be for more than nothing")
	}

	body, err := json.Marshal(map[string]any{
		"service_name": "gate",
		"gateway":      opts.Gateway,
		"amount_usd":   opts.AmountUSD,
		"description":  opts.Description,
		"callback_url": opts.CallbackURL,
	})
	if err != nil {
		return Checkout{}, err
	}

	var out struct {
		Success     bool   `json:"success"`
		Invoice     string `json:"invoice_number"`
		CheckoutURL string `json:"checkout_url"`
		Message     string `json:"message"`
	}
	if err := c.post(ctx, "/api/v1/pay/checkout", body, &out); err != nil {
		return Checkout{}, err
	}

	// A bridge that could not start a payment has no URL to send anyone to.
	// Treating a missing URL as success is how a payer ends up back here with
	// "status=success" glued on and nothing paid.
	if !out.Success || out.CheckoutURL == "" || out.Invoice == "" {
		msg := out.Message
		if msg == "" {
			msg = "the payment gateway could not start this payment"
		}
		return Checkout{}, errors.New(msg)
	}
	return Checkout{Invoice: out.Invoice, URL: out.CheckoutURL}, nil
}

// StartOptions describes the payment to raise.
type StartOptions struct {
	AmountUSD   float64
	Gateway     string // bridge gateway slug, e.g. "zarinpal"
	Description string
	CallbackURL string // where the gateway returns the payer
}

// Confirm asks the bridge whether the money for an invoice actually moved.
//
// The answer is the bridge's, which is in turn the gateway's — nothing the
// payer's browser carries is evidence, so nothing from the return URL other
// than the invoice number is passed on or believed.
func (c *Client) Confirm(ctx context.Context, invoice string) (paid bool, err error) {
	if c == nil {
		return false, ErrNotConfigured
	}
	if strings.TrimSpace(invoice) == "" {
		return false, errors.New("no invoice number to confirm")
	}

	var out struct {
		Success bool   `json:"success"`
		Status  string `json:"status"`
	}
	path := "/api/v1/pay/verify/" + url.PathEscape(invoice) + "?status=success"
	if err := c.get(ctx, path, &out); err != nil {
		return false, err
	}
	// "paid" is the bridge's word for settled. Anything else — pending, failed,
	// or a status this version does not know — is not a payment.
	return out.Success && strings.EqualFold(out.Status, "paid"), nil
}

func (c *Client) post(ctx context.Context, path string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.sign(req, body)
	return c.do(req, out)
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	// A GET has no body, and the bridge signs over the request body, so the
	// signed payload is empty here. That is what the bridge computes too.
	c.sign(req, nil)
	return c.do(req, out)
}

// sign applies the bridge's HMAC scheme: sha256 over "<app>:<timestamp>:<body>"
// keyed with the shared secret, with the app id and timestamp sent alongside so
// the far side can recompute it.
func (c *Client) sign(req *http.Request, body []byte) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(c.Secret))
	fmt.Fprintf(mac, "%s:%s:%s", c.AppID, ts, body)

	req.Header.Set("X-NabuGate-App-Id", c.AppID)
	req.Header.Set("X-NabuGate-Timestamp", ts)
	req.Header.Set("X-NabuGate-Signature", hex.EncodeToString(mac.Sum(nil)))
}

func (c *Client) do(req *http.Request, out any) error {
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	// The bridge answers 422 with a usable sentence when a gateway refuses, so
	// the body is read before the status is judged.
	body, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if readErr != nil {
		return fmt.Errorf("payment bridge response could not be read: %w", readErr)
	}

	// Anything but 2xx is an ERROR, never a verdict.
	//
	// This used to decode 4xx into the zero value and return nil, so a rotated
	// secret, a refused signature or a moved route came back from Confirm as
	// (false, nil) — indistinguishable from "the gateway says this is unpaid".
	// Every payer was told their payment had not been confirmed and nothing was
	// logged, because the caller only logs on a non-nil error.
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var problem struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		_ = json.Unmarshal(body, &problem)
		switch {
		case problem.Message != "":
			return fmt.Errorf("payment bridge refused the request (%d): %s", res.StatusCode, problem.Message)
		case problem.Error != "":
			return fmt.Errorf("payment bridge refused the request (%d): %s", res.StatusCode, problem.Error)
		default:
			return fmt.Errorf("payment bridge refused the request (%d)", res.StatusCode)
		}
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("payment bridge returned an unreadable response: %w", err)
	}
	return nil
}
