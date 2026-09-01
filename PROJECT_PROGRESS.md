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
4. **Config-file project keys are unmetered.** The balance gate and the debit in
   `RecordUsage` apply only to console-minted tokens with an owner. The eleven keys in
   `config.default.yaml` (`nabudesk`, `nabuwrite`, `rasad-gcc`, …) have no owner and no
   spend cap — usage is counted, nothing bounds it. Right for the org's own apps as long
   as they are the org's; wrong the day one of those keys is handed to a customer. A
   per-key `max_usd_per_day` in the policy is the smallest fix.
5. **An unpriced model is served free.** `usage.Cost` returns 0 for any `provider/model`
   missing from the price table, and nothing warns. Pinned by `TestUnpricedModelCostsZero`
   so the next person changes it on purpose: a boot-time check that every routable model
   has a price would close it.
6. **Overdraft is bounded by one request, per concurrent request.** The gate is a
   pre-flight `Balance <= 0`; a single expensive call, or several in flight at once, can
   take a balance below zero by their full cost. Every metered response now carries
   `X-Nabu-Balance-USD`, and `X-Nabu-Balance-Warning: low` under one dollar, so a caller
   can see it coming; a reservation per request is what would actually bound it.

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

---

## 2026-08-02 (later) — an alias that was advertised and failed every call

`nabu-embed` appears on `/v1/models` and answered every request with an error.
Its whole chain was dead:

| rung | provider | model | why it failed |
|---|---|---|---|
| primary | openai | text-embedding-3-small | `OPENAI_API_KEY` empty → provider skipped at start-up |
| fallback 1 | gemini | **text-embedding-004** | Google retired it — the live API answers **404** |
| fallback 2 | cloudflare | @cf/baai/bge-large-en-v1.5 | `CLOUDFLARE_API_KEY` unset → skipped |

This file already recorded that retirement a few aliases further down;
`nabu-embed` had simply not been updated with the others. It now names
`gemini-embedding-001`, verified against Google's API with this deployment's key
(200, where `text-embedding-004` gives 404).

Confirmed on the live gateway after deploying:

```
nabu-embed  → 1536 dims via gemini/gemini-embedding-001
chat-embed  → unchanged, still parspack
```

### The error hid the two facts that explained it

The fallback loops kept only the most recent error, so the message read:

```
all targets failed for embedding alias "nabu-embed": provider "cloudflare" not available
```

Cloudflare was the **last** rung and the least interesting. The unset key and
the retired model — the two things that actually explain the failure — were both
overwritten by it. Diagnosing this meant reading the config, checking three
provider keys, and calling Google directly.

Every rung's reason is now collected and reported in order, primary first, each
naming its provider and model. A skipped provider says "is its API key set?",
because in practice that is always what it means. Four tests pin it.

## Worth knowing — this alias now works, and that activates its width hazard

`nabu-embed` was previously **failing**, which was at least safe. Now that it
resolves, the deliberate width-crossing in its chain is live:

```
nabu-embed with    dimensions: 1536  → 1536 dims
nabu-embed without dimensions        → 3072 dims   (gemini-embedding-001's default)
```

That is by design for this alias — it is the "any width will do" chain for
instant, throwaway search. **It must still never be used to build a stored
index.** A caller that omits `dimensions` gets 3072-wide vectors, and a
collection pinned to 1536 will either reject the insert or, worse, end up split
across two incompatible vector spaces with nothing raising an error.

Consumers that store vectors should use a width-pinned alias — `chat-embed`,
`desk-embed`, `write-embed`, `zooey-embed` — exactly as they do today. Nothing
currently points at `nabu-embed`; `NABUCHAT_KEY` merely has it in its allow-list,
which is why the failure was found at all.

---

## 2026-08-02 (later) — swept every alias the gateway advertises

`nabu-embed` turned out to be advertised and broken, so the same question was
put to every other alias. Results against the live gateway with the admin key:

**Embeddings — all six healthy.** chat-embed, desk-embed and write-embed serve
1536 via parspack; gen-embed, zooey-embed and nabu-embed serve via gemini.
Zooey's contract was checked on both sides and is sound: `EMBED_DIMENSIONS = 768`
in its source, it sends `dimensions`, the alias honours it, and a guard test ties
the constant to its Qdrant collection.

