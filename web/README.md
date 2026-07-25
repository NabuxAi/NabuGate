# NabuGate Console (`web/`)

A web admin console for the NabuGate AI gateway — the **NabuGate console** screen
from the *NabuGen* design, implemented as a React + Vite SPA. RTL-first
(Persian, Vazirmatn), built on the NabuDesk design language (indigo/blue accent,
slate neutrals, emerald/amber/violet statuses).

## Screens

| View | What it shows |
| --- | --- |
| **داشبورد** (Dashboard) | KPI tiles (requests / tokens / cost / active providers / fallback rate), provider grid with live/idle status + type badges, usage-by-project, service health. |
| **پرووایدرها** (Providers) | Every upstream provider from `config.yaml` — type, `base_url`, enabled/skipped. |
| **مدل‌ها و آلیاس‌ها** (Models & aliases) | Chat alias → primary + fallback routing table, plus image / audio / embedding aliases and the passthrough note. |
| **کلیدهای پروژه** (Project keys) | Per-key policy: project, `allow` list (globs), rate limit. |
| **مصرف و هزینه** (Usage & cost) | Usage by model and the pricing table (USD / 1M tokens). |
| **ساب‌اجنت‌ها**، **لاگ‌ها** | Placeholders for follow-up work. |

## Data

The views read from `src/data/mock.js`, whose shapes mirror the gateway's real
config (providers, alias routes, per-key policy) and its `GET /v1/usage` output.
The numbers are **representative** — swap the module for live calls to
`GET /v1/models` and `GET /v1/usage` (and the config) to make it real, with no
change to the views.

## Develop

```bash
cd web
npm install
npm run dev      # http://localhost:5173
npm run build    # → dist/  (static bundle; base is relative)
npm run preview  # serve the built bundle
```

The bundle is static (`base: './'`), so it can be served by the gateway, a
static host, or Coolify. Fonts (Vazirmatn) load from Google Fonts, matching the
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
`src/styles/tokens.css`.
