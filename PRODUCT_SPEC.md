# NabuGate — product spec

Reconstructed from the repository on 2026-08-02 by reading the router, providers, policy
layer, console and tests.

## What it is

One OpenAI-compatible endpoint that every Nabu project calls instead of holding a provider
key of its own. A project sends an alias — `nabu-fast`, `nabu-smart`, `nabu-vision` — and
the gateway picks a provider, falls back across vendors when one fails, keeps every secret
on its own side, enforces per-project quotas, and records what was spent.

The point is the inversion: a leaked project key costs one project's quota, not the
organisation's OpenAI account.

## Stack

Go 1.24, standard library plus `gopkg.in/yaml.v3`. Static binary, distroless image, port
`8080`. No database — state is a JSON admin store on disk plus in-memory usage counters.
An embedded SPA (`web/dist`) serves the console at `/admin/`.

## Users

| Type | Uses |
|---|---|
| A Nabu project (NabuDesk, NabuGen, NabuWrite, …) | `/v1/*` with a project key |
| Operator | the console at `/admin/` — mints keys, edits agents, reads usage |
| End user | never touches it directly |

## Surface

`POST /v1/chat/completions`, `/v1/responses`, `/v1/embeddings`, `/v1/images/generations`,
`/v1/audio/speech`; `GET /v1/models`, `/v1/usage`, `/v1/photos/search`;
`GET|DELETE /v1/conversations[/{id}]`; `GET /healthz`. Console API under `/admin/api/*`.

Request bodies pass through to the provider untouched — only `model` and the stream flags
are rewritten — so `tools`, `tool_choice`, `response_format`, `seed` and the rest work
whether or not the gateway knows their names.

## Main journeys

1. **Operator sets up** — first visit to `/admin/` offers first-run setup, then sign-in
   (local password or Nabu SSO against an admin allow-list).
2. **Mint a project key** — with an allow-list of aliases, a rate limit, and optionally an
   origin allow-list so the key is safe to embed in a browser app.
3. **A project calls the gateway** — key authenticated, policy checked, alias routed to a
   provider, falling back on failure; tokens and cost recorded per project and model.
4. **Define an agent** — a named system prompt plus default parameters over an alias,
   loaded from `agents/*.yaml`, callable as a model name.
5. **Watch spend** — console overview and `/v1/usage`.

## Business rules inferred from the code

- A project key carries an alias allow-list; anything outside it is refused, so a key
  cannot reach a model it was not issued for.
- Origin allow-lists are checked per request, because a key embedded in a web app cannot be
  kept secret and the origin is the second factor.
- Fallback is multi-layer: alias → target list, tried in order.
- Usage is recorded per project *and* model, with cost derived from configured pricing.
- The console shell is served unauthenticated because it contains the login form; every
  endpoint that returns data requires a session cookie.
- Provider secrets live only in `config.yaml`, which is gitignored and has never been
  committed.

## External integrations

OpenAI (and OpenAI-compatible: Groq, OpenRouter, AvalAI, GapGPT, Cloudflare Workers AI,
TokenRouter), Anthropic, Google Gemini, ArvanCloud AIaaS (`auth_scheme: apikey`), Pexels for
stock photos, NabuAuth for console SSO. Six published SDKs under `packages/` (Node, Python,
Go, Rust, Dart, PHP).

## Complete

Routing and fallback, all six OpenAI-wire endpoints, streaming, agents, policy enforcement,
per-project usage and cost, conversation memory, the photo proxy, the admin console with
local and SSO sign-in, and the SDKs.

## Fixed in this pass

Console sign-in had no brute-force protection — see `PROJECT_PROGRESS.md`.

## Release requirements

`config.yaml` with real provider keys, an admin-store path so the console mounts at all,
and TLS terminated in front of the container.

## Assumptions

- Single process: in-memory throttle and usage counters are not shared across replicas.
- The gateway is deployed behind a reverse proxy, so `X-Forwarded-For` is trustworthy.
