# ElevenLabs through NabuGate

NabuGate is the only place the ElevenLabs API key lives. NabuGen (and any other
project) calls the gateway with its own NabuGate key. The gateway adds
`xi-api-key`, meters the call, and returns ElevenLabs' status and body unchanged.

Two surfaces exist:

| Surface | When to use it |
|---|---|
| `POST /v1/audio/speech` and `POST /v1/audio/transcriptions` | OpenAI-shaped clients. Speech here does **not** pass `language_code`, `voice_settings`, audio tags, or timestamps. |
| `/elevenlabs/*` | Everything else, and any speech call that must match ElevenLabs 1:1. **NabuGen should use this.** |

The path under `/elevenlabs` is the ElevenLabs path. A client that today calls
`https://api.elevenlabs.io/v1/text-to-speech/{voice_id}` calls
`https://<gateway>/elevenlabs/v1/text-to-speech/{voice_id}` with the same body.

## Authentication

```http
Authorization: Bearer <nabugate-project-key>
```

Do not send `xi-api-key`. If a request includes it, the gateway drops it and
sends the deployment key instead. The deployment key is `ELEVENLABS_API_KEY`
and is never returned in a response.

The project key is the tenant. It needs `elevenlabs` or `elevenlabs/*` on its
allow-list (an empty allow-list or `*` also permits it). The key's
`rate_limit` (requests per minute) applies to every call, including status
polls. A key restricted to other providers (`providers: ["openai"]`) receives
403.

Optional bring-your-own key, same rules as the rest of the gateway:

```http
X-Nabu-Key-elevenlabs: <that tenant's own ElevenLabs key>
X-Nabu-Key-Mode: own-first
```

A call served on the tenant's own key is metered and costs $0. `own` with no
key is 400. A provider set to `access: request` refuses the gateway key until
the account is approved; the tenant's own key still works.

## What comes back

*   Upstream responses, including errors, are passed through: same status, same
    body, same `Content-Type`. A 422 from ElevenLabs is a 422 here, with
    ElevenLabs' JSON, not a gateway wrapper.
*   Gateway-only failures use `{"error":{"message":"..."}}`:
    *   `401` missing or unknown project key
    *   `403` allow-list, provider list, or `access: request`
    *   `404` path this proxy does not forward (account, workspace, API keys)
    *   `413` upload over 1 GiB, or a JSON body over 48 MiB
    *   `429` project rate limit (`Retry-After: 1`)
    *   `502` ElevenLabs did not answer
    *   `503` `ELEVENLABS_API_KEY` is not set, so the provider never came up
*   Every proxied response also carries:
    *   `X-Nabu-Provider: elevenlabs`
    *   `X-Nabu-Model: <model id used for the price>`
    *   `X-Nabu-Cost-USD: <dollars charged for this response>`

A poll of a job that was already billed returns `X-Nabu-Cost-USD: 0.000000`.

## Bodies, uploads, streaming

JSON is forwarded byte for byte (`language_code`, `voice_settings`, audio tags
such as `[whispers]`, `seed`, pronunciation dictionaries). Multipart uploads
are streamed, not buffered. Audio and video responses are streamed back.
Redirects are not followed, so the API key cannot ride along to another host.

The body cap is 1 GiB. Inline base64 on the video API is the vendor's own
25 MiB limit; larger media goes through `POST /v1/assets` and is referenced by
`asset_id`.

## Metering

