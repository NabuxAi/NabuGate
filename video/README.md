# NabuGate intro video

A 45-second, 16:9 explainer rendered with [Remotion](https://www.remotion.dev/) from one composition. Locales live in `src/copy.js` and match the site languages (`fa`, `en`, `ar`). Persian uses Vazirmatn, English uses Inter, Arabic uses Noto Naskh Arabic. `fa` and `ar` are laid out right to left; code, alias names and header names stay left to right.

The picture carries the whole script. There is no voiceover and no music — only a silent audio track on the MP4 so a player has something to mute. Do not invent a voice track. If a real narration is added later, it has to come from this deployment’s own `POST /v1/audio/speech`.

## Re-render

Chrome and ffmpeg are required.

```bash
cd video
npm install
# CHROME=/path/to/google-chrome npm run render
npm run render
```

`render.mjs` writes, for each locale:

| File | Where it is served |
|---|---|
| `nabugate-<lang>.mp4` | H.264, silent AAC, faststart |
| `nabugate-<lang>.webm` | VP9, no audio |
| `nabugate-<lang>.jpg` | Poster frame |

into `web/public/intro/`. Then rebuild the site so the gateway’s embedded bundle picks them up:

```bash
cd web && npm run build
```

Commit `web/public/intro/`, `web/dist/` and `web/src.sha256` together.

## Claude Desktop tutorial

`src/Guide.jsx` is a second composition, `guide-<lang>`: a narrated, step-by-step tutorial for connecting Claude Desktop's third-party Gateway mode to NabuGate. It is embedded at the top of the docs section `claude-desktop`. Unlike the intro, it has a voice. The script lives in `src/guideCopy.js`: `say` is narrated, `title` and `body` are shown on screen. A scene lasts as long as its narration clip (`src/guideTimings.json`, written by `tts.mjs`).

```bash
cd video
# Narration: ElevenLabs eleven_v3 (the model that speaks Persian), one MP3 per
# scene into public/guide/<lang>/. Only missing clips are generated; --force
# remakes all of them. The key comes from the environment and is never stored.
ELEVENLABS_API_KEY=… node tts.mjs
# Picture + sound → web/public/guide/claude-desktop-<lang>.{mp4,jpg}
CHROME=/path/to/chrome node render-guide.mjs        # or: node render-guide.mjs fa
```

Then rebuild the site as above. If the menu labels in Claude Desktop change, edit `UI` in `src/guideCopy.js`; the labels follow Anthropic's third-party docs (claude.com/docs/third-party/claude-desktop).

## Fonts

Shipped under `public/fonts/` so a re-render does not fetch them:

- Vazirmatn (Saber Rastikerdar) — SIL Open Font License 1.1
- Inter (Rasmus Andersson) — SIL Open Font License 1.1
- Noto Naskh Arabic (Google) — SIL Open Font License 1.1
