// Package web embeds the built NabuGate admin console (the Vite bundle under
// dist/) so the gateway ships it as a single static binary — no Node runtime
// and no separate static host required. The bundle is committed to the repo so
// `go build ./...` works without an npm build step; rebuild it with
// `cd web && npm run build` after changing the console sources.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Assets returns the console bundle rooted at its dist directory (so "index.html"
// and "assets/…" resolve directly). ok is false when the bundle has not been
// built/committed, letting the caller skip mounting the console instead of
// failing to start.
func Assets() (files fs.FS, ok bool) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, false
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil, false
	}
	return sub, true
}
