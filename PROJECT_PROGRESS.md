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

## Fixed 2026-08-02 — `encoding_format` was dropped, silently

`/v1/embeddings` decoded the request into a typed struct that had no
`encoding_format` field, so the parameter was discarded, and every response was
a JSON array of floats.

That is not cosmetic. **The official OpenAI SDK sends `encoding_format: "base64"`
by default**, so most callers ask for it without ever choosing to — and a client
that asks for base64 decodes whatever comes back as packed little-endian
float32. Given a JSON array it produces a vector a quarter of the promised
length: 384 floats where 1536 were expected. Nothing raises. The vector store
then refuses the write, the ingest reports success, and the datastore stays
empty.

Found while proving NabuChat's retrieval pipeline: its embeddings came back at
384 dimensions and the upsert wrote nothing, with no error anywhere.

The handler now accepts the field, encodes base64 as little-endian float32
(narrowing from the float64 the gateway carries — a precision no embedding model
actually provides), leaves `float` and an omitted value exactly as they were,
and returns 400 for anything else rather than quietly sending floats to a caller
waiting on base64.

Proven end to end against a running gateway, not just in unit tests:

```
encoding_format: base64   → string, decodes to 1536 floats
encoding_format: float    → array of 1536 floats
omitted                   → array of 1536 floats   (unchanged behaviour)
encoding_format: hex      → HTTP 400
```

and then NabuChat's own `QdrantManager` driven through this gateway: three
chunks embedded, upserted into real Qdrant, searched, ranked correctly, removed.

Covered by `internal/server/embed_encoding_test.go` — five cases, all asserting
the *decoded* vector rather than the shape of the string, because length is what
breaks a vector store.

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

---

## 2026-08-02 — six consumers were dialling a host that is not there

Three services were found separately today with `NABUGATE_BASE_URL` pointing at
`nabugate.nabuxai.com`. That is not the gateway. It answers **503**;
`gate.nabuxai.com` answers 401 without a key — alive and asking for one.

Since the same fault kept appearing, every stored environment variable across
all Coolify applications was swept for hostnames, and each distinct external
host was probed. 77 hosts; the dead ones that an application actually dials:

| service | variable | was |
|---|---|---|
| NabuChat | `NABUGATE_BASE_URL` | `nabugate.nabuxai.com/v1` |
| NabuDesk | `NABUGATE_BASE_URL` | `nabugate.nabuxai.com/v1` |
| dadebaran.ir | `NABUGATE_URL` | `nabugate.nabuxai.com` |
| mrc_imagegen | `NABUGATE_HOST` | `nabugate.nabuxai.com` |
| internship | `LLM_BASE_URL` | `nabugate.nabuxai.com/v1` |
| bootcamp | `LLM_BASE_URL` | `nabugate.nabuxai.com/v1` |

All six corrected. **dadebaran.ir is a live public product** whose model picker
is this gateway's catalogue — its key lists 135 models and it had been posting
every request to a dead host. mrc_imagegen's key lists 489.

Two of the six were wrong in the **repository**, not only in the stored value —
`mrc_imagegen` (Dockerfile and compose default) and `internship` (both compose
files, `.env.example`, and a test that asserted the dead address, which is how
it survived: the suite agreed with the bug). Those are fixed in code. The other
four were stored values overriding correct defaults, invisible from the code.

Verified after redeploying: each container now holds the corrected value, and
dadebaran fetches the model list from inside its own container.

### What this says about the failure mode

A stored environment variable silently overriding a correct default produces a
service that is healthy, logs nothing useful, and does none of its work. It
cannot be found by reading the repository, because the repository is right. The
sweep took minutes and found four more instances of a fault that had been found
three times by accident.

Worth repeating whenever a host is renamed: probe every stored value, not the
code.

## Needs you

**Two bots have no LLM credential at all.** Both now point at the right gateway
and can still call nothing, because both `LLM_API_KEY` and `ANTHROPIC_API_KEY`
are empty:

- **bootcamp** (`bootcamp.nabuxai.com`) is configured with `AI_MODEL=nabu-smart`,
  so it is plainly meant to use this gateway. It needs a project key. Minting one
  is a one-line change here, but whether that bot should be running at all is
  your call, not something to decide by giving it credentials.
- **internship** (`internship.nabuxai.com`) is configured with
  `AI_MODEL=claude-opus-4-8` against Anthropic directly. That needs your
  Anthropic key — or point `AI_MODEL` at a `nabu-*` alias and give it a gateway
  key instead, which is the arrangement everything else here uses.

Both are Telegram bots with no web surface, so a 404 on their domain is correct
and not a fault. Both containers are healthy.