Prices are the ElevenLabs API list in `config.default.yaml` (checked
2026-11-17 against [elevenlabs.io/pricing/api](https://elevenlabs.io/pricing/api)).
A call is billed on the 2xx that did the work. Upstream errors are not billed.
Async jobs are billed once: on create when the duration is already known,
otherwise on the first poll that carries a duration. The "already billed" set
is in memory; a restart can charge one late poll a second time.

| Work | Price | How the quantity is taken |
|---|---|---|
| TTS and dialogue, `eleven_v3`, `eleven_multilingual_v2`, `eleven_v4` | $80 / 1M characters | `text`, or the sum of `inputs[].text` / `dialogue[].text`. Audio tags count. |
| Flash / Turbo / `eleven_v3_conversational` / `eleven_v4_turbo` | $40 / 1M characters | same |
| Voice design (`eleven_multilingual_ttv_v2`) | $80 / 1M characters | preview `text`. If the preview text is omitted, 1,000 characters, marked estimated. Saving a preview (`generated_voice_id`, no text) is free. |
| Scribe v2 | $0.22 / hour (`per_minute: 0.003667`) | `audio_duration_secs` on the transcript. Multipart `model_id` is not read; the response's model is used when it has one, otherwise `scribe_v2`. |
| Scribe v1 and v3 | $0.20 / hour | same, when the response names that model |
| Sound effects | $0.12 / minute | `duration_seconds`. If omitted, 10 seconds, marked estimated. |
| Music | $0.15 / minute | `music_length_ms`. If omitted, 30 seconds, marked estimated. `/music/plan`, `/music/upload` and finetunes are not billed as minutes. |
| Voice changer and voice isolator | $0.12 / minute | `X-Nabu-Duration-Seconds`, because the response is audio and has no duration. Without the header the call is recorded unpriced (cost $0, flagged). |
| Legacy dubbing `POST /v1/dubbing` | $0.50 / minute | `expected_duration_sec` on create, or `media_metadata.duration` on the first completed poll. This is the no-watermark API default. |
| Dubbing v2 `POST /v1/dubbing/project` | $2.20 / minute | same. The project API defaults to `dubbing_v2`; the gateway bills that rate for every project-API create. |
| Forced alignment | $0.20 / hour | the last word or character `end` in the response. ElevenLabs does not publish a separate alignment price; this is Scribe's rate. |
| Lip-sync `sync-lipsync-v3`, `sync-3`, `creatify-aurora` | $7.20 / minute ($0.12 / second) | `duration_secs` in the JSON, otherwise `X-Nabu-Duration-Seconds`. sync.so lists sync-3 at about $0.107–$0.133/s. |
| Other video model ids (Veo, Seedance, …) | unpriced until a row is added | forwarded as-is. An unpriced 2xx is recorded with cost $0 and the unpriced flag. |
| Voice list / get / edit / delete, asset upload, model list | not billed | still authenticated and rate-limited |

`X-Nabu-Duration-Seconds` is a number of seconds, must be greater than 0 and at
most 6 hours, and is stripped before the request reaches ElevenLabs. Send it on
voice changer, isolator, and any lip-sync body that has no `duration_secs`.

## Speech

`eleven_v3` accepts audio tags in `text` (`[whispers]`, `[laughs]`, `[excited]`, …).
`language_code` is an ISO 639-1 code. `voice_settings` overrides the stored
voice for this request only.

```bash
curl -X POST "$GATEWAY/elevenlabs/v1/text-to-speech/$VOICE_ID?output_format=mp3_44100_128" \
  -H "Authorization: Bearer $NABU_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "text": "[whispers] سلام، این یک آزمون است.",
    "model_id": "eleven_v3",
    "language_code": "fa",
    "voice_settings": {
      "stability": 0.4,
      "similarity_boost": 0.8,
      "style": 0.0,
      "use_speaker_boost": true,
      "speed": 1.0
    }
  }' \
  --output speech.mp3
```

| Method and path | Response |
|---|---|
| `POST /v1/text-to-speech/{voice_id}` | audio bytes |
| `POST /v1/text-to-speech/{voice_id}/stream` | chunked audio |
| `POST /v1/text-to-speech/{voice_id}/with-timestamps` | JSON: `audio_base64`, `alignment`, `normalized_alignment` (character start/end times) |
| `POST /v1/text-to-speech/{voice_id}/stream/with-timestamps` | streamed alignment |
| `POST /v1/text-to-dialogue` | multi-voice audio |
| `POST /v1/text-to-dialogue/stream` | streamed |
| `POST /v1/text-to-dialogue/with-timestamps` | audio plus alignment |
| `POST /v1/text-to-dialogue/stream/with-timestamps` | streamed |

`output_format` is a query parameter (`mp3_44100_128`, `mp3_44100_192`,
`opus_48000_128`, `pcm_24000`, `wav_44100`, `ulaw_8000`, …). The vendor's tier
still decides which formats it will actually emit.

Dialogue:

```bash
curl -X POST "$GATEWAY/elevenlabs/v1/text-to-dialogue" \
  -H "Authorization: Bearer $NABU_KEY" -H "Content-Type: application/json" \
  -d '{
    "inputs": [
      {"text": "سلام.", "voice_id": "'"$VOICE_A"'"},
      {"text": "[laughs] به به.", "voice_id": "'"$VOICE_B"'"}
    ],
    "model_id": "eleven_v3",
    "language_code": "fa"
  }' --output dialogue.mp3
```

Timestamps:

```bash
curl -X POST "$GATEWAY/elevenlabs/v1/text-to-speech/$VOICE_ID/with-timestamps" \
  -H "Authorization: Bearer $NABU_KEY" -H "Content-Type: application/json" \
  -d '{"text":"سلام.","model_id":"eleven_v3","language_code":"fa"}'
```

## Voices

```bash
# list
curl "$GATEWAY/elevenlabs/v1/voices" -H "Authorization: Bearer $NABU_KEY"
curl "$GATEWAY/elevenlabs/v2/voices" -H "Authorization: Bearer $NABU_KEY"

# one voice, and its settings
curl "$GATEWAY/elevenlabs/v1/voices/$VOICE_ID" -H "Authorization: Bearer $NABU_KEY"
curl "$GATEWAY/elevenlabs/v1/voices/$VOICE_ID/settings" -H "Authorization: Bearer $NABU_KEY"

# shared library
curl "$GATEWAY/elevenlabs/v1/shared-voices?page_size=10" -H "Authorization: Bearer $NABU_KEY"
```

Instant voice clone. The file field is `files` (repeat it for more than one sample).

```bash
curl -X POST "$GATEWAY/elevenlabs/v1/voices/add" \
  -H "Authorization: Bearer $NABU_KEY" \
  -F "name=Ada" \
  -F "description=narrator" \
  -F "files=@sample.mp3"
```

Edit and delete. Voices live on the gateway's ElevenLabs account, so every
tenant of this deployment sees the same library.

```bash
curl -X POST "$GATEWAY/elevenlabs/v1/voices/$VOICE_ID/edit" \
  -H "Authorization: Bearer $NABU_KEY" \
  -F "name=Ada (new)"

curl -X DELETE "$GATEWAY/elevenlabs/v1/voices/$VOICE_ID" \
  -H "Authorization: Bearer $NABU_KEY"
```

Voice design, then save the preview you want:

```bash
curl -X POST "$GATEWAY/elevenlabs/v1/text-to-voice/design" \
  -H "Authorization: Bearer $NABU_KEY" -H "Content-Type: application/json" \
  -d '{
    "voice_description": "warm female narrator, clear Persian",
    "text": "سلام، من صدای جدیدم.",
    "auto_generate_text": false
  }'

curl -X POST "$GATEWAY/elevenlabs/v1/text-to-voice" \
  -H "Authorization: Bearer $NABU_KEY" -H "Content-Type: application/json" \
  -d '{"voice_name":"Ada","voice_description":"warm narrator","generated_voice_id":"'"$PREVIEW_ID"'"}'
```

Also proxied: `POST /v1/text-to-voice/create-previews`,
`POST /v1/text-to-voice/{voice_id}/remix`,
`GET /v1/text-to-voice/{generated_voice_id}/stream`,
`GET /v1/voices/settings/default`,
`POST /v1/voices/{voice_id}/settings/edit`,
`POST /v1/similar-voices`, and the PVC routes under `/v1/voices/pvc/...`.

## Speech to text (Scribe)

```bash
curl -X POST "$GATEWAY/elevenlabs/v1/speech-to-text" \
  -H "Authorization: Bearer $NABU_KEY" \
  -F "model_id=scribe_v2" \
  -F "language_code=fa" \
  -F "timestamps_granularity=word" \
  -F "diarize=true" \
  -F "file=@note.mp3"
```

The JSON response is Scribe's: `text`, `language_code`, `words[]`
(`text`, `start`, `end`, `type`, `speaker_id`), `audio_duration_secs`.
Billing uses `audio_duration_secs`.

Async (webhook) creates return an id; fetch the transcript later. The first
fetch that includes a duration is the one that is billed.

```bash
curl "$GATEWAY/elevenlabs/v1/speech-to-text/transcripts/$TRANSCRIPTION_ID" \
  -H "Authorization: Bearer $NABU_KEY"

curl -X DELETE "$GATEWAY/elevenlabs/v1/speech-to-text/transcripts/$TRANSCRIPTION_ID" \
  -H "Authorization: Bearer $NABU_KEY"
```

## Dubbing

Legacy create. Multipart. `expected_duration_sec` on the 200 is what gets billed
at $0.50/min. Poll until `status` is `dubbed`, then download the audio. The
poll is not billed again.

```bash
curl -X POST "$GATEWAY/elevenlabs/v1/dubbing" \
  -H "Authorization: Bearer $NABU_KEY" \
  -F "name=spot" \
  -F "source_lang=fa" \
  -F "target_lang=en" \
  -F "file=@spot.mp4"

curl "$GATEWAY/elevenlabs/v1/dubbing/$DUBBING_ID" \
  -H "Authorization: Bearer $NABU_KEY"

curl "$GATEWAY/elevenlabs/v1/dubbing/$DUBBING_ID/audio/en" \
  -H "Authorization: Bearer $NABU_KEY" --output dubbed.mp3
```

Dubbing v2 is the project API and defaults to `dubbing_v2` ($2.20/min). Create
returns before the source is fetched (`queued`, then `ready`). Pass
`X-Nabu-Duration-Seconds` if you know the source length at create time;
otherwise the first poll that includes `media_metadata.duration` is billed.

```bash
curl -X POST "$GATEWAY/elevenlabs/v1/dubbing/project" \
  -H "Authorization: Bearer $NABU_KEY" \
  -H "X-Nabu-Duration-Seconds: 45" \
  -F "model_id=dubbing_v2" \
  -F "source_language=fa" \
  -F "target_language=en" \
  -F "file=@spot.mp4"
```

Also proxied: `GET /v1/dubbing`, `DELETE /v1/dubbing/{dubbing_id}`,
`GET /v1/dubbing/{dubbing_id}/transcript/{language_code}`, and the project
language, transcript, and render routes under `/v1/dubbing/project/...` and
`/v1/dubbing/resource/...`.

## Sound effects

`duration_seconds` is optional (0.5–30). Omitted, the gateway bills 10 seconds
and marks the row estimated.

```bash
curl -X POST "$GATEWAY/elevenlabs/v1/sound-generation" \
  -H "Authorization: Bearer $NABU_KEY" -H "Content-Type: application/json" \
  -d '{"text":"soft rain on a window","duration_seconds":6,"prompt_influence":0.3}' \
  --output rain.mp3
```

## Music

```bash
curl -X POST "$GATEWAY/elevenlabs/v1/music" \
  -H "Authorization: Bearer $NABU_KEY" -H "Content-Type: application/json" \
  -d '{"prompt":"quiet solo piano, no vocals","music_length_ms":15000,"model_id":"music_v1"}' \
  --output piano.mp3

curl -X POST "$GATEWAY/elevenlabs/v1/music/stream" \
  -H "Authorization: Bearer $NABU_KEY" -H "Content-Type: application/json" \
  -d '{"prompt":"quiet solo piano","music_length_ms":15000}'
```

Also proxied: `POST /v1/music/detailed`, `POST /v1/music/detailed/stream`,
`POST /v1/music/plan` (not billed as audio), `POST /v1/music/video-to-music`,
`POST /v1/music/stem-separation`, and `/v1/music/finetunes`.

## Voice changer (speech to speech)

ElevenLabs serves this at `/v1/voice-changer/{voice_id}`. The gateway also
accepts `/v1/speech-to-speech/{voice_id}` and rewrites it to that path. The
file field is `audio`. Send `X-Nabu-Duration-Seconds` so the minute rate can
be applied.

```bash
curl -X POST "$GATEWAY/elevenlabs/v1/speech-to-speech/$VOICE_ID" \
  -H "Authorization: Bearer $NABU_KEY" \
  -H "X-Nabu-Duration-Seconds: 12" \
  -F "model_id=eleven_multilingual_sts_v2" \
  -F "audio=@in.wav" \
  --output changed.mp3

curl -X POST "$GATEWAY/elevenlabs/v1/voice-changer/$VOICE_ID/stream" \
  -H "Authorization: Bearer $NABU_KEY" \
  -H "X-Nabu-Duration-Seconds: 12" \
  -F "audio=@in.wav"
```

## Voice isolator

File field `audio`. Same duration header.

```bash
curl -X POST "$GATEWAY/elevenlabs/v1/audio-isolation" \
  -H "Authorization: Bearer $NABU_KEY" \
  -H "X-Nabu-Duration-Seconds: 20" \
  -F "audio=@noisy.wav" \
  --output isolated.wav

curl -X POST "$GATEWAY/elevenlabs/v1/audio-isolation/stream" \
  -H "Authorization: Bearer $NABU_KEY" \
  -H "X-Nabu-Duration-Seconds: 20" \
  -F "audio=@noisy.wav"
```

## Forced alignment

File field `file`, transcript field `text`. The response is character and word
timings; billing uses the last `end`.

```bash
curl -X POST "$GATEWAY/elevenlabs/v1/forced-alignment" \
  -H "Authorization: Bearer $NABU_KEY" \
  -F "text=سلام، این متن همان صدا است." \
  -F "file=@speech.wav"
```

## Audio Native

```bash
curl -X POST "$GATEWAY/elevenlabs/v1/audio-native" \
  -H "Authorization: Bearer $NABU_KEY" \
  -F "name=article" \
  -F "voice_id=$VOICE_ID" \
  -F "model_id=eleven_multilingual_v2" \
  -F "auto_convert=true" \
  -F "file=@article.html"
```

Also proxied: `GET /v1/audio-native/{project_id}/settings` and the content
update routes under `/v1/audio-native/`.

## Video lip-sync

Create a job, poll it, then read `content_url` from the completed payload.
That URL is a signed vendor URL (about an hour). Fetch the generation again
for a fresh one. The gateway does not proxy the file host.

`model_id` is forwarded as sent. The gateway prices `sync-lipsync-v3`,
`sync-3` and `creatify-aurora`. The video API's published models also include
`creatify-aurora` (image + speech audio), `veo-3.1-generate-001`,
`veo-3.1-fast-generate-001`, and the `bytedance-seedance-*` ids. Any other id
is forwarded; if ElevenLabs rejects it, you get their error body unchanged.

Send `duration_secs` when the model accepts it. For a lipsync body that has no
duration field, send `X-Nabu-Duration-Seconds` on the create. A create that
has neither is not billed (the completed poll does not carry a duration).

```bash
# 1. Large inputs: upload, then reference the asset.
curl -X POST "$GATEWAY/elevenlabs/v1/assets" \
  -H "Authorization: Bearer $NABU_KEY" \
  -F "name=face" -F "asset=@face.png"
# -> {"asset_id":"..."}

# 2. Create. sync-lipsync-v3 is the Sync lipsync model.
curl -X POST "$GATEWAY/elevenlabs/v1/flows/video" \
  -H "Authorization: Bearer $NABU_KEY" \
  -H "Content-Type: application/json" \
  -H "X-Nabu-Duration-Seconds: 8" \
  -d '{
    "model_id": "sync-lipsync-v3",
    "image": {"type": "asset", "asset_id": "'"$IMAGE_ASSET"'"},
    "audio": {"type": "asset", "asset_id": "'"$AUDIO_ASSET"'"}
  }'
# -> {"id":"gen_...","status":"pending"}

# 3. Poll until status is "completed" or "failed".
curl "$GATEWAY/elevenlabs/v1/flows/video/$GENERATION_ID" \
  -H "Authorization: Bearer $NABU_KEY"
# completed:
# {"id":"...","status":"completed","content_url":"https://...","content_mime_type":"video/mp4"}

# List
curl "$GATEWAY/elevenlabs/v1/flows/video?page_size=20" \
  -H "Authorization: Bearer $NABU_KEY"
```

A model that takes `duration_secs` (Veo, Seedance) puts it in the JSON and
does not need the header:

```bash
curl -X POST "$GATEWAY/elevenlabs/v1/flows/video" \
  -H "Authorization: Bearer $NABU_KEY" \
  -H "Content-Type: application/json" \
  -H "X-Nabu-Duration-Seconds: 8" \
  -d '{
    "model_id": "creatify-aurora",
    "image": {"type": "asset", "asset_id": "'"$IMAGE_ASSET"'"},
    "audio": {"type": "asset", "asset_id": "'"$AUDIO_ASSET"'"}
  }'
```

`creatify-aurora` takes `image` and `audio` and has no `duration_secs`. The
header is how that job is metered. Models that do have `duration_secs` (Veo,
Seedance) should send it in the JSON and can omit the header.

Inline images (25 MiB decoded) use
`{"type":"inline_base64","content_base64":"...","mime_type":"image/png"}`
instead of an asset. A previous generation is
`{"type":"generation","generation_id":"..."}`.

## Assets and the model list

```bash
curl -X POST "$GATEWAY/elevenlabs/v1/assets" \
  -H "Authorization: Bearer $NABU_KEY" \
  -F "name=clip" -F "asset=@clip.mp4"

curl "$GATEWAY/elevenlabs/v1/assets/$ASSET_ID" -H "Authorization: Bearer $NABU_KEY"
curl -X DELETE "$GATEWAY/elevenlabs/v1/assets/$ASSET_ID" -H "Authorization: Bearer $NABU_KEY"

curl "$GATEWAY/elevenlabs/v1/models" -H "Authorization: Bearer $NABU_KEY"
```

## Paths that are not forwarded

`/v1/user`, `/v1/workspace/*`, service accounts, API-key management, and
history. Those administer the shared ElevenLabs account. They answer 404 from
the gateway and are not sent upstream.

## Deployment

Set this on the NabuGate process. Nothing else is required for the proxy; the
provider block is already in `config.default.yaml`.

```bash
ELEVENLABS_API_KEY=...
```

The provider stays off when that variable is empty, and `/elevenlabs/*` then
answers 503. The OpenAI-shaped speech and transcription aliases that name
`elevenlabs` stay dark the same way.

Give the NabuGen project key an allow-list entry of `elevenlabs` (or `*`) and
a `rate_limit` that can absorb status polling.
