# NabuGate — progress

Last updated 2026-08-02. Companion to [`PRODUCT_SPEC.md`](PRODUCT_SPEC.md).

## Done in this pass

**Console sign-in is rate-limited.** It was not, and it is the single door to everything
the gateway protects: minting project keys, editing agents, adding admins, resetting usage.

`adminstore.Authenticate` was already careful — PBKDF2, constant-time comparison, and dummy
key derivation for a username that does not exist so absence cannot be timed. What was
missing sat one level up: nothing counted failures. An attacker could guess at whatever
rate they could send, against a password that only has to clear a ten-character minimum.
The `RateLimit` field already in the codebase applies to project API tokens, not to this.

Added `internal/server/throttle.go`: eight attempts, then a ten-minute lockout, keyed on
**username and source address together**. Neither alone works — keying on the username lets
anyone lock a real admin out by guessing at their name, and keying on the address lets one
host walk through every account. Failures are now logged with the username and IP, which
previously went unrecorded.

Deliberately the same shape as NabuAuth's throttle: the two services are run by the same
operators, and having them behave differently under attack would only make both harder to
reason about.

Covered by `internal/server/throttle_test.go` — the limit, window expiry, reset on success,
both key-choice failure modes, and `X-Forwarded-For` parsing.

## Checked and found healthy — do not re-investigate

- **No secrets in git.** `config.yaml` is gitignored and has never been committed; only
  `config.default.yaml` and `config.example.yaml` are tracked.
- **Every `/v1/*` endpoint is behind `s.auth`**, including the conversation API.
- **Every console endpoint that returns data is behind `consoleAuth`.** The SPA shell is
  intentionally public because it carries the login form, and it carries no gateway key —
  a past design that did was already corrected and is documented in the code.
- **First-run setup cannot be replayed.** `consoleSetup` refuses once an admin exists.
- **`go vet` clean, all 8 test packages pass.**

## Remaining work

1. **Throttle state is per-process.** Fine for the current single-container deployment;
   if the gateway is ever replicated, an attacker can spread guesses across replicas. Worth
   revisiting only when a second replica is actually run.
2. **No password complexity rule beyond the ten-character minimum.** With the throttle in
   place this is much less pressing; the throttle is the control that matters.
3. **Origin allow-lists are opt-in per token.** A token minted without one is unrestricted,
   which is correct for server-side callers and wrong for a browser app — worth a warning in
   the console when a token is created without origins and is likely to be embedded.

## Validation

```bash
go build ./...
go vet ./...
go test ./...
go test ./internal/server/ -run Throttle -v
```

## Needs you

Nothing is blocked. Provider keys in `config.yaml` and an admin-store path are deployment
configuration, already documented in `config.example.yaml`.
