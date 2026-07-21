# Cloudflare Workers AI & TokenRouter

> **خلاصه (Persian).** دو ارائه‌دهندهٔ جدید و **سازگار با OpenAI** به NabuGate اضافه
> شده‌اند: **Cloudflare Workers AI** (استنتاجِ سِرورلِس روی لبهٔ کلادفلر) و
> **TokenRouter** (روترِ +۳۰۰ مدل با مسیریابیِ هوشمند). هر دو فقط با ویرایشِ
> `config.yaml` و تنظیمِ چند متغیرِ محیطی کار می‌کنند — **هیچ کدِ جدیدی لازم نیست**،
> چون هر دو wire-compatible با OpenAI هستند و از همان آداپتورِ `openai` استفاده
> می‌کنند. هر دو به‌صورتِ `passthrough` علامت خورده‌اند، پس می‌توانید هر مدلی را
> مستقیم با `cloudflare/<model>` یا `tokenrouter/<model>` صدا بزنید — مثلاً
> `tokenrouter/glm-5.2`.

Both providers are ordinary OpenAI-wire upstreams, so they plug into the existing
`type: openai` adapter with **no new code**. Each is marked `passthrough: true`,
which turns the provider name into a first-class namespace (`cloudflare/…`,
`tokenrouter/…`) and, where the upstream supports it, surfaces its catalogue on
the gateway's own `GET /v1/models`.

| Provider | Base URL | Auth | Key env | Passthrough | `/v1/models` discovery |
| --- | --- | --- | --- | --- | --- |
| Cloudflare Workers AI | `https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1` | `Bearer` | `CLOUDFLARE_API_KEY` (+ `CLOUDFLARE_ACCOUNT_ID`) | yes | no endpoint → served from the static `models:` list |
| TokenRouter | `https://api.tokenrouter.io/v1` | `Bearer` | `TOKENROUTER_API_KEY` | yes | yes (live) |

Like every keyed provider, each is **skipped automatically** when its key is
unset, so the gateway still boots with whatever subset you have configured.

---

## Cloudflare Workers AI

