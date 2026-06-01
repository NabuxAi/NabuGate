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
     OpenAI          Groq         Anthropic          Gemini   (OpenRouter…)
```

## Components

| Component          | Responsibility                                             |
| ------------------ | ---------------------------------------------------------- |
| **AI Gateway**     | Single entry point for all projects (`internal/server`)    |
| **Provider Adapter** | Translate the unified request to each vendor's API (`internal/provider`) |
| **Model Registry** | Alias → provider/model table (`models:` in `config.yaml`)  |
| **Router**         | Pick the target for a task/alias (`internal/router`)       |
| **Fallback Engine**| If the primary fails, try the next target (`internal/router`) |
| **Observability**  | Structured JSON logs: latency, tokens, status              |
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
| `POST /v1/chat/completions`  | Chat completion (alias-routed)                      |
| `POST /v1/images/generations`| Image generation; returns `data[].b64_json`         |
| `POST /v1/audio/speech`      | Text-to-speech; returns raw audio bytes (wav/mp3)   |
| `GET  /v1/models`            | List available aliases (chat + image + audio)       |
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

## Aliases (default config)

| Alias         | Primary → fallbacks                                        |
| ------------- | --------------------------------------------------------- |
| `nabu-fast`   | Groq → OpenAI mini → Claude Haiku                          |
| `nabu-smart`  | OpenAI 4o → Claude Sonnet → Gemini 1.5 Pro                 |
| `nabu-cheap`  | OpenRouter Llama 8B → Groq Llama 8B                        |
| `nabu-vision` | OpenAI 4o → Gemini 1.5 Pro                                 |
| `nabu-image`  | OpenAI gpt-image-1 → Gemini 2.5 Flash Image (image gen)    |
| `nabu-voice`  | OpenAI gpt-4o-mini-tts → Gemini 2.5 Flash TTS (speech)     |

Image aliases live under `images:` and speech aliases under `audio:` in the
config. Edit `config.yaml` to add providers, aliases, or change routing — no code
change needed.

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

```bash
docker build -t nabugate .
docker run -p 8080:8080 \
  -e OPENAI_API_KEY=sk-... -e GROQ_API_KEY=gsk-... \
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

## Roadmap (post-MVP)

- Streaming (`stream: true`) passthrough
- `/v1/embeddings` capability
- Per-project policy engine (which aliases each key may use)
- Cost tracking and rate limiting
