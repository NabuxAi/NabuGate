# NabuGate — the organisation's central AI gateway

One OpenAI-compatible entry point that every Nabu project calls. Projects never
hold a provider key: they send an alias like `nabu-fast` to NabuGate, and the
gateway picks the provider, falls back across vendors, keeps the secrets,
enforces per-project quotas and records what was spent.

Go, standard library plus `gopkg.in/yaml.v3`. Builds to a static binary, ships
as a distroless image, listens on `8080`.

- **Gateway**: `https://gate.nabuxai.com/v1`
- **Console**: `https://gate.nabuxai.com/admin/`

## API

OpenAI-wire compatible, so any OpenAI client works by changing the base URL:

| Endpoint | Purpose |
|---|---|
| `POST /v1/chat/completions` | Chat, streaming or not |
| `POST /v1/responses` | Responses API |
| `POST /v1/embeddings` | Vectors, with `dimensions` |
| `POST /v1/images/generations` | Images |
| `POST /v1/audio/speech` | Text to speech |
| `POST /v1/audio/transcriptions` | Speech to text |
| `GET /v1/models` | Aliases, agents and passthrough catalogues |
| `GET /v1/agents` | Agents with their model and tool names |
| `GET /v1/usage` | Tokens and cost for the calling key |
| `GET /v1/photos/search` | Stock photos (Pexels) |

Request bodies pass through to the provider untouched — only `model` and the
stream flags are rewritten. So `tools`, `tool_choice`, `response_format`,
`seed`, `top_p`, `stop` and penalties all work, and `tool_calls` come back in
the response, whether or not the gateway names them.

## Bringing your own key

A request can carry its own upstream credentials and have the gateway spend
those instead of — or before — the ones this deployment holds:

```http
X-Nabu-Key-Gemini: AIza...
X-Nabu-Key-Openai: sk-...
X-Nabu-Key-Mode:   own-first
```

The header suffix is the provider name as the config spells it, matched
case-insensitively. The mode decides the order the two credential sources are
tried in:

| Mode | Meaning |
|---|---|
| `own` | Only your keys. A provider you sent no key for is skipped, and says so. |
| `own-first` | Yours, then the gateway's. **The default** when you send any key. |
| `global-first` | The gateway's, then yours as the backstop. |
| `global` | The gateway's only. The default when you send no keys. |

Two things follow from this:

- **Nothing is stored.** The keys live for the request and are gone with it.
- **A call served by your key is not billed here.** Your vendor already
  charged you; the gateway records the usage but not the cost.

A key is only honoured for a provider this deployment defines — it is not a
way to reach an arbitrary endpoint through the gateway. If a request tried
both credentials, the error names which one failed.

### Saving a key instead of sending it

The console's **Providers** screen lists every upstream the gateway knows of —
including ones it does not route to yet, which are the ones worth asking for —
and lets you save your own key rather than sending it on every request. Saved
keys are sealed with AES-GCM under `NABUGATE_SECRET_KEY`, are never returned by
any endpoint (you see the first characters, nothing more), and a header still
beats a saved key so a one-off override needs no visit to the console.

Without `NABUGATE_SECRET_KEY` set, the gateway **refuses** to store keys rather
than writing them in the clear, and the console says so.

Saved keys and grants are managed from the same screen, and an admin can revoke
a granted provider from the approval queue.

### Or asking for ours

If you would rather spend the gateway's credential, ask for it on the same
screen. Providers marked `access: auto` in the config grant it on the spot; the
rest queue for an admin, who approves and adds credit in one action. A grant
only ever concerns the gateway's own key: it can add access, never remove it,
and it never touches a key baked into the deployment's config.

### Calling it from a browser

The gateway sends no CORS headers unless `server.cors_origins` names some, so by
default it is server-to-server only and nothing is exposed. List the origins your
pages are served from and it answers preflights, echoes that one origin (never
`*`, which cannot carry credentials), and exposes the balance headers so a page
can read them:

```yaml
server:
  cors_origins:
    - "app.example.com"
    - "*.example.org"
```

This decides whether a browser may make the call. A key's own `allowed_origins`
still decides whether that key may be used from there, and both must pass.

## SDKs

One client per language, all covering the whole surface above.

| Language | Package | Registry |
|---|---|---|
| Node / TypeScript | `@nabugate/sdk` | npm |
| Python | `nabugate` | PyPI |
| Go | `github.com/nabuxai/nabugate-go` | Go modules |
| Rust | `nabugate` | crates.io |
| Dart / Flutter | `nabugate_sdk` | pub.dev |
| PHP / Laravel | `nabux/nabugate-laravel` | Packagist |

Each lives under `packages/` with its own README. See `packages/README.md` for
the release process.

