---
name: nabugate
description: Connect any project to NabuGate, the org's central AI gateway. Use whenever a project needs an LLM, embeddings, image generation, TTS, or a named AI assistant — instead of calling OpenAI/Anthropic/Gemini directly, or adding a provider SDK or API key to the project. Also use when minting a per-app token, restricting a key by origin, reading real usage, or debugging "all targets failed", empty model responses, wrong embedding dimensions, or a Coolify deploy that exits with no logs.
---

# NabuGate

NabuGate is the single AI entry point for every project in this org. **No project
contains AI.** Projects hold no provider key, import no provider SDK and know no
vendor. They speak the OpenAI wire protocol to the gateway, name an alias or an
agent, and the gateway picks the provider, handles fallback, holds the secrets,
enforces per-app quota and records real cost.

- Repo: `NabuxAi/NabuGate` — Go 1.24, stdlib + `gopkg.in/yaml.v3` only
- Production: `https://gate.nabuxai.com/v1`
- Console: `https://gate.nabuxai.com/admin/`

## The one rule

**A project's only AI credential is a NabuGate project token.** If you are about
to write `OPENAI_API_KEY` into a project, stop — that key belongs in the gateway.

This is not bureaucracy. A key found in one project's env was labelled
`OPENAI_API_KEY` and was actually an **OpenRouter** key; nothing downstream could
tell which vendor it was for. Another project ran on the gateway's **admin key**,
so its spend was indistinguishable from everyone else's and a leak would have
handed over the whole gateway.

## Connecting a project — the short version

1. Open the console, sign in, **Tokens → + new token**.
2. Give it the app's name (usage is attributed to it), an allow-list, and — if
   the key ships inside a web app — the origins it may be used from.
3. Copy the secret. **It is shown once**; only its hash is stored.
4. In the project:

```bash
NABUGATE_BASE_URL=https://gate.nabuxai.com/v1
NABUGATE_API_KEY=<the secret>
```

5. Call it like OpenAI:

```bash
curl $NABUGATE_BASE_URL/chat/completions \
  -H "Authorization: Bearer $NABUGATE_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"model":"nabu-fast","messages":[{"role":"user","content":"سلام"}]}'
```

Endpoints: `/chat/completions`, `/responses`, `/embeddings`,
`/images/generations`, `/audio/speech`, `/models`, `/usage`.

Chat bodies pass through transparently — only `model` and the stream flags are
rewritten, so `tools`, `tool_choice`, `response_format`, `seed`, penalties all
work and `tool_calls` come back. Do not break this.

`GET /v1/models` with a project token lists exactly what that token may reach.
It is the fastest way to check a token's scope.

## Aliases, agents, registry, passthrough

**Alias** — a named routing chain: `nabu-fast`, `nabu-smart`, `nabu-cheap`,
`nabu-vision`, `nabu-embed`. Portable across deployments.

**Agent** — a system prompt + default params layered on an alias, addressable
exactly like a model. One YAML file, **no code**:

```yaml
name: myproject-writer
description: "What it does."
model: nabu-smart
temperature: 0.7
max_tokens: 2048
system: |
  You are …
```

Then `{"model": "myproject-writer", …}` runs it in one request with the
router's fallback intact.

**Model registry** — a model is an identity, not a provider coordinate.
`gpt-5.5` is the same model whether Parspack, AvalAI or GapGPT serves it, and
each spells the upstream name differently:

```yaml
model_registry:
  gpt-5.5:
    param_style: reasoning
    serves:
      - { provider: parspack, model: "openai/gpt-5.5" }
      - { provider: avalai,   model: "gpt-5.5" }
      - { provider: gapgpt,   model: "gpt-5.5" }
```

A target may then name the model alone and the router expands it into one
attempt per serving provider, skipping any without a live key. **A provider
going down switches to the next one serving the same model** — the model never
changes, only who serves it. Naming a pinned coordinate
(`parspack/openai/gpt-5.5`) still falls back to the same model elsewhere.

**Passthrough** — address any model of a multi-model provider directly as
`<provider>/<model>`. Grant with `allow: ["parspack/*"]`.

## Where agent files must live

