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