```php
use NabuGate\Client\NabuGateClient;

$nabu = app(NabuGateClient::class);

$answer = $nabu->completeText(
    [['role' => 'user', 'content' => 'Summarise this quarter.']],
    'nabu-fast',
);

$nabu->stream(
    [['role' => 'user', 'content' => 'Write a haiku.']],
    fn (string $delta) => print($delta),
);
```

```python
from nabugate import NabuGateClient

nabu = NabuGateClient(api_key=os.environ["NABUGATE_API_KEY"])
for delta in nabu.stream([{"role": "user", "content": "Write a haiku."}]):
    print(delta, end="", flush=True)
```

## Sub-agents

A sub-agent is a named assistant — a system prompt plus default parameters
riding on an existing alias — defined entirely in config, with no code. It is
called exactly like a model:

```json
{ "model": "cine-motion-designer", "messages": [...] }
```

So any OpenAI-compatible client runs one in a single request, over the same
fallback chain. Agents appear in `/v1/models` and are governed by the calling
key's allow-list. `agents/` holds the Cinematic Scrollytelling squad.

An agent can also declare `tools:` — HTTP functions the **gateway executes
server-side** in a bounded tool-call loop, so a YAML file alone gives an agent
the ability to query a shop, a CRM, any API. The caller changes nothing; a
caller that sends its own `tools` keeps the plain pass-through. See
[`agents/README.md`](agents/README.md) for the schema and the safety rails
(SSRF guard, timeouts, step cap), and `agents/accountcity-support.yaml` for a
worked example.

## Console single sign-on

The admin console accepts a Nabu account through NabuAuth, restricted to an
explicit allow-list:

| Variable | Meaning |
|---|---|
| `NABUAUTH_URL` | NabuAuth base URL |
| `NABUAUTH_CLIENT_ID` | This gateway's client id |
| `NABUAUTH_CLIENT_SECRET` | Its secret; also signs the sign-in flow cookie |
| `NABUAUTH_REDIRECT_URI` | Defaults to `<host>/admin/api/nabu/callback` |
| `NABU_CONSOLE_NABUAUTH_ADMINS` | Comma-separated emails allowed into the console |

Proving who someone is says nothing about whether they may administer a gateway
that holds provider secrets and mints tokens, so the allow-list is a separate
decision from the sign-in. With no list the button stays hidden and the
endpoints refuse — an empty list reads as "nobody", never "everyone".

## Paying for credit

Wallet top-ups go through **NabuPay**, the payment bridge NabuDesk exposes. The
gateways themselves — Zarinpal, Aqayepardakht, Larapay, Stripe, PayPal, Polar,
NowPayments — are configured there, so no merchant credential lives in this
repo and no card detail passes through this app: the payer is handed to the
bank and comes back.

| Variable | Meaning |
|---|---|
| `NABUPAY_URL` | The bridge's base URL, e.g. `https://desk.nabuxai.com` |
| `NABUPAY_SECRET` | Shared secret; requests are signed `sha256("<app>:<ts>:<body>")` |
| `NABUPAY_APP_ID` | Identifies this caller to the bridge, default `gate` |
| `NABUPAY_GATEWAY` | Default gateway slug, default `zarinpal` |
| `NABU_PUBLIC_URL` | Where the gateway returns the payer; derived from the request when unset |

With `NABUPAY_URL` or `NABUPAY_SECRET` unset there is no gateway, and the panel
says top-ups are unavailable. That is the honest answer, and it is the one that
was missing while the button added to the balance without any money moving.

The balance moves only when the bridge says the gateway confirmed the payment.
Starting a top-up records the invoice as pending against the account and
credits nothing; the panel finishes it when the payer returns, by asking the
server to settle **its own** pending invoices. Nothing the payer's browser
carries is read — not the query the gateway appends, not an invoice number —
because a return URL is something anyone can type. Settling is safe to repeat,
so refreshing that page credits once.

Operator guide, including every log line and what to tell a customer:
[`docs/payments.md`](docs/payments.md). Customer-facing walkthrough:
[`docs/getting-started.md`](docs/getting-started.md), and the public page at
`/docs`.

## Running it

```bash
go build ./...     # build
go vet ./...       # static checks
go test ./...      # tests
go run ./cmd/gateway -config config.yaml
```

Adding a provider or an alias is usually a `config.yaml` edit and no code: add a
provider with a `type` and `api_key_env`, then map an alias to it under
`models`, `images`, `audio` or `embeddings`. A new adapter is only needed for a
provider that speaks none of the OpenAI, Anthropic or Gemini wire formats.

Secrets are read from the environment only, never written into the config or
the image. A provider whose key is unset is skipped, so the gateway still comes
up on whatever subset is configured.
