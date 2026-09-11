# What a minute of live voice costs

Vendor list prices as of 2026-09-11, USD. "Per minute" is a minute of a
two-way call, whoever is talking. Assumptions for the backend/text parts:
a caller speaks about 150 words a minute, the assistant about the same,
the backend is consulted ~1.5 times a minute with ~3k tokens of context in
and ~300 out (grows with the conversation).

## Engines

| Engine | Voice part | Thinking part | ≈ total / min | Notes |
|---|---|---|---|---|
| **GPT-Live** (`gpt-live-1`) | $0.05 flat, billed per second | delegated model, see below | **$0.051–0.07** | full-duplex; backend model chosen separately |
| ↳ backend `gpt-5.6-luna` ($0.20 in / $1.20 out) | | ≈ $0.001 | $0.051 | cheapest sensible backend |
| ↳ backend `gpt-5-mini` ($0.25 / $2.00) | | ≈ $0.002 | $0.052 | |
| ↳ backend `gpt-5.6-terra` ($2 / $12) | | ≈ $0.010 | $0.060 | strong reasoning, tools |
| ↳ backend `gpt-5.6-sol` ($4 / $20) | | ≈ $0.019 | $0.069 | |
| **ElevenLabs Agents** (ConvAI) | $0.08 pay-as-you-go ($0.16 burst); plan minutes cheaper | LLM billed on top (their rate) | **$0.08–0.12** | cloned voices, best TTS quality |
| **OpenAI Realtime** (`gpt-realtime`) | $32 in / $64 out per 1M audio tokens (≈600 tokens/min each way) | built in | **≈ $0.25–0.35** | one model does everything; context re-billed each turn |
| **Realtime mini** (`gpt-realtime-mini`) | $10 / $20 per 1M audio tokens | built in | **≈ $0.08–0.12** | |
| **Chained** (STT → text LLM → TTS) | `gpt-4o-transcribe` $0.006 + `gpt-4o-mini-tts` ≈ $0.012 (≈1k audio tokens/min spoken) | `gpt-5.6-luna` ≈ $0.001 | **≈ $0.02–0.03** | not full-duplex, 1–2 s turn latency |

ElevenLabs plan minutes: Starter $6 → 75 min ($0.08), Creator $22 → 275
min ($0.08), Pro $99 → 1,238 min ($0.08), Scale $299 → 3,738 min ($0.08),
Business $990 → 12,375 min ($0.08). Their TTS alone is $0.05–0.10 per 1k
characters.

## What that means per conversation

A typical qualification call is 3–5 minutes:

| Engine | 4-minute call | 1,000 calls / month |
|---|---|---|
| GPT-Live + luna | $0.21 | $205 |
| GPT-Live + terra | $0.24 | $240 |
| ElevenLabs Agents | $0.32–0.48 | $320–480 |
| gpt-realtime | $1.00–1.40 | $1,000–1,400 |
| gpt-realtime-mini | $0.32–0.48 | $320–480 |
| Chained | $0.08–0.12 | $80–120 |

## What we charge

The gateway meters GPT-Live at `pricing."openai/gpt-live-1".per_minute`
(0.05 by default) times the key owner's plan rate. Zooey sells prepaid
voice credits at 12 ¢/min (GPT-Live) and 15 ¢/min (ElevenLabs) —
`VOICE_RATE_CENTS_PER_MIN_*` — which is ~2× the raw cost on GPT-Live
with a cheap backend and covers the backend tokens, which the gateway
does not meter per session. If a product runs a dear backend (sol) or long
calls, raise its rate; the delegated tokens land on the gateway's OpenAI
bill either way.

## Not in these numbers

- Telephony (Twilio/Telnyx minutes) if the call comes from a phone number.
- STUN/TURN for callers behind strict NATs (a TURN relay costs bandwidth).
- Storage of recordings, if kept.