**Chat — eight of thirteen failed**, every one with `provider "x" not
available`: ollama, avalai, gapgpt, arvan, cloudflare, tokenrouter, groq and
openai have no API key here, so they are skipped at start-up. Skipping is
deliberate. Advertising the aliases that depended on them was not — and
**dadebaran presents this catalogue directly as its users' model picker**, so
those were options a person could pick and could not use.

An alias is now listed only when a rung of its chain can be served. The owner
reported is the rung that will actually serve it, not the configured primary.

### Two mistakes made getting there

**The first filter hid three working aliases.** nabu-fast, nabu-smart and
nabu-cheap name a model and no provider — which per `config.Target` means the
model names a model-registry entry the router expands across serving providers
at resolve time. The filter read the empty string as "provider missing". An
empty provider now counts as reachable: what cannot be answered where the
catalogue is built should not cause a working model to disappear. Caught by
checking the live catalogue against live requests rather than trusting the
change.

**The error accumulator was applied to one loop out of six.** Chat still
reported only its last rung, which is how this whole thread started. All six
paths — chat, streaming, images, audio, Responses and transcription — now report
every rung.

### What the fixed error immediately revealed

```
all targets failed for model alias "nabu-kimi":
  dahl/moonshotai/Kimi-K2.6: dahl: invalid response (status 403):
  <!DOCTYPE html>… <title>Just a moment...</title>
```

`nabu-kimi` and `nabu-minimax` are not failing for want of a key. The **dahl**
provider is behind Cloudflare bot protection and answers the gateway with a
challenge page. The old message blamed `provider "openai" not available` — the
last rung, and entirely beside the point.

## Needs you

**Decide what to do about dahl.** `inference.dahl.global` returns a Cloudflare
interstitial to server-to-server requests, so `nabu-kimi` and `nabu-minimax`
cannot work as configured. Either the account needs an allowlist or a
non-challenged endpoint from dahl, or those two aliases should be pointed at
another provider — both models are available through parspack and tokenrouter.
They remain advertised because a reachable provider exists on paper; only trying
reveals the challenge page.

**The keyless providers are a choice, not a fault.** ollama, avalai, gapgpt,
arvan, cloudflare, tokenrouter and groq are all configured and skipped for want
of a key. Set any of their keys and the corresponding aliases reappear in
`/v1/models` on the next start with no code change.

---

## 2026-08-02 (final) — every advertised chat alias now works

`nabu-kimi` and `nabu-minimax` were the last two failing. Their primary is dahl,
which sits behind Cloudflare bot protection and answers this gateway with a
challenge page — HTTP 403 and "Just a moment...", not an API error.

Their only fallbacks named a **different model** on a provider with no key:
`nabu-kimi` fell back to `openai/gpt-4o`, `nabu-minimax` to
`groq/llama-3.1-70b-versatile`. So an alias named after a model had no rung that
could serve that model, and its one substitute was unreachable anyway.

Both models are served by **openrouter2**, which is configured and working. That
rung is now placed ahead of the unrelated-model fallback: an alias named after a
model should try that model on every vendor that has it before answering as
something else. dahl stays primary, so fixing its access later needs no further
change.

```
nabu-fast      ok via parspack        nabu-kimi     ok via openrouter2
nabu-smart     ok via parspack        nabu-minimax  ok via openrouter2
nabu-cheap     ok via parspack
nabu-vision    ok via parspack
nabu-parspack  ok via parspack
```

**7 of 7 advertised chat aliases work.** With the six embedding aliases already
verified, every alias this gateway advertises can now be served.

### A correction

The previous entry said Kimi and MiniMax were available through parspack and
tokenrouter. **That was wrong** — asserted without checking. They are on
openrouter2. The catalogue was the place to look and I had not looked.

## Needs you

**dahl access, if you want it.** `inference.dahl.global` returns a Cloudflare
interstitial to server-to-server requests. Either get an allowlist or a
non-challenged endpoint from them, or leave it: both aliases now serve their
real model through openrouter2, and dahl resumes taking the traffic
automatically the moment it starts answering.