The image bakes `agents/` at build time. **An agent that lives only in the
consuming project's repo never reaches the gateway.** Commit it to
`NabuxAi/NabuGate` too, with a header naming the source of truth.

## Hard-won rules

Every one of these came from something breaking in production.

### A fallback must cross vendors

Two models behind the same key share one quota and one outage — that is one
target, not two. A chain of `gemini-2.0-flash-lite → gemini-2.0-flash` fails
twice at once. End every chain on a genuinely different provider.

### A flaky provider needs several rungs of its own

Some providers' per-model availability moves minute to minute: the same model
returns content, then a 502, then an empty body. Give such a provider two or
three model rungs, not one.

### Reasoning models speak a different parameter dialect

`gpt-5.x` and the o-series replaced `max_tokens` with `max_completion_tokens`
and accept only the default temperature. Sending the classic parameters is
rejected upstream — and Parspack surfaces that rejection as an **HTML 502
page**, which reaches the client as "empty completion" and is close to
untraceable. Declare `param_style: reasoning` on the model.

The dialect is **not inferable from the name**: both `gpt-4o-*-search-preview`
models speak it while plain `gpt-4o` does not. Probe, do not guess.

### An image alias is not always a picture generator

`nabu-image` and `nabu-photo` answer a prompt with a scene: one draws it, the
other finds a photograph of it. `nabu-header`, `nabu-card` and `nabu-story` do
not. They render mrc_imagegen's fixed layouts — a kicker, two headline lines, a
theme, the brand's typeface and palette — so the prompt is **copy to be set**,
not a scene to imagine. Ask one of them for "a cat on a roof" and you get those
words in the brand's typography.

That is also why they carry no fallback. A diffusion model standing in for a
branded header returns something that is not the brand at all, and a caller who
asked for the brand would rather see an error than a stranger's design.

For exact control send JSON in the prompt — `kicker`, `head1`, `head2`,
`theme`, `brand`, `palette`, `design` — which is what a caller that already
wrote its copy should do. Plain text is split across the two headline lines and
clipped to what the canvas holds; that is a convenience, not composition. When
the copy matters, get it from the `mrc-imagegen-writer` agent (one chat call,
usually one the caller is already making) and pass the result through.

### Never let a stored index cross vector widths

`nabu-embed`'s chain crosses widths on purpose (1536 → 768 → 1024). That is fine
for search performed at query time and **wrong for anything that stores the
vectors**: a mid-flight fallback either fails the insert or splits the corpus
across two incompatible embedding spaces with no error raised.

Any project with a fixed-width vector column gets its **own alias**, whose
fallbacks are all the same width:

```yaml
embeddings:
  myproject-embed:
    primary:
      provider: parspack
      model: "openai/text-embedding-3-small"   # natively 1536
    fallback:
      - provider: gemini
        model: "gemini-embedding-001"          # 1536 only because we ask for it
```

Send `dimensions` on every request and verify the width before inserting.
`gemini-embedding-001` defaults to **3072**, and pgvector caps HNSW at 2000.
Store `embed_model` and `embed_dim` per row so a stale index is detectable.

Existing aliases: `write-embed`, `chat-embed`, `desk-embed` (1536),
`gen-embed` (1024).

### An empty response is a failure

An upstream can close a stream having emitted a role delta, a stop and no
content, while the same model answers fine non-streaming. The router treats that
as a failed target and continues the chain. **In a client, never present an
empty generation as success** — and never log it, or an "accepted" empty result
becomes a style exemplar.

### Some models leak their reasoning into `content`

`moonshotai/kimi-k2.6` and `minimax/minimax-m2.7` on Parspack return their chain
of thought in the content field — no separate reasoning field, no delimiter to
strip. The caller gets "The user says … the persona is …" where an answer should
be.

MiniMax does it **intermittently**, which is worse than always: the same prompt
answers cleanly once and leaks the next time, so it reads as a flaky model
rather than a broken one. Probe a model more than once before trusting it at the
head of a user-facing alias.

## The console

`https://gate.nabuxai.com/admin/`

Username and password. The first account is created through a setup form that
refuses forever after — so **create it immediately** on a fresh gateway; until
you do, anyone who finds the URL can become the admin.

What it does:

