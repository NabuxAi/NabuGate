# NabuGate — Central AI / LLM Gateway

NabuGate is a single, OpenAI-compatible entry point for every project in the
org. Projects **never** talk to OpenAI, Gemini, Claude, Groq or OpenRouter
directly — they call NabuGate with a model **alias** (e.g. `nabu-fast`), and the
gateway handles provider selection, fallback, secrets, and observability.

```
project ──▶ POST /v1/chat/completions { "model": "nabu-fast", ... }
                       │
                  ┌────▼─────┐
                  │ NabuGate │  auth → router → provider adapter → fallback
                  └────┬─────┘
        ┌──────────────┼───────────────┬───────────────┐
     Dahl           OpenAI          Groq / Anthropic / Gemini   (OpenRouter / Parspack…)
```

The org's default upstream is **Dahl** (`inference.dahl.global`, OpenAI-wire),
serving open models such as MiniMax and Kimi; the hosted vendors act as
fallbacks.

## Components

| Component          | Responsibility                                             |
| ------------------ | ---------------------------------------------------------- |
| **AI Gateway**     | Single entry point for all projects (`internal/server`)    |
| **Provider Adapter** | Translate the unified request to each vendor's API (`internal/provider`) |
| **Model Registry** | Alias → provider/model table (`models:` in `config.yaml`)  |
| **Router**         | Pick the target for a task/alias (`internal/router`)       |
| **Fallback Engine**| If the primary fails, try the next target (`internal/router`) |
| **Observability**  | Structured JSON logs: latency, tokens, cost, status        |
| **Cost Tracking**  | Per-project / per-model token + USD totals (`internal/usage`) |
| **Policy Engine**  | Per-key alias allow-list + request rate limit (`internal/policy`) |
| **Secret Manager** | API keys live in env vars, never in code or project repos  |

## API

OpenAI-compatible, so existing SDKs work — just point `base_url` at NabuGate and
use a `nabu-*` alias as the model name.

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer nabu_dev_key_change_me" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "nabu-fast",
    "messages": [{ "role": "user", "content": "سلام، خلاصه کن" }]
  }'
```

Response (note the extra `provider` / `upstream_model` fields and
`X-Nabu-Provider` / `X-Nabu-Model` headers showing what actually served it):

```json
{
  "object": "chat.completion",
  "model": "nabu-fast",
  "provider": "groq",
  "upstream_model": "llama-3.1-70b-versatile",
  "choices": [{ "index": 0, "finish_reason": "stop",
                "message": { "role": "assistant", "content": "…" } }],
  "usage": { "prompt_tokens": 5, "completion_tokens": 3, "total_tokens": 8 }
}
```

Other endpoints:

| Method & path                | Description                                          |
| ---------------------------- | --------------------------------------------------- |
| `POST /v1/chat/completions`  | Chat completion (alias- or passthrough-routed); supports `stream: true` |
| `POST /v1/responses`         | OpenAI Responses API, proxied transparently (JSON or SSE) |
| `POST /v1/images/generations`| Image generation; returns `data[].b64_json`         |
| `POST /v1/audio/speech`      | Text-to-speech; returns raw audio bytes (wav/mp3)   |
| `POST /v1/embeddings`        | Text embeddings; `input` may be a string or array   |
| `GET  /v1/models`            | List aliases, passthrough providers' discovered models **and** sub-agents |
| `GET  /v1/usage`             | Accumulated token usage and cost (per project/model)|
| `GET  /healthz`              | Liveness probe                                      |

Image example:

```bash
curl -X POST http://localhost:8080/v1/images/generations \
  -H "Authorization: Bearer nabu_dev_key_change_me" \
  -d '{ "model": "nabu-image", "prompt": "a calm minimal illustration", "n": 1 }'
```

Speech example (saves an audio file):

```bash
curl -X POST http://localhost:8080/v1/audio/speech \
  -H "Authorization: Bearer nabu_dev_key_change_me" \
  -d '{ "model": "nabu-voice", "input": "سلام", "voice": "Kore" }' \
  --output speech.wav
