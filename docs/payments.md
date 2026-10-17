# Payments: connecting a gateway and fixing what goes wrong

NabuGate never talks to a bank. Wallet top-ups go through **NabuPay**, the
payment bridge NabuDesk exposes; the gateways themselves (Zarinpal,
Aqayepardakht, Larapay, Stripe, PayPal, Polar, NowPayments) are configured
there. No merchant credential lives in this repo and no card detail passes
through this service: the payer is handed to the bank and comes back.

What stays here is the money. NabuGate owns the wallet, so NabuGate decides
when a balance moves, and it moves only when the bridge says the gateway
confirmed the payment.

## Configuration

| Variable | Meaning | Default |
|---|---|---|
| `NABUPAY_URL` | Bridge base URL, e.g. `https://desk.nabuxai.com` | unset = payments off |
| `NABUPAY_SECRET` | Shared secret; requests are signed `sha256("<app>:<ts>:<body>")` | unset = payments off |
| `NABUPAY_APP_ID` | Identifies this caller to the bridge | `gate` |
| `NABUPAY_GATEWAY` | Default gateway slug the bridge should use | `zarinpal` |
| `NABU_PUBLIC_URL` | Where the gateway returns the payer | derived from the request |

With `NABUPAY_URL` or `NABUPAY_SECRET` unset the gateway still starts. The
startup log says `payments disabled`, `/api/status` answers
`"payments_enabled": false`, the panel greys out every pay button and says so,
and `POST /api/me/recharge` answers `501`.

Set `NABU_PUBLIC_URL` explicitly when the service sits behind a proxy or
Traefik. Without it the return URL is built from the request's `Host` header,
which behind some proxies is the internal hostname, and the payer lands on an
address they cannot reach.

## The flow

1. `POST /api/me/recharge {"amount": 25}` raises an invoice on the bridge
   (`POST {NABUPAY_URL}/api/v1/pay/checkout`), records it **before the payer
   leaves** as `pending` against the signed-in account, and returns
   `{invoice, checkout_url}`.
2. The panel sends the browser to `checkout_url`. The bank charges in toman at
   the rate NabuDesk is configured with; the wallet is credited the USD amount
   requested, whatever that day's rate was.
3. The bank returns the payer to `{NABU_PUBLIC_URL}/panel/account`.
4. The panel calls `POST /api/me/payments/settle`. The server first marks
   this account's invoices older than seven days `expired`, then lists its
   own pending invoices (at most 20, oldest first), asks the bridge
   `GET /api/v1/pay/verify/{invoice}` for each, and:
   - credits once per invoice whose status is `paid` — unless the bridge's
     `amount_usd` is more than a cent below what was invoiced, which marks the
     row `underpaid` and credits nothing;
   - marks `failed` an invoice the bridge calls `failed`, `expired`,
     `cancelled`, `rejected` or `refunded`, so it stops being asked about.

Top-ups are between $1 and $5,000, rounded to the cent, enforced by the
server. The bridge's current methods come from `GET /api/v1/pay/gateways` and
are shown to signed-in users through `/api/status` (`payment_gateways`,
`payments_healthy`); with no gateway named, the payer picks one on NabuPay's
own page, which only lists the ones that work.

## Buying a plan

`POST /api/plans/{id}/subscribe` charges one term to the balance and, in the
same write, adds a `plan` history row (negative amount) and a `plan-credit`
row for any credit the plan bundles. The body may carry:

- `idempotency_key` — a second request with the same key is the same click
  and charges nothing;
- `confirm_switch: true` — required to replace a different active plan,
  whose remaining days are lost; without it the answer is `409
  plan_switch_needs_confirm` with `days_left`.

Short of money, the answer is `402 insufficient_balance` with
`shortfall_usd`. `POST /api/me/recharge {"plan": "<id>"}` then raises an
invoice for exactly that shortfall (at least $1), and the settle that credits
it buys the plan in the same request.

Errors carry a stable `error.code` (`payments_disabled`,
`amount_out_of_range`, `gateway_refused`, `bridge_unreachable`,
`insufficient_balance`, `plan_switch_needs_confirm`, `no_such_plan`,
`plan_unavailable`); the console translates by code.

Nothing carried by the returning browser is read: not the query string the
gateway appended, not an invoice number. A return URL is something anyone can
type, so the only invoices ever settled are the ones this account started.
Settling is idempotent; refreshing the return page credits once.

