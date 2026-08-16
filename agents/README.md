# NabuGate sub-agents — `agents/`

Each `*.yaml` here is one **sub-agent**: a named assistant that layers a system
prompt and default sampling parameters on top of an existing NabuGate chat
alias. Agents are defined **from outside the binary** — no code — and NabuGate
loads every file in this directory when `agents_dir` points at it:

```yaml
# config.yaml
agents_dir: "./agents"
```

## Invoke one

An agent is addressable as a `model`, so any OpenAI-compatible client runs it in
a single call:

```bash
curl $NABU/v1/chat/completions \
  -H "Authorization: Bearer $NABU_KEY" \
  -d '{
    "model": "cine-motion-designer",
    "messages": [{"role":"user","content":"Storyboard the hero scene for a coffee brand."}]
  }'
```

NabuGate prepends the agent's `system` prompt, fills any params the caller left
unset, routes to the agent's `model` (with the usual fallback chain), and echoes
the agent name back as `model` (plus an `X-Nabu-Agent` response header). Agents
also show up on `GET /v1/models`.

## Define one

```yaml
name: my-agent          # optional; defaults to the file name
description: "..."       # shown on /v1/models
model: nabu-smart        # an existing chat alias or "<provider>/<model>"
system: |
  Your instructions here.
temperature: 0.7         # optional defaults; a caller value always wins
top_p: 1.0
max_tokens: 2048
```

Drop the file in, restart the gateway, and the agent is live. `${VAR}` env
references are expanded, just like the main config.

## Give an agent tools