- **Tokens** — mint one per app. An allow-list is required: a token that reaches
  everything is an admin key, and a console form is exactly how one gets created
  by accident. Optional origin restriction and rate limit.
- **Usage** — real, per-app, and persisted across redeploys. Denied requests are
  counted next to successful ones, so an app with the wrong origin or a revoked
  token is visible rather than merely absent.

### Origin filtering

A key that ships inside a web app cannot be kept secret, so the gateway also
checks where the request came from — matched against `Origin`, falling back to
`Referer`.

```
*.nabuxai.com     matches app.nabuxai.com, a.b.nabuxai.com, nabuxai.com
                  does NOT match evil-nabuxai.com
```

The wildcard is anchored at a dot boundary on purpose. A token with an origin
list **refuses a request with no Origin at all** rather than assuming a
non-browser caller is fine. This is not a defence against a non-browser client,
which can send any header — that is what the token is for.

## Adding a provider or alias

Almost always config-only: add a provider with `type` and `api_key_env`, map an
alias to it. A new adapter is needed only when the provider is not wire
compatible with OpenAI / Anthropic / Gemini. **A provider whose key env is empty
is skipped automatically**, so listing one costs nothing until someone signs up.

Providers with a permanent free tier already listed and inert until keyed:
Cerebras, Mistral, NVIDIA NIM, GitHub Models, SiliconFlow, Together, Cohere.
(Mistral's 1B-token tier requires opting into training on your data — route
nothing confidential through it.)

## Debugging

| Symptom | Cause |
|---|---|
| `all targets failed: provider "x" not available` | that provider's key env is empty — every rung below is dead |
| `provider: empty completion` | upstream returned 200 with no content; the chain continues |
| Empty draft, no error | the client treated an empty stream as success |
| HTML instead of JSON from a provider | classic params sent to a reasoning model — set `param_style` |
| Insert fails on the vector column | the embedding alias fell back to a different width |
| 401 with a valid-looking key | wrong token, or its `allow` glob excludes the alias |
| 403 "not permitted from this origin" | the token has an origin list and this request did not match |
| Model answers with meta-analysis | a model that leaks reasoning into `content` |

```bash
GET /v1/models    # what this token may reach — fastest scope check
GET /v1/usage     # per-model counts; a model with 0 requests is silently falling back
```

`/v1/usage` is the honest answer to "which model is actually serving this?"
Config states intent; usage states fact.

## Deploying on Coolify

- Deploy by **pushing to `main` and redeploying**. Do not hand-edit the server.
- Use `expose`, **never `ports`**. The proxy already holds host 80, 443 and 8080;
  a published port collides, the bind fails, and the container exits *before it
  can log anything* — which reads as a failure with no cause. This has broken
  two services here.
- No `container_name` and no fixed image tag: Coolify derives both per
  deployment and hard-coding them collides across redeploys.
- Give a public domain to **one** service per compose. Two services both
  declaring `SERVICE_FQDN_*` get handed the same hostname.
- Use `https://` in the domain so a certificate is issued — a `Secure` session
  cookie is never sent over plain HTTP and login fails silently without one.
- Internal services (Postgres, Qdrant, MinIO, mail catchers) get **no** public
  domain.
- A distroless image running as nonroot cannot write to a root-owned volume, and
  has no shell to fix it at runtime. Create the directory in the build stage with
  `--chown=65532:65532`.
- `SERVICE_PASSWORD_*` variables that Coolify generates can silently diverge from
  an app's own `*_PASSWORD` / `*_API_KEY` var. Both Postgres and Qdrant broke
  this way here. Compare the values inside both containers, not in the UI.

**Check the load before deploying.** That box runs ~146 containers on 8 cores and
its nightly `mariadb-dump` has pushed load average past 700. A build during that
can take live customer sites down with it. `uptime` first; wait if it is high.

## Reference implementations

| project | shows |
|---|---|
| `NabuxAi/NabuWrite` | agents as YAML, no-fallback embedding alias, `dimensions` on every embed, width verified before insert, empty-generation guard |
| `NabuxAi/NabuSu` | the narrowest possible token — one agent only, because its safety limits live in that agent's prompt |
| `NabuxAi/NabuChat` | gateway-backed chat + a width-pinned embedding alias for a Qdrant index |
