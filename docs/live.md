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

## Billing — `POST /v1/live/sessions/{id}/usage`

Only the vendor knows how long a call ran, and it tells the *browser*, in
`session.usage.updated` and `session.closed` events on the data channel. Your
server relays that duration here; the gateway charges the key's owner the
seconds it has not yet billed at the model's `per_minute` price (times the
plan rate, like any other metered call):

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

The delegated backend model's tokens are billed by the vendor to the
gateway's own account and are not metered per session here; price the
`per_minute` rate with that in mind.
