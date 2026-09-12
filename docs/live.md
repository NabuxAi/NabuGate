# Realtime voice (GPT-Live) through the gateway

GPT-Live is a full-duplex voice model: the browser streams microphone audio to
the vendor over WebRTC and hears the reply on the same connection, while a
backend model does the thinking. The gateway does not carry that audio. Its
two jobs are the signalling handshake and the bill.

## Signalling — `POST /v1/live/sessions`

The body is the vendor's session-creation body with the model given as a
gateway alias, either at the top level (like every other endpoint) or inside
`session.model` (the vendor's own shape, so an SDK-built body works unchanged):

```json
{
  "model": "nabu-live",
  "session": {
    "instructions": "You are the voice assistant of Zoey Realestate…",
    "audio": { "output": { "voice": "quartz" } },
    "delegation": {
      "type": "responses",
      "responses": { "model": "gpt-5.6-terra", "instructions": "…", "tools": [] }
    }
  },
  "transport": { "type": "webrtc", "sdp": "<the browser's SDP offer>" }
}
```

The gateway replaces the alias with the upstream model and forwards the rest
untouched; whatever the vendor answers comes back verbatim with a `201`, plus
the usual `X-Nabu-Provider` / `X-Nabu-Model` headers:

```json
{ "session": { "id": "live_123" }, "transport": { "type": "webrtc", "sdp": "<answer>" } }
```

Apply the answer to the browser's `RTCPeerConnection` and the call is live.
The key stays on your server: the browser posts its offer to you, you post it
here. A `live:` alias is declared like an `audio:` one:

```yaml
live:
  nabu-live:
    primary: { provider: openai, model: "gpt-live-1" }
pricing:
  "openai/gpt-live-1": { per_minute: 0.05 }
```

**Every live alias needs a `per_minute` price** for each provider/model it can
land on. That price is the only thing a live minute is billed from, so an
unpriced alias would connect and bill nothing. The gateway checks at start-up:
an unpriced alias is logged (`live alias refused`), answers every session
request with `503 live alias misconfigured: …`, is left out of `/v1/models`,
and shows `"disabled": true` in `/v1/health` — while everything else keeps
serving. `NABU_CONFIG_YAML` replaces the baked config wholesale, so an inline
config needs its own `live:` block and price.

**Creating a session is one attempt, never retried.** A create whose answer was
lost may already be running on the vendor's clock; replaying the offer would
open a second session that nothing ever closes. Retry from the browser, with a
fresh offer. A vendor refusal keeps its class: `400` for a body or offer the
vendor rejected, `429` for the vendor's rate limit, `502` for anything only the
gateway's operator can fix (a rejected key, a vendor 5xx). The vendor's own
message comes back on one line, with anything credential-shaped masked.

## Billing — `POST /v1/live/sessions/{id}/usage`

Only the vendor knows exactly how long a call ran, and it tells the *browser*:
`session.closed` carries a `usage` object (the SDK also reads a
`session.usage.updated` event if the vendor sends one). The vendor's docs show
that object without naming its duration field, so `packages/live-web` reads the
plausible spellings — `seconds`, `duration_seconds`, … and millisecond
variants — and ignores a shape it does not recognise rather than throwing.

**Billing does not depend on that event.** A product can report seconds from its
own clock instead — NabuCRM does, every 15 seconds and once more on hang-up,
capped at the call's allowance. Either way your server relays the duration
here; the gateway charges the key's owner the seconds it has not yet billed at
the model's `per_minute` price (times the plan rate, like any other metered
call):

```json
{ "seconds": 90, "final": true }
```

```json
{ "session_id": "live_123", "seconds": 90, "billed_seconds": 90, "cost_usd": 0.075, "final": true }
```

`seconds` is the cumulative duration, a snapshot rather than an increment, so a
repeated or out-of-order report never bills twice. `final` closes the
session; a session nobody finalises is forgotten after six hours. Only the
key that created a session may report on it — anything else is `404`.

The registry of open sessions lives in `live-sessions.json` on the state volume
(`NABU_STATE_DIR`, `/data` in the image), rewritten on every change, so a
redeploy in the middle of a call does not turn its remaining reports into
`404 unknown live session` — minutes that would otherwise never be billed. A
session opened on the caller's own vendor key (`X-Nabu-Key-openai`) stays
unbilled for its whole life, not only on the request that created it.

The delegated backend model's tokens are billed by the vendor to the
gateway's own account and are not metered per session here; price the
`per_minute` rate with that in mind.

## Seeing whether it works

`GET /v1/health` lists live aliases with the rest (`"kind": "live"`): how many
rungs have a key in this deployment, `"disabled": true` with the reason when
the alias is refused, and — since health never contacts a vendor — the last
session request's outcome (`last_ok_at`, `last_error`, `last_error_at`).
`GET /v1/models` lists a live alias to every key allowed to use it.

A real signalling check, never part of a plain `go test ./...`:

```bash
NABUGATE_LIVE_SMOKE=1 OPENAI_API_KEY=sk-… go test ./internal/provider -run TestLiveSmoke -v
```

It opens one real session (client delegation, so no backend model runs) with a
synthetic SDP offer and checks the vendor answers with a session id and an SDP
answer. The session never connects media and ends when ICE gives up, so expect
a few seconds of billing at most. If the vendor rejects the synthetic offer,
capture a real one in a browser (`pc.localDescription.sdp` after ICE
gathering), save it, and pass `NABUGATE_LIVE_SMOKE_OFFER=/path/to/offer.sdp`.
The only complete test is a person on a microphone.

## Actions and the browser SDK

A call that can *do* things — search, create, book — needs the actions
run inside the product, as the signed-in user. `packages/live-web` is the
browser side (WebRTC, events, function-call relay, usage); the product
adds three endpoints. See `packages/live-web/README.md`, and
`docs/live-costs.md` for what a minute costs per engine.
