# Routerah model catalogue in NabuGate

Routerah is configured as an OpenAI-compatible passthrough provider. No
static list of 800+ IDs is committed: NabuGate fetches Routerah's public
catalogue every five minutes and publishes each ID under `routerah/`.

## Enable

1. Set `ROUTERAH_API_KEY` as a **secret** in NabuGate's Coolify environment
   (or your local `.env`). Never commit it.
2. Redeploy the NabuGate service (Coolify must pass the env var to the
   container). If `NABU_CONFIG_YAML` overrides the shipped config, also
   declare the Routerah provider with `models_url` in that override.
3. Confirm that `GET /v1/models` returns `routerah/<model-id>` entries.
   Requests still need a valid NabuGate project key permitted to use that ID.
4. Call `POST /v1/chat/completions` with e.g.
   `"model": "routerah/ripple-65b-preview"` or the exact ID from the catalog.

For a project-scoped token, add `routerah/*` only to its allow-list if that
project should be able to use this provider. A gateway admin key can call
all permitted routes. Users with a Routerah key may also pass their own key
where NabuGate's bring-your-own-key controls support it.

## Catalogue and billing

- Live discovery: `GET https://api.routerah.app/api/pricing/catalog`
  (public, no provider credential needed). The catalog supports text, image,
  audio, embeddings and video **listings**.
- Inference: `https://api.routerah.app/v1` (requires `ROUTERAH_API_KEY`).
- Prices: loaded from the same catalog when the key enables Routerah and
  refreshed every 15 minutes. The gateway retains the last good prices on
  failed refreshes. It disables Routerah on startup if prices cannot load,
  rather than meter paid requests as free.
- Text/embedding rates use USD per million tokens. Image fixed rates use
  USD per image. Unsupported fixed-price units are *not* silently treated
  as text tokens.
- Existing explicit `pricing:` entries like `routerah/<model-id>` take
  precedence. Model IDs are preserved, including namespaced IDs and :free.

## Capability boundaries

The normal OpenAI-compatible chat, image, speech, transcription and embedding
endpoints can route to an eligible Routerah model. Each request must use the
appropriate endpoint, and upstream availability varies by account and model.

NabuGate does not currently expose a video-generation endpoint, so **video
models can be listed but cannot be generated through NabuGate yet**.
Neither presence in the public catalog nor visibility in `/v1/models` proves
that a model is usable by the configured Routerah key. Check upstream
entitlements and actual request results before publishing a claim of access.