```

Embeddings example:

```bash
curl -X POST http://localhost:8080/v1/embeddings \
  -H "Authorization: Bearer nabu_dev_key_change_me" \
  -d '{ "model": "nabu-embed", "input": ["متن اول", "متن دوم"] }'
```

Streaming example (Server-Sent Events). The gateway normalizes every provider's
stream into OpenAI-style `chat.completion.chunk` events ending with `[DONE]`:

```bash
curl -N -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer nabu_dev_key_change_me" \
  -d '{ "model": "nabu-fast", "stream": true,
        "messages": [{ "role": "user", "content": "خلاصه کن" }] }'
```

Fallback applies only until the first byte streams; once output has started the
gateway is committed to that provider.

### Parameter passthrough & tools

For OpenAI-wire providers (OpenAI, Groq, OpenRouter) the whole request body is
forwarded upstream verbatim — the gateway overrides only `model` (alias →
upstream) and the streaming flags. So every OpenAI parameter passes through
untouched: `tools` / `tool_choice` (function calling), `response_format` (JSON
mode), `top_p`, `stop`, `seed`, `presence_penalty`, `frequency_penalty`, `n`,
`user`, … and `tool_calls` come back in the response. Anthropic and Gemini map
the common typed params (`temperature`, `top_p`, `max_tokens`, `stop`); native
tool translation for those two is a follow-up. Transient upstream failures
(network, 429, 5xx) are retried with backoff before the router moves to the next
fallback target.

### Multi-model providers (passthrough & discovery)

Some upstreams — **Parspack AI Studio**, OpenRouter, Groq — are not a single
model but a provider hosting dozens. Writing one alias per model does not scale.
Mark such a provider `passthrough: true` in `config.yaml` and it becomes a
first-class namespace:

- **Direct routing, no alias.** Address any of its models as
  `"<provider>/<model>"`. The gateway splits on the **first** `/`, so a
  vendor-namespaced upstream ID keeps its own slashes:

  ```bash
  curl -X POST http://localhost:8080/v1/chat/completions \
    -H "Authorization: Bearer nabu_dev_key_change_me" \
    -d '{ "model": "parspack/openai/gpt-5.5",
          "messages": [{ "role": "user", "content": "سلام" }] }'
  # → provider "parspack", upstream model "openai/gpt-5.5"
  ```

- **Live discovery.** `GET /v1/models` returns the configured aliases **plus**
  every model discovered from each passthrough provider's own `/v1/models`
  (cached ~5 min), as `parspack/…` entries with `owned_by: parspack`. Any
  static `models:` listed under the provider are advertised too, so a provider
  without a usable `/v1/models` still shows up. Discovery failures degrade
  gracefully — the endpoint keeps serving aliases and the last good catalogue.

- **Namespace grants.** A project key allows a whole provider with a wildcard:
  `allow: ["parspack/*"]` covers `parspack/openai/gpt-5.5` and every other
  `parspack/<model>` (including nested IDs). `/v1/models` is filtered to what
  each key may use.

Configured **aliases** still work unchanged and are the recommended default for
apps (they add multi-provider fallback); passthrough is the escape hatch for
reaching a provider's long tail of models without editing config per model.

## Aliases (default config)

| Alias          | Primary → fallbacks                                          |
| -------------- | ----------------------------------------------------------- |
| `nabu-fast`    | Dahl MiniMax → Groq → OpenAI mini → Claude Haiku            |
| `nabu-smart`   | Dahl Kimi → OpenAI 4o → Claude Sonnet → Gemini 1.5 Pro      |
| `nabu-cheap`   | OpenRouter Llama 8B → Groq Llama 8B → Dahl MiniMax          |
| `nabu-vision`  | OpenAI 4o → Gemini 1.5 Pro                                   |
| `nabu-minimax` | Dahl MiniMax-M2.7 → Groq (pin MiniMax explicitly)           |
| `nabu-kimi`    | Dahl Kimi-K2.6 → OpenAI 4o (pin Kimi explicitly)            |
| `nabu-local`   | Ollama (local, on-prem — no fallback so data never leaves)  |
| `nabu-parspack`| Parspack GPT-5.5 → Claude Sonnet 4.6 → Gemini 2.5 Flash     |
| `nabu-image`   | OpenAI gpt-image-1 → Gemini 2.5 Flash Image (image gen)     |
| `nabu-voice`   | OpenAI gpt-4o-mini-tts → Gemini 2.5 Flash TTS (speech)      |
| `nabu-embed`   | OpenAI text-embedding-3-small → Gemini text-embedding-004   |

The `nabu-local` alias routes to a self-hosted **Ollama** server (OpenAI-wire
compatible), so even on-prem/local inference goes through the gateway rather than
projects talking to Ollama directly. Its `ollama` provider is *keyless* — it is
enabled purely by setting `OLLAMA_BASE_URL` (the OpenAI-compatible root incl.
`/v1`, e.g. `http://ollama:11434/v1`) and skipped when that is unset. It has no
cloud fallback on purpose: if Ollama is unreachable the call fails instead of
silently spilling the request to a hosted provider.