The panel settles on **every** load of the account and payments screens, not
only on return from the bank. A gateway that confirms late (NowPayments waits
for chain confirmations) or one dropped request to the bridge therefore cannot
strand a payment: the next visit asks again.

## Payment statuses

| Status | Meaning |
|---|---|
| `pending` | Invoice raised, money not yet confirmed. Re-asked on every visit. |
| `success` | Bridge said `paid`; wallet credited once. |
| `failed` | Bridge said the gateway refused or cancelled it. Nothing credited. |
| `expired` | Unconfirmed for seven days; no longer asked about. Nothing credited. |
| `underpaid` | Bridge said `paid` but collected less than invoiced. Nothing credited — a support case. |

An admin top-up from the console is recorded as `success`, kind `admin`, with
id `manual-<timestamp>`, and never touches the bridge. It refuses an address
with no account and any amount that is not positive.

## Reading the log

| Line | Cause | Fix |
|---|---|---|
| `payments disabled (NABUPAY_URL or NABUPAY_SECRET not set)` | env missing | set both, redeploy |
| `payment bridge refused the request (401)` | secret or app id mismatch, or server clock skew (the signature carries a timestamp) | compare `NABUPAY_SECRET` / `NABUPAY_APP_ID` with the bridge; check `date` on the host |
| `payment bridge refused the request (422): …` | the bank gateway itself refused the invoice; the bridge's sentence follows | usually transient; otherwise the gateway's own merchant config in NabuDesk |
| `payment bridge returned an unreadable response` | the URL points at something that is not the bridge (an HTML error page, a wrong path) | check `NABUPAY_URL`, no trailing path |
| `could not confirm a payment` | bridge did not answer the verify call | invoice stays `pending`; nothing lost; next page load retries |
| `confirmed payment could not be credited` | store write failed after the bridge said `paid` | disk / permissions on the admin store; the invoice stays pending and retries |
| `wallet credited` | done | — |
| `gateway collected less than the invoice; not credited` | the bridge reported a smaller `amount_usd` | check the payment in NabuDesk; credit by hand what arrived |
| `plan paid for but not activated; credit kept` | the top-up landed but buying the plan failed | the money is in the wallet; the customer can press Activate |
| `another instance holds the console state` | a second container on the same `/data` | stop the stale container |

## Verifying by hand

Start a checkout as a signed-in user and look at what came back:

```bash
curl -s -X POST https://gate.example.com/api/me/recharge \
  -H "Cookie: nabu_console=<session>" -H "Content-Type: application/json" \
  -d '{"amount": 1}'
```

A `checkout_url` means the bridge is reachable and the signature is accepted.
`501` means payments are off; `502` carries the bridge's refusal verbatim.

Then settle without going to the bank:

```bash
curl -s -X POST https://gate.example.com/api/me/payments/settle \
  -H "Cookie: nabu_console=<session>"
```

`{"payments":[{"invoice":"…","paid":false,"amount":1}],"credited":false}` is
the correct answer for an unpaid invoice. An empty `payments` array with
`credited:false` means nothing is pending for this account.

## What the customer sees, and what to tell them

- **"Money left my card, balance unchanged"**: open Payments, press
  "بررسی وضعیت". If the bank confirmed, it credits immediately. Otherwise the
  invoice is pending and will be re-asked; an unconfirmed charge is reversed by
  the bank within 72 hours.
- **"I got an error page after paying"**: irrelevant. The invoice was bound to
  the account before they left. Sign in and open Payments.
- **"درگاه پرداخت آدرسی برای ادامه نداد"**: the bridge could not raise an
  invoice. Check the log for `payment bridge refused`.
- **"402 on my key after topping up"**: check the balance on the account
  screen. If it is still zero the payment is not confirmed. If it is positive
  the key belongs to a different account.

For support, ask for the invoice id from the Payments table, the amount and the
account email. Never ask for card numbers.

## Duplicate containers

The admin store is one JSON file held in memory and written back whole, with
usage debits batched between writes. Two processes on it are two wallets that
overwrite each other: a payment credited by one is lost on the other's next
write, or credited twice. The gateway therefore takes an exclusive lock on
`console.json.lock` at startup; a second instance waits up to 30 seconds for
the first to stop (a normal redeploy) and then exits with
`another instance holds the console state`. A Coolify deploy that left the
previous container running shows up as that line in the new container's log:
stop the stale one.