An agent can also carry **tools**: functions the model may call that **the
gateway executes server-side** (inspired by NabuChat's HTTP tool). The caller
sends one ordinary chat request and gets one ordinary answer — the tool
traffic never leaves NabuGate:

```
caller ──▶ NabuGate ──▶ model: "call track_order(order_id=42)"
              │                      ▲
              ├──▶ GET shop API ◀────┘ result appended as role:tool
              │                      ▲
              └──▶ model: "سفارش شما ارسال شد." ──▶ caller
```

Declare them in the agent file — this is the whole "easy add" story, no code:

```yaml
name: accountcity-support
model: nabu-smart
system: |
  تو دستیار پشتیبانی فروشگاه هستی. برای وضعیت سفارش از ابزار track_order استفاده کن.
max_tool_steps: 4        # optional, default 4, max 8
tools:
  - name: track_order                 # the function name the model calls
    type: http                        # the only executor so far
    description: وضعیت سفارش مشتری را از فروشگاه می‌گیرد
    method: GET
    url: "https://shop.example.com/orders/{order_id}"
    headers:
      Authorization: "Basic ${SHOP_BASIC}"   # env, like the rest of the config
    path_params: [order_id]           # substituted into {order_id} in the url
    parameters:                        # JSON schema = what the model sees
      type: object
      properties:
        order_id: {type: string, description: "شماره سفارش"}
      required: [order_id]
    timeout_ms: 8000                   # optional, default 8000, max 15000
    max_response_bytes: 8192           # optional, truncate the tool result
```

How a call is built from the model's arguments:

- `path_params` are URL-escaped into `{placeholder}`s in the url.
- For body methods (POST/PUT/PATCH), `body_template` is sent as JSON with
  `{{arg}}` placeholders filled; a value that is exactly `"{{arg}}"` keeps the
  argument's JSON type. Header values also accept `{{arg}}`.
- Any argument not consumed by the path or the body is appended as a query
  parameter — so a plain GET tool needs nothing but `parameters`.
- `${VAR}` in url/headers expands from the gateway environment when the agent
  file loads (identical to the main config — set the vars before start), and
  once more at call time.

A complete, working example lives in `accountcity-support.yaml`.

### The rules of the loop

- **Client tools win.** If the caller sends its own `tools`, the gateway
  injects nothing and runs no loop: the request passes through untouched,
  exactly as before. Agent tools apply only to requests that bring none.
- **Bounded.** The loop runs at most `max_tool_steps` rounds (default 4, hard
  cap 8). A model still asking for tools after that gets one final call with
  the tools removed, so it answers with what it gathered. Usage is summed over
  every round-trip and billed once. `X-Nabu-Tool-Calls` counts executions.
- **Tool failures are answers, not errors.** A timeout, an HTTP 500, an
  unknown function name — each goes back to the model as the tool result so it
  can retry or apologise. Only provider failures fail the request.
- **Streaming is stream-shaped.** Tool calling needs the full exchange before
  the answer exists, so `stream: true` runs the loop non-streamed and returns
  the finished answer as one SSE delta. SSE clients keep working; the answer
  simply arrives all at once.
- **OpenAI-wire providers only.** The loop needs the OpenAI `tools` /
  `tool_calls` contract end to end, which the Anthropic and Gemini adapters do
  not translate. An agent with tools whose `model` routes to such a provider
  fails fast with a 400 explaining the limitation. `/v1/responses` is not
  tool-looped either — it keeps its raw proxy behaviour. Inside a `flows/`
  chain, a step naming a tool-bearing agent runs as a plain prompt agent: the
  chain drives the conversation, so its tools are simply not offered.

### Safety rails (the executor assumes a hostile model)

- Only `http`/`https`; redirects capped at 3, every hop re-validated.
- **SSRF guard**: loopback, RFC1918, link-local and other non-public targets
  are refused at dial time. For a tool that legitimately points inside your
  own network, set `NABUGATE_TOOL_SSRF_ALLOW=1` on the gateway — deliberately,
  and only with agent YAML you trust.
- The caller's `Authorization` header is never forwarded to a tool endpoint;
  a tool call carries only the headers its YAML declares.
- Every call is time-boxed (`timeout_ms`, max 15 s) and its result truncated
  (`max_response_bytes`, max 64 KiB).
- A broken declaration (bad schema, unknown type, duplicate name) skips the
  agent with a startup warning — it never fails a request halfway through.

### Discovering tools

`GET /v1/agents` (same auth as everything else) lists agents with their model
and tool names, scoped to the calling key's allow-list:

```json
{ "object": "list", "data": [
  { "name": "accountcity-support", "model": "nabu-smart",
    "tools": ["track_order", "follow_up_order"], "max_tool_steps": 4 }
]}
```

## The Cinematic Scrollytelling squad

Seven specialists that together produce Apple-style, scroll-driven product
pages — the storyboard, the motion, the code, the words, and the polish:

| Agent | Role |
|-------|------|
| `cine-creative-director`   | Scroll storyboard, visual rhythm, art direction |
| `cine-interactive-designer`| Scroll/pointer → scene timeline: pins, scrubbing, thresholds |
| `cine-motion-designer`     | Transitions, easing, timing, camera moves (motion = meaning) |
| `cine-3d-artist`           | Product model, lighting, materials, rendered frame sequence |
| `cine-frontend-developer`  | Fast responsive build: GSAP/ScrollTrigger, Canvas, WebGL, scrubbing |
| `cine-content-strategist`  | Per-scene copy, feature order, the sales narrative |
| `cine-performance-a11y`    | Smooth on weak phones + accessible, real reduced-motion path |

Grant a project key the whole squad with a glob: `allow: ["cine-*"]`.

## Product agents

| Agent | Role |
|-------|------|
| `nabusu-night-companion` | Sits with someone who cannot sleep at 3am — short, level, never promises sleep |

`nabusu-night-companion` backs the chat in
[NabuSu](https://github.com/NabuxAi/NabuSu), a night companion for people who
cannot sleep and for people tapering off benzodiazepines under medical
supervision.

Its `system` block is not tone guidance. It carries hard limits — never a dose,
a schedule or a rate of reduction, and an immediate hand-off to emergency care
when someone reports a seizure or hallucinations. The app repeats those limits
on its own side, and drops any reply containing dosing language. Treat edits to
that file as changes to a safety contract, not copywriting.

## SEO & Content Quality squad

Three specialists and a flow (`seo-audit-team`) that audit On-Page SEO, synthesize Schema.org JSON-LD (FAQPage, VideoObject, Article), and compute 0-100 quality scores:

| Agent | Role |
|-------|------|
| `seo-content-auditor`     | On-Page audit, headings structure, keyword density/LSI, CWV checks |
| `seo-schema-engineer`     | JSON-LD structured data generator (FAQPage, VideoObject, Article) |
| `seo-strategist-reviewer` | 100-point scoring, internal link graph strategy, Markdown report builder |

Grant a project key the whole squad with a glob: `allow: ["seo-*"]`.

## Squads in this directory

| prefix | purpose | source of truth |
|---|---|---|
| `cine-*` | cinematic scrollytelling: creative direction, motion, 3D, frontend, content, perf/a11y | this repo |
| `nabusu-*` | NabuSu companion | this repo |
| `write-*` | NabuWrite: composer, inline editor, edit classifier | **`NabuxAi/NabuWrite`** |
| `seo-*` | SEO & schema engineering, content audit, quality scoring | this repo |

### Agents owned by another repo

`write-*` is authored in NabuWrite and copied here. It has to live in both
places: the image bakes this directory at build time (`COPY agents /app/agents`),
so an agent that exists only in the consuming project never reaches the gateway
— and the consuming project needs it in its own tree to be a complete product.

Each such file carries a header naming its origin. Edit it there, copy across,
and expect a change made only here to be overwritten by the next sync.

The same applies to any future project: bring its agents in, and give its
project key a matching glob under `server.keys`, e.g.

```yaml
keys:
  - key: "${MYPROJECT_KEY}"
    project: "myproject"
    allow: ["myproject-*"]
    rate_limit: 120
```

An entry whose key env is empty is skipped by the policy builder, so it stays
inert until the variable is set.