Aliases live under `models:` (chat), `images:`, `audio:` and `embeddings:` in
the config. Edit `config.yaml` to add providers, aliases, or change routing — no
code change needed.

`nabu-parspack` is an opt-in route to **Parspack AI Studio**
(`my.parspack.com`), an OpenAI-wire-compatible aggregator fronting 100+ models
behind one key (`PARSPACK_API_KEY`). Point the alias's `model` at any id from
`GET https://my.parspack.com/api/aistudio/api/v1/models` to pin a specific
Parspack model.

## Sub-agents (agents defined from outside, run in one call)

A **sub-agent** is a named assistant = a **system prompt + default parameters**
layered on top of an existing chat alias. Agents are declared **entirely in
config — no code** — either inline under `agents:` or, so specialists can be
authored and dropped in from outside the main config, as one YAML file per agent
in an `agents_dir`. An agent is addressable as a `model`, so any OpenAI client
runs it in a single fast call and it rides the router's normal fallback chain.

```yaml
# config.yaml
agents_dir: "./agents"          # load every *.yaml in this dir as one agent each

agents:                          # …and/or define them inline
  cine-motion-designer:
    model: nabu-smart            # any existing chat alias or "<provider>/<model>"
    system: "You are a Motion Designer for cinematic scroll-driven pages…"
    temperature: 0.7             # optional defaults; an explicit request value wins
```

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer nabu_dev_key_change_me" \
  -d '{ "model": "cine-motion-designer",
        "messages": [{ "role": "user", "content": "Storyboard the hero scene." }] }'
```

NabuGate prepends the agent's system prompt, fills any params the caller left
unset, routes to the agent's `model`, and echoes the agent name back as `model`
(plus an `X-Nabu-Agent` header). Agents also appear on `GET /v1/models`, and a
project key can be granted a group with a glob (`allow: ["cine-*"]`).

The bundled [`agents/`](agents/) directory ships the **Cinematic Scrollytelling
squad** — seven specialists (creative director, interactive designer, motion
designer, 3D artist, front-end developer, content strategist, and a
performance/accessibility engineer) that together design Apple-style,
scroll-driven product pages. See [`agents/README.md`](agents/README.md).

## Policy Engine (per-project keys)

Keys come in two forms. Simple `api_keys` get full access; rich `keys` carry a
per-project policy:

```yaml
server:
  api_keys: ["admin_key"]            # full access, no rate limit
  keys:
    - key: "crm_prod_key"
      project: "crm"
      allow: ["nabu-fast", "nabu-embed"]  # globs ok ("nabu-*"); "*"/empty = all
      rate_limit: 120                      # requests/minute (0 = unlimited)
```

- A request for an alias outside `allow` returns **403**.
- Exceeding `rate_limit` returns **429** (token bucket, per key).
- `GET /v1/models` is filtered to the aliases each key may use.

If both `api_keys` and `keys` are empty the gateway refuses to start (so it is
never accidentally left open); set `NABU_ALLOW_NO_AUTH=1` to run without auth
for local development.

## Cost tracking

Add a price table (USD per 1M tokens, keyed by `provider/model`); the gateway
attributes each call's tokens and cost to the calling project and the upstream
model, and logs a `billed` line per request.

```yaml
pricing:
  "openai/gpt-4o": { input: 2.5, output: 10 }
  "gemini/gemini-1.5-pro": { input: 1.25, output: 5 }
