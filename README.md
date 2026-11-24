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
| `POST /v1/images/generations` | Images; edits and references with `images` |
| `POST /v1/videos/generations` | Start a video job (text- or image-to-video) |
| `GET /v1/videos/generations/{id}` | Poll a video job |
| `POST /v1/audio/speech` | Text to speech |
| `POST /v1/audio/transcriptions` | Speech to text |
| `POST /v1/systemone` | System 1 structured decisions (Jev / TypeSafe) |
| `POST /v1/decisions` | Structured decisions — dedicated alias, any chat alias, or `provider/model` |
| `POST /v1/decisions/calibrate` | Fit the local engine's calibration temperature on labelled samples |
| `GET /v1/models` | Aliases, agents and passthrough catalogues |
| `GET /v1/agents` | Agents with their model and tool names |
| `GET /v1/usage` | Tokens and cost for the calling key |
| `GET /v1/photos/search` | Stock photos (Pexels) |

Request bodies pass through to the provider untouched — only `model` and the
stream flags are rewritten. So `tools`, `tool_choice`, `response_format`,
`seed`, `top_p`, `stop` and penalties all work, and `tool_calls` come back in
the response, whether or not the gateway names them.

### Image models

`POST /v1/images/generations` takes an alias or an OpenRouter model id
directly. The generating aliases are below; `/v1/models` lists the rest (FLUX,
SDXL, GPT and Gemini image, stock photos, branded graphics). Of these, only
`nabu-image` falls back to other models:

| Alias | Model | Input pictures | Price |
|---|---|---|---|
| `nabu-image` | gpt-image-1, then Gemini, Wan 2.6, Pexels stock | no | per rung |
| `nabu-wan` | Alibaba Wan 2.6 (`wan2.6-t2i`) | no | $0.031 a picture |
| `nabu-image-edit` | Alibaba Wan 2.6 Image (`wan2.6-image`) | yes, up to 4 | $0.031 a picture |
| `nabu-wan25` | Alibaba Wan 2.5 preview (`wan2.5-t2i-preview`) | no | $0.03 a picture |
| `nabu-wan22` | Alibaba Wan 2.2 Plus (`wan2.2-t2i-plus`) | no | $0.05 a picture |
| `nabu-wan-flash` | Alibaba Wan 2.2 Flash (`wan2.2-t2i-flash`) | no | $0.025 a picture |
| `openrouter/<model>` | any OpenRouter image-output model, e.g. `openrouter/google/gemini-3.1-flash-image`, `openrouter/google/gemini-3-pro-image`, `openrouter/google/gemini-2.5-flash-image`, `openrouter/openai/gpt-5-image`, `openrouter/openai/gpt-5-image-mini` | yes | what OpenRouter reports for the call |

`aspect_ratio` (or the shape of `size`) is mapped onto the nearest canvas
each model draws. Wan 2.5 preview draws 1280×1280 to 1440×1440 pixels between
1:4 and 4:1; Wan 2.2 draws sides of 512 to 1440 px (1024×1024 by default).
Those three are on Alibaba's asynchronous API, so the request waits while the
gateway polls the task (usually seconds, at most three minutes). An
`openrouter/…` id is served through OpenRouter's chat completions with
`modalities: ["image","text"]`; the picture comes back as `b64_json` like any
other, and the call is billed at the `usage.cost` OpenRouter returns (or at the
price table, and booked unpriced when the model has none, if it returns no
cost). Prices are Alibaba Model Studio's international list prices.

### Editing and reference images

`POST /v1/images/generations` also edits a picture or draws a new one from
character and style references. Send the input pictures in `images` (or one in
`image`) alongside the prompt, with an editing alias — `nabu-image-edit`
(Alibaba Wan 2.6 Image) or an `openrouter/…` image model (Gemini and GPT
image models edit too):

```json
{
  "model": "nabu-image-edit",
  "prompt": "Make it look like a watercolor painting.",
  "images": ["https://example.com/photo.png"]
}
```

Each input is an `https://` URL or a `data:image/png|jpeg|webp;base64,...` URI;
at most 4 per request and 20 MB decoded in total. Without `aspect_ratio` or
`size` the result keeps the input's shape. The answer is the usual
`data[].b64_json`, billed per output picture like a generation on the same
model. An alias with no model that reads input pictures — `nabu-image`, whose
chain ends in stock photos — answers `400` rather than return a picture
unrelated to the one sent.

### Video generation

Video is asynchronous: starting one returns a job id, and polling the id
returns the video once it is rendered (about 30 s for 5 s of text-to-video,
about a minute from a first frame). Every alias is Alibaba Wan: one model makes
video from the prompt, and another from a first frame when the request carries
`image_url`. No alias falls back to another model.