**The keyless providers remain a choice.** ollama, avalai, gapgpt, arvan,
cloudflare, tokenrouter and groq are configured and skipped for want of a key.
Set any key and its aliases return to `/v1/models` on the next start, with no
code change.

## Image and audio: the categories that claim was never checked against

The entry above says "every alias this gateway advertises can now be served."
That was a chat-and-embedding result stated as a whole-gateway one. Image and
audio had not been tried, and neither category worked.

### Three branded image aliases pointed at a service that was already running

`nabu-card`, `nabu-header` and `nabu-story` route to the `imagegen` provider,
which is `mrc_imagegen` — deployed, healthy, and answering at
`imagen-api.nabuxai.com` the whole time. The gateway had no `MRC_IMAGEGEN_URL`
and no `MRC_IMAGEGEN_KEY`, so the provider was skipped at startup and all three
aliases were hidden from `/v1/models`. `nabu-photo` was hidden the same way for
want of `PEXELS_API_KEY`.

Both keys existed already, on `mrc_imagegen` itself (`MRC_API_KEY` for inbound
callers, `PEXELS_KEY`). Nothing was invented: the service authenticates with
`X-API-Key`, and the key was confirmed against the live endpoint — no key 401,
that key 200 — before being copied.

### nabu-story had never rendered anything

The gateway maps `nabu-story` to `kind: "story"`, and the renderer's API only
ever accepted `card`, `header` and `design`. Every call returned 422 from
pydantic. The canvas the alias promises simply did not exist.

It exists now (`mrc_imagegen@ed19439`): the story is the card at 9:16 rather
than a second layout, because it is the same brand card and two hand-tuned
layouts drift apart the first time either is edited. `build()` takes a height,
the bottom group moves by the full extra height and the content group by half,
and the 1350 card is unchanged — asserted element by element in the tests.

### The Gemini rung of every audio alias was structurally dead

`nabu-voice` failed twice over: OpenAI has no key, and Gemini answered 400
`Voice name alloy is not supported`. The gateway speaks OpenAI's wire protocol,
so every caller sends an OpenAI voice name, and the adapter forwarded it
verbatim — the fallback could never once serve the request it exists to catch.
Voice names are now translated, and `nabu-voice` returns a 177 KB WAV from
Gemini with no OpenAI key at all.

### nabu-9router was a chat alias filed under audio

It targeted `anthropic/claude-3-5-sonnet-20241022` from the `audio:` section,
so `/v1/audio/speech` asked a text model for speech. Moved to `models:`.

### Verified end to end, not asserted

```
nabu-card    ok   1,628 KB PNG        nabu-photo   ok    150 KB JPEG
nabu-header  ok   1,825 KB PNG        nabu-image   ok    150 KB JPEG
nabu-story   ok   2,251 KB PNG 2160x3840
nabu-voice   ok     177 KB WAV 24 kHz mono, via the Gemini fallback
```

**5 of 5 image aliases and the audio alias now work.** Together with 7/7 chat
and 6/6 embedding, the claim the previous entry made is finally true — with the
one exception below.

## Needs you

**9Router needs its one-time dashboard setup.** `nabu-9router` fails with 401
`API key required for remote API access`. The self-hosted service holds its
provider hookups in the `ninerouter-data` volume and issues its own key from
its dashboard; nothing here can produce that key. Until it is set, the alias
fails loudly rather than pretending. The dashboard has no built-in auth and
holds provider tokens — put Basic Auth or an IP allow-list in front of it in
Coolify before opening it.

**`nabu-voice` still has no primary.** It works today only because the Gemini
fallback finally understands the request. Set `OPENAI_API_KEY` if you want
`gpt-4o-mini-tts` as the primary rung.


## The stored-index rule is enforced now, not written down again

`write-embed`'s own comment named this hazard precisely — *"split the corpus
across two incompatible embedding spaces with no error at all"* — and a later
change added a second rung beside the first anyway, reasoning that both are 1536
wide. **The comment was read and overridden.** Writing it in a third place would
not have helped.

Three assertions over `config.default.yaml` now carry it:

- the five stored-index aliases have **no** fallback rungs;
- **every** embedding alias is classified as stored or query-time, so a new one
  fails until someone decides which it is. `write-embed` did not acquire its
  second rung from ignorance of the rule — the question was simply never put;
