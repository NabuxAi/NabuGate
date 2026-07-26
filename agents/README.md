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

## Squads in this directory

| prefix | purpose | source of truth |
|---|---|---|
| `cine-*` | cinematic scrollytelling: creative direction, motion, 3D, frontend, content, perf/a11y | this repo |
| `nabusu-*` | NabuSu companion | this repo |
| `write-*` | NabuWrite: composer, inline editor, edit classifier | **`NabuxAi/NabuWrite`** |

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
