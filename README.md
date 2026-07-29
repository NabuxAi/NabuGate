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
| `GET /v1/models` | Aliases, agents and passthrough catalogues |
| `GET /v1/usage` | Tokens and cost for the calling key |
| `GET /v1/photos/search` | Stock photos (Pexels) |

Request bodies pass through to the provider untouched — only `model` and the
stream flags are rewritten. So `tools`, `tool_choice`, `response_format`,
`seed`, `top_p`, `stop` and penalties all work, and `tool_calls` come back in
the response, whether or not the gateway names them.

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
