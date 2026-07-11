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
     Dahl           OpenAI          Groq / Anthropic / Gemini   (OpenRouter…)
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
| `POST /v1/chat/completions`  | Chat completion (alias-routed); supports `stream: true` |
| `POST /v1/images/generations`| Image generation; returns `data[].b64_json`         |
| `POST /v1/audio/speech`      | Text-to-speech; returns raw audio bytes (wav/mp3)   |
| `POST /v1/embeddings`        | Text embeddings; `input` may be a string or array   |
| `GET  /v1/models`            | List available aliases (chat/image/audio/embeddings)|
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

## Aliases (default config)

| Alias          | Primary → fallbacks                                          |
| -------------- | ----------------------------------------------------------- |
| `nabu-fast`    | Dahl MiniMax → Groq → OpenAI mini → Claude Haiku            |
| `nabu-smart`   | Dahl Kimi → OpenAI 4o → Claude Sonnet → Gemini 1.5 Pro      |
| `nabu-cheap`   | OpenRouter Llama 8B → Groq Llama 8B → Dahl MiniMax          |
| `nabu-vision`  | OpenAI 4o → Gemini 1.5 Pro                                   |
| `nabu-minimax` | Dahl MiniMax-M2.7 → Groq (pin MiniMax explicitly)           |
| `nabu-kimi`    | Dahl Kimi-K2.6 → OpenAI 4o (pin Kimi explicitly)            |
| `nabu-image`   | OpenAI gpt-image-1 → Gemini 2.5 Flash Image (image gen)     |
| `nabu-voice`   | OpenAI gpt-4o-mini-tts → Gemini 2.5 Flash TTS (speech)      |
| `nabu-embed`   | OpenAI text-embedding-3-small → Gemini text-embedding-004   |

Aliases live under `models:` (chat), `images:`, `audio:` and `embeddings:` in
the config. Edit `config.yaml` to add providers, aliases, or change routing — no
code change needed.

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

If both `api_keys` and `keys` are empty, auth is disabled (dev mode).

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
start with just one provider configured. If `server.api_keys` is empty, auth is
disabled (dev mode).

## Deploy with Coolify / Docker

No config is baked into the image (that would publish the example API key as a
live credential), so you must mount your own `config.yaml` — the gateway exits
if it is missing.

```bash
cp config.example.yaml config.yaml   # then set real api_keys
docker build -t nabugate .
docker run -p 8080:8080 \
  -e DAHL_API_KEY=dahl-... -e OPENAI_API_KEY=sk-... -e GROQ_API_KEY=gsk-... \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  nabugate
```

In Coolify, deploy this directory as a **Docker Compose** or **Dockerfile**
resource, set the provider keys as environment variables, mount your
`config.yaml`, and expose port **8080** (Coolify provides TLS + the `/healthz`
check).

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