```

Inspect totals at `GET /v1/usage` — admin (full-access) keys see every project
and model; project-scoped keys see only their own totals:

```json
{ "by_project": { "crm": { "requests": 2, "prompt_tokens": 2000,
                           "completion_tokens": 1000, "cost_usd": 0.015 } },
  "by_model":   { "openai/gpt-4o": { "requests": 2, "cost_usd": 0.015, ... } } }
```

Unpriced models are still tracked for token usage (cost 0).

## Run locally

```bash
cp config.example.yaml config.yaml
cp .env.example .env            # fill in the provider keys you have
export $(grep -v '^#' .env | xargs)
go run ./cmd/gateway -config config.yaml
```

Providers whose API-key env var is empty are skipped automatically, so you can
start with just one provider configured. `config.example.yaml` ships a dev
`api_keys` entry; if you empty it, set `NABU_ALLOW_NO_AUTH=1` to run without auth
(the gateway otherwise refuses to start open).

## Deploy with Coolify / Docker

The image bakes a **secret-free default config** ([`config.default.yaml`](config.default.yaml))
at `/app/config.yaml`, so the gateway boots out of the box — you only provide
secrets as environment variables. Minimum to go live:

- `NABU_API_KEY` — the gateway admin key projects must send. **Required:** the
  gateway refuses to start open unless this (or a config `api_keys`) is set, so
  you can't accidentally expose an unauthenticated, money-spending gateway. For
  local dev only, set `NABU_ALLOW_NO_AUTH=1` to run without auth.
- At least one provider key (`DAHL_API_KEY`, `OPENAI_API_KEY`, …). Providers
  whose key is unset are skipped automatically.

```bash
docker build -t nabugate .
docker run -p 8080:8080 \
  -e NABU_API_KEY=nabu_prod_key \
  -e DAHL_API_KEY=dahl-... -e OPENAI_API_KEY=sk-... \
  nabugate
```

In Coolify, deploy this directory as a **Docker Compose** or **Dockerfile**
resource, set `NABU_API_KEY` + your provider keys as environment variables,
assign a domain to port **8080** (Configuration → Domains), and deploy. Coolify
provides TLS and can health-check `/healthz`. Opening the bare domain returns
`404` by design — the gateway only serves `/healthz` and the `/v1/*` endpoints.

**Custom routing (optional).** To change aliases/providers, override the baked
default in one of two ways (either wins over the default):

- **Inline (no mount):** set `NABU_CONFIG_YAML` to the entire config. Ideal on a
  PaaS — no file, so no bind-mount-of-a-missing-file trap (which Docker turns
  into an empty directory and crash-loops the gateway).
- **Mounted file:** create `config.yaml` first, then mount it at
  `/app/config.yaml`. Never mount a *missing* source file — that is the empty
  directory trap.

```bash
docker run -p 8080:8080 \
  -e NABU_API_KEY=nabu_prod_key -e DAHL_API_KEY=dahl-... \
  -e NABU_CONFIG_YAML="$(cat config.yaml)" \
  nabugate
```

`${VAR}` references inside the config are expanded from the environment, so
gateway `api_keys` and other values can come from env.

## Configuration

See [`config.example.yaml`](config.example.yaml). The shape is:

```yaml
server:
  port: 8080
  api_keys: ["nabu_dev_key_change_me"]   # internal keys projects must send
providers:
  groq:
    enabled: true
    type: openai            # openai | anthropic | gemini
    base_url: "https://api.groq.com/openai/v1"
    api_key_env: "GROQ_API_KEY"
models:
  nabu-fast:
    primary:  { provider: groq,   model: "llama-3.1-70b-versatile" }
    fallback:
      - { provider: openai, model: "gpt-4o-mini" }
```

`type: openai` covers any OpenAI-wire-compatible provider (OpenAI, Groq,
OpenRouter, and OpenAI-compatible gateways). Anthropic and Gemini have dedicated
adapters.

## Roadmap (future)

- Streaming for media/embeddings (chat streaming is implemented)
- Persisted usage metrics (current totals are in-memory, reset on restart)
- Prometheus `/metrics` export
