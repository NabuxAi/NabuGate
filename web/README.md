# NabuGate Console (`web/`)

A web admin console for the NabuGate AI gateway — the **NabuGate console** screen
from the *NabuGen* design, implemented as a React + Vite SPA. Bilingual
(Persian RTL and English LTR), built on the NabuDesk design language
(indigo-to-violet brand gradient, slate neutrals, emerald/amber/violet statuses).

## Screens

Three surfaces share one bundle and one design system:

| Surface | Path | What it is |
| --- | --- | --- |
| **Landing** | `/`, `/fa`, `/en` | Public home page: hero with a live routing demo, provider/model marquee (from `/api/public/models`), features, setup snippets per tool, pricing, FAQ. |
| **Docs** | `/docs`, `/en/docs`, and inside the console | Searchable section nav, "on this page" rail, prev/next; one content file per language in `src/views/docs/`. |
| **Console** | `/panel/…` (users), `/admin/…` (admins) | Dashboard, balance & usage, plans, payments, API keys, providers, models, requests, integration, profile, security; admins also get global usage, users, access requests, agents & flows, system keys. |

## Languages

Persian (`fa`, right-to-left, the default) and English (`en`, left-to-right).
`src/i18n/index.jsx` holds the language provider, `useT()` and the locale-aware
formatters (`fmtInt`, `usd`, `fmtDate` …). Each component keeps its own
`{ fa, en }` dictionary next to the markup that uses it. The language comes from
`?lang=`, a `/fa` or `/en` path prefix, the saved choice, or the browser, in
that order; `index.html` applies it before first paint so the page never flips
direction after loading.

## Develop

```bash
cd web
npm install
npm run dev      # http://localhost:5173
npm run build    # → dist/  (static bundle; base is relative)
npm run preview  # serve the built bundle
```

The bundle is static (`base: './'`), so it can be served by the gateway, a
static host, or Coolify. Fonts (Vazirmatn, Inter, JetBrains Mono) load from Google Fonts, matching the
design; self-host under `public/` if an offline build is required.

## Served by the gateway (`/admin/`)

The built bundle is committed to `web/dist/` and embedded into the gateway
binary (`web/embed.go`, `//go:embed all:dist`), so a running gateway serves the
console at **`/admin/`** with no extra process:

```bash
go run ./cmd/gateway -config config.yaml   # then open http://localhost:8080/admin/
```

Only the static shell is served openly; every piece of live gateway data still
comes from the auth-guarded `/v1/*` endpoints, so the console carries the admin
key when it calls them. If `web/dist/` is missing (bundle not built), the
gateway simply skips mounting `/admin/` and behaves exactly as before.

**After changing the console sources, rebuild and re-commit the bundle** so the
embedded copy stays in sync:

```bash
cd web && npm run build   # regenerates web/dist/ (committed)
```

## Stack

React 18 + Vite 5, no router (lightweight `useState` view switch), no UI
framework — styling is plain CSS driven by design tokens in
`src/styles/tokens.css` (day and night themes), refined by `polish.css` and
`shell.css`; the landing and docs pages add `landing.css` and `docs.css`. Icons
are inline SVG (`src/components/Icon.jsx`). Fonts: Vazirmatn for Persian,
Inter for English, JetBrains Mono for code, from Google Fonts.