[Workers AI](https://developers.cloudflare.com/workers-ai/) runs open models
(Meta Llama, OpenAI `gpt-oss`, Qwen, Mistral, DeepSeek, BAAI `bge` embeddings, …)
serverlessly on Cloudflare's edge. It exposes an
[OpenAI-compatible surface](https://developers.cloudflare.com/workers-ai/configuration/open-ai-compatibility/)
for `/chat/completions`, `/embeddings` and `/responses`.

### 1. Get credentials

You need two values, both from the Cloudflare dashboard:

- **Account ID** — on the right-hand sidebar of your account/Workers AI page. It
  becomes part of the URL, so NabuGate templates it into `base_url` via
  `${CLOUDFLARE_ACCOUNT_ID}`.
- **API token** — create a token with the **Workers AI** permission
  (Account → API Tokens). It is sent as `Authorization: Bearer <token>`.

```bash
# .env
CLOUDFLARE_ACCOUNT_ID="xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
CLOUDFLARE_API_KEY="cf-token-with-workers-ai-permission"
```

Both are required. If `CLOUDFLARE_API_KEY` is empty the provider is skipped; if
you set the key but forget `CLOUDFLARE_ACCOUNT_ID`, requests hit a malformed URL
and Cloudflare returns 404 — set both together.

### 2. Config (already in `config.example.yaml` / `config.default.yaml`)

```yaml
providers:
  cloudflare:
    enabled: true
    type: openai
    base_url: "https://api.cloudflare.com/client/v4/accounts/${CLOUDFLARE_ACCOUNT_ID}/ai/v1"
    api_key_env: "CLOUDFLARE_API_KEY"
    passthrough: true
    models:                                    # advertised on /v1/models
      - "@cf/meta/llama-3.3-70b-instruct-fp8-fast"
      - "@cf/meta/llama-3.1-8b-instruct"
      - "@cf/openai/gpt-oss-120b"
      - "@cf/openai/gpt-oss-20b"
      - "@cf/baai/bge-large-en-v1.5"
      - "@cf/baai/bge-m3"

models:
  nabu-cloudflare:
    primary:  { provider: cloudflare, model: "@cf/meta/llama-3.3-70b-instruct-fp8-fast" }
    fallback:
      - { provider: cloudflare, model: "@cf/meta/llama-3.1-8b-instruct" }
      - { provider: cloudflare, model: "@cf/openai/gpt-oss-120b" }
```

Workers AI has **no OpenAI-style `/v1/models` endpoint**, so live discovery is a
no-op for it — the static `models:` list above is exactly what the gateway
advertises. Add or remove IDs there to curate what `/v1/models` shows.

### 3. Model IDs

Workers AI model IDs use the `@cf/<author>/<model>` form. The catalogue changes
frequently (Cloudflare adds models weekly), so browse the live list at
[developers.cloudflare.com/workers-ai/models](https://developers.cloudflare.com/workers-ai/models/).
A few current, widely-used IDs:

| Purpose | Model ID |
| --- | --- |
| Fast flagship chat | `@cf/meta/llama-3.3-70b-instruct-fp8-fast` |
| Small/cheap chat | `@cf/meta/llama-3.1-8b-instruct` |
| OpenAI open-weight | `@cf/openai/gpt-oss-120b`, `@cf/openai/gpt-oss-20b` |
| Embeddings (EN) | `@cf/baai/bge-large-en-v1.5` |
| Embeddings (multilingual) | `@cf/baai/bge-m3` |

### 4. Call it

Via the alias (with fallback):

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer nabu_dev_key_change_me" \
  -d '{ "model": "nabu-cloudflare",
        "messages": [{ "role": "user", "content": "سلام، خلاصه کن" }] }'
```

Via **passthrough** — any `@cf/…` model, no alias needed. The router splits on
the **first** `/`, so the `@cf/author/model` ID keeps its own slashes:

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer nabu_dev_key_change_me" \
  -d '{ "model": "cloudflare/@cf/openai/gpt-oss-120b",
        "messages": [{ "role": "user", "content": "Explain edge inference" }] }'
# → provider "cloudflare", upstream model "@cf/openai/gpt-oss-120b"
```

Embeddings work too. Passthrough direct-routing only applies to chat/responses,
so reach Workers AI embeddings through an **embeddings alias** — `nabu-embed`
already carries a Cloudflare `bge` fallback:

```bash
curl -X POST http://localhost:8080/v1/embeddings \
  -H "Authorization: Bearer nabu_dev_key_change_me" \
  -d '{ "model": "nabu-embed", "input": ["متن اول", "second text"] }'
```

> **Pricing.** Workers AI bills in "neurons" and varies per model; NabuGate does
> not ship a Cloudflare price table, so these calls are tracked for tokens at
> cost 0 until you add entries under `pricing:` keyed by
> `cloudflare/@cf/<author>/<model>`.

---

## TokenRouter

[TokenRouter](https://www.tokenrouter.com/) ([docs](https://docs.tokenrouter.io/))
is an OpenAI-wire aggregator/router fronting **300+ verified models** (OpenAI,
Claude, Gemini, Llama, Mistral, DeepSeek, GLM, …) behind one key, with smart
`auto:*` routing modes. It is a drop-in OpenAI endpoint at
`https://api.tokenrouter.io/v1`.

### 1. Get an API key

Create a key in the [TokenRouter console](https://app.tokenrouter.io) and set it:

```bash
# .env
TOKENROUTER_API_KEY="tr-..."
```

### 2. Config (already shipped)

```yaml
providers:
  tokenrouter:
    enabled: true
    type: openai
    base_url: "https://api.tokenrouter.io/v1"
    api_key_env: "TOKENROUTER_API_KEY"
    passthrough: true

models:
  nabu-tokenrouter:
    primary:  { provider: tokenrouter, model: "auto:balance" }
    fallback:
      - { provider: tokenrouter, model: "gpt-4o" }
      - { provider: tokenrouter, model: "anthropic:claude-3-5-sonnet-20241022" }
```

TokenRouter is built on an OpenAI-compatible catalogue API, so its `/v1/models`
**feeds live discovery** — every model it exposes shows up on the gateway's
`/v1/models` as `tokenrouter/<id>` (cached ~5 min). No static `models:` list is
needed.

### 3. Addressing models

TokenRouter uses a colon (`:`) inside model IDs — not a slash — so the gateway's
first-`/` passthrough split stays clean. Ways to address a model:

| Form | Example (passthrough) | Meaning |
| --- | --- | --- |
| Plain OpenAI ID | `tokenrouter/gpt-4o` | route `gpt-4o` |
| Provider-pinned | `tokenrouter/anthropic:claude-3-5-sonnet-20241022` | force Anthropic |
| Other vendors (e.g. GLM) | `tokenrouter/glm-5.2` | route GLM 5.2 |
| Auto-routing | `tokenrouter/auto:balance` | let TokenRouter pick |

Auto-routing modes: `auto:fast` (speed), `auto:balance` (speed/cost/quality),
`auto:cost` (cheapest), `auto:quality` (best). Provider prefixes: `openai:`,
`anthropic:`, `gemini:`, `mistral:`, `deepseek:` (and more — see the models page).

### 4. Call it

Via the alias (primary uses TokenRouter's own smart routing):

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer nabu_dev_key_change_me" \
  -d '{ "model": "nabu-tokenrouter",
        "messages": [{ "role": "user", "content": "Compare GLM and Llama briefly" }] }'
```

Via passthrough — reach **GLM-5.2** (the model from
[the models page](https://www.tokenrouter.com/models?search=glm-5.2)) directly,
no alias:

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer nabu_dev_key_change_me" \
  -d '{ "model": "tokenrouter/glm-5.2",
        "messages": [{ "role": "user", "content": "سلام" }] }'
# → provider "tokenrouter", upstream model "glm-5.2"
```

> Confirm the exact catalogue ID (e.g. `glm-5.2` vs. a `zhipu:`-prefixed form) on
> the [models page](https://www.tokenrouter.com/models); pass it through verbatim.

---

## Passthrough & key allow-lists

With both providers marked `passthrough: true`, a project key grants a whole
namespace with a wildcard. The shipped example key already includes them:

```yaml
server:
  keys:
    - key: "crm_prod_key_change_me"
      project: "crm"
      allow: ["cloudflare/*", "tokenrouter/*", "nabu-cloudflare", "nabu-tokenrouter", ...]
```

- `allow: ["cloudflare/*"]` covers every `cloudflare/@cf/<author>/<model>`.
- `allow: ["tokenrouter/*"]` covers every `tokenrouter/<id>` (including the
  colon-prefixed and `auto:*` forms).
- `GET /v1/models` is filtered to what each key may use.

## Verify

```bash
# List everything this key can reach (aliases + discovered passthrough models).
curl http://localhost:8080/v1/models \
  -H "Authorization: Bearer nabu_dev_key_change_me"

# The response header X-Nabu-Provider / X-Nabu-Model shows what actually served
# the request; the JSON body echoes provider + upstream_model.
```

## Troubleshooting

- **Provider missing from `/v1/models`** — its key env is unset, so it was
  skipped at startup. Check the boot logs for a `provider "…" disabled` warning.
- **Cloudflare 404 / "not found"** — `CLOUDFLARE_ACCOUNT_ID` is wrong or unset
  (the account id is embedded in the URL), or the API token lacks the Workers AI
  permission.
- **Cloudflare models don't appear via discovery** — expected: Workers AI has no
  OpenAI `/v1/models`. Curate them in the provider's static `models:` list.
- **TokenRouter model errors** — verify the exact ID on the models page; some
  features (JSON mode, `seed`, `logit_bias`) are provider-specific, so prefer
  `auto:*` or an explicit provider prefix when using them.

## Sources

- [Cloudflare Workers AI — OpenAI compatible API endpoints](https://developers.cloudflare.com/workers-ai/configuration/open-ai-compatibility/)
- [Cloudflare Workers AI — Models](https://developers.cloudflare.com/workers-ai/models/)
- [TokenRouter — OpenAI Compatibility](https://docs.tokenrouter.io/getting-started/openai-compatibility)
- [TokenRouter — Models catalogue](https://www.tokenrouter.com/models)