| Alias | From text | From a photo | Tiers | Lengths | Price a second |
|---|---|---|---|---|---|
| `nabu-video` | `wan2.6-t2v` | `wan2.6-i2v` | 720P, 1080P | 2–15 s | 720P $0.10, 1080P $0.15 |
| `nabu-video-wan25` | `wan2.5-t2v-preview` | `wan2.5-i2v-preview` | 480P, 720P, 1080P | 5 or 10 s | 480P $0.05, 720P $0.10, 1080P $0.15 |
| `nabu-video-wan22` | `wan2.2-t2v-plus` | `wan2.2-i2v-plus` | 480P, 1080P | 5 s | 480P $0.02, 1080P $0.10 |
| `nabu-video-fast` | `wan2.1-t2v-turbo` | `wan2.2-i2v-flash` | 480P, 720P | 5 s | text $0.036; photo 480P $0.015, 720P $0.036 |
| `nabu-video-wan21` | `wan2.1-t2v-plus` | `wan2.1-i2v-plus` | 720P | 5 s | $0.10 |

```http
POST /v1/videos/generations
{"model": "nabu-video", "prompt": "A red fox trotting through fresh snow",
 "aspect_ratio": "16:9", "size": "1280x720", "seconds": 5,
 "image_url": "https://example.com/first-frame.png"}

→ {"id": "video_3f9c…", "object": "video.generation", "status": "processing", "model": "nabu-video", …}

GET /v1/videos/generations/video_3f9c…
→ {"id": "video_3f9c…", "status": "processing", …}
→ {"id": "video_3f9c…", "status": "completed", "data": [{"url": "https://…mp4"}], "seconds": 5, "resolution": "720P", …}
→ {"id": "video_3f9c…", "status": "failed", "message": "…"}
```

`prompt` is required unless `image_url` is given; everything else is optional.
`image_url` (an `https://` URL or a `data:image/png|jpeg|webp;base64,...` URI)
makes it image-to-video, which follows the first frame's shape. Nothing is
refused for asking what a model does not offer; it is mapped to the nearest
thing it does. The tier is `resolution` (`480P`, `720P`, `1080P`), else a tier
written as `size`, else the tier whose canvas is nearest a `WxH` `size`, else
720P — then the nearest tier the model renders, ties going up (720P on
`nabu-video-wan22` renders at 1080P). The text-to-video shape is the nearest of
16:9, 9:16, 1:1, 4:3 and 3:4 to `aspect_ratio` (or the shape of `size`); 480P
has no 4:3 or 3:4. `seconds` is rounded to the nearest length the model takes,
5 s when not given; a model with one length is not sent one. The default is
16:9, 720P, 5 s.

A job id belongs to the key that started it; any other key gets `404`. The
video is billed once, per second of output at the tier it came out at (the
price list above: Alibaba Model Studio's international list prices), on the
first poll that sees it finished; a failed job costs nothing. The URL is
the vendor's signed link and expires after about 24 hours, and so does the job.
Your own vendor key (`X-Nabu-Key-*`) cannot start a video job: finishing it
takes that key on every poll, and the gateway does not keep keys.

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

### Several keys for one provider

Repeat the header. The keys are tried in the order given, so the second one
takes over when the first is refused — a spending cap, a rotation, a project
someone deleted:

```http
X-Nabu-Key-Gemini: AIza...primary
X-Nabu-Key-Gemini: AIza...spare
```

Repeated headers are the list, and only repeated headers: a comma inside a
value is part of the key, never a separator. Splitting on one would quietly
turn a single valid credential into two invalid ones.

Each key is a rung of its own in the fallback chain and is named separately in
any error — `gemini (your key #2)`, not a second `gemini` — because three
refusals of three credentials is a different problem from one provider being
down three times.

Signed-in console users can also save keys per provider (sealed with
`NABUGATE_SECRET_KEY`; without it the gateway refuses to store rather than
keeping plaintext). Saved keys are tried **after** any sent on the request, so
an explicit header still wins and a saved key stays useful as the fallback.

## Subscriptions

A deployment can sell the right to spend *its own* vendor keys. Declare the
offers under `plans:` (see `config.default.yaml`), and a console user buys a
term out of the balance they already topped up.

Two charges, deliberately separate:

- the **plan fee**, once per term, which buys access;
- the **calls**, metered against the same balance at the plan's `markup`,
  never below `min_charge_usd` per call.

Someone running on their own key pays neither — their vendor billed them, and
the gateway records that usage at zero cost. A plan only matters for providers
marked `access: request`; everything else stays reachable as it always was, and
a token baked into the config has no owner and is never gated.

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
| `NABUAUTH_MIRROR_URL` | NabuAuth origin the browser is sent to when it came through `gate.nabuxai.ir`; defaults to `https://auth.nabuxai.ir` |
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