- a stored-index alias points at a provider type that forwards the caller's
  `dimensions`, which is how a 3072-wide vector reaches a 1536-wide column.

`nabu-embed` remains the deliberate exception and is registered as one.

### Both regressions reproduced before committing

```
re-add the chat-embed fallback   -> FAIL "chat-embed has 1 fallback rung(s)…"
add an unclassified alias        -> FAIL "brand-new-embed is not classified…"
config restored                  -> ok
```

Each message names the alias and what to do about it, because a test that only
says "assertion failed" sends the next person to read the same comment that did
not work the first time.

### A first draft of this was a bad test

It asserted that the `nabu-embed` comment still contains the word "stored".
That is testing prose: the comment says *"stores the vectors"* and *"persisted
index"*, so it failed on wording rather than meaning. Replaced with the
structural check above. Worth recording — a brittle test that fails for the
wrong reason teaches people to delete tests.


## The alias sweep is a command now

`/healthz` returns `{"status":"ok"}` — the process is up, which is not the
question anyone has. This gateway answers `/healthz` perfectly while serving
none of the aliases a given consumer asks for, and the first sign is an opaque
failure inside somebody else's product. That is exactly how NabuChat's retrieval
came to look like a broken vector database.

Every alias in this repo was verified by hand this week, one request at a time,
and none of it was repeatable. `gateway -selftest` does the sweep: each
advertised chat and embedding alias is asked to do the smallest real piece of
work it can, and the exit status is non-zero if any fail — so it can gate a
deploy rather than be read once.

Image and audio sit behind `-selftest-all`. They cost money per call, and
billing whoever runs a health check is how the check stops being run.

**The embedding case does more than call the alias.** It records the natural
width, then asks for 1536 and verifies it gets 1536. Honouring `dimensions` is
the property every consumer storing these vectors depends on, and it is
invisible until it fails: `gemini-embedding-001` answers **3072** by default, so
a caller with a `vector(1536)` column that silently receives 3072 either fails
its insert or corrupts the column — far from here, with nothing pointing back.

### Run against the live deployment

```
13/20 aliases answered

6/6 embedding aliases  natural width recorded, all honour dimensions=1536
8/15 chat aliases      answered
7   chat aliases       failed — every one an unset provider key, except
                       nabu-9router (401, its dashboard setup is still owed)
```

Nothing in that is new. It is the same picture the manual sweep produced days
ago — the difference is that it now takes one command, and says so out loud
instead of living in a progress note.

## 2026-08-03 — the gateway was serving a build from before its own changes

`nabu-ocr` was added to the config, committed and pushed. The running gateway
started 2026-08-02 21:25 and had none of it — nor the flows feature merged
around the same time.

Nothing in this repository deployed it. Only CI, which tests and stops. A survey
of twelve repositories in this fleet found **one** with a deployment workflow,
and that one had been added an hour earlier for the same reason.

`deploy.yml` triggers Coolify after CI passes and waits for the result. Firing a
webhook and exiting is the version of this job that is always green: it reports
that a request was sent, which is not a build that succeeded and a container
that started. Failed, cancelled, or still running after fifteen minutes each
fail the job.

The trigger uses `--fail-with-body`, because a bare `curl` exits 0 on a 401 —
the trap that had nabuchat's monthly usage reset reporting success for two
months while resolving a dead hostname.

One step here that nabuchat's version does not have: after the deploy, ask the
gateway whether it answers. A deploy that finishes and serves nothing is not a
successful deploy, and "the container started" is a different claim from "the
aliases answer". Skipped when `NABUGATE_SELFTEST_URL` is unset rather than
failing, because a deployment with no public URL is a normal thing.

Verified with the variables unset: `::error::COOLIFY_API_URL is not set`.

## Needs you

```
COOLIFY_API_URL         https://cp.nabuxai.com/api/v1
COOLIFY_RESOURCE_UUID   de5lmm5tewiy3kfyjz7y6tl8
COOLIFY_API_TOKEN       Coolify > Keys & Tokens, scoped to this resource
NABUGATE_SELFTEST_URL   optional — the gateway's own origin, to check it answers
```

The UUID was read from the running container's name, not guessed. The token has
to come from your panel.
