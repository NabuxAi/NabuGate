package web

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The bundle under dist/ is committed and the Dockerfile never runs npm, so a
// change to the sources that was not rebuilt produces a deploy byte-identical
// to the one before it. Nothing catches that on its own: Go cannot see the
// frontend and there are no JS tests.
//
// `npm run build` stamps src.sha256 over the tracked build inputs AND the asset
// filenames dist/index.html references. This recomputes both, so the stamp
// cannot be current while dist is stale, missing, or was left out of a commit.
func TestBundleWasBuiltFromTheCurrentSources(t *testing.T) {
	recorded, err := os.ReadFile("src.sha256")
	if err != nil {
		t.Fatalf("no src.sha256: run `cd web && npm run build` to create it (%v)", err)
	}

	got, err := stampHash()
	if err != nil {
		t.Fatalf("recompute stamp: %v", err)
	}
	if want := strings.TrimSpace(string(recorded)); got != want {
		t.Fatalf("the console bundle does not match its sources.\n"+
			"  recorded %s\n  actual   %s\n"+
			"Run `cd web && npm run build`, then commit web/dist and web/src.sha256 "+
			"together — or the change will not reach production.", want, got)
	}
}

var assetRe = regexp.MustCompile(`assets/[A-Za-z0-9._-]+`)

// stampHash mirrors web/stamp-src-hash.mjs exactly.
//
// The file list comes from git, not the filesystem: walking picked up untracked
// files (a .DS_Store) that CI never sees, so CI failed telling the developer to
// rebuild — which reproduced the same wrong stamp.
func stampHash() (string, error) {
	out, err := exec.Command("git", "ls-files", "-z", "src", "public", "index.html", "vite.config.js").Output()
	if err != nil {
		return "", err
	}
	var files []string
	for _, f := range strings.Split(string(out), "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		return "", os.ErrNotExist
	}
	sort.Strings(files)

	h := sha256.New()
	for _, rel := range files {
		h.Write([]byte(rel))
		h.Write([]byte{0})
		body, err := os.ReadFile(filepath.FromSlash(rel))
		if err != nil {
			return "", err
		}
		h.Write(body)
		h.Write([]byte{0})
	}

	indexHTML, err := os.ReadFile(filepath.Join("dist", "index.html"))
	if err != nil {
		return "", err
	}
	assets := assetRe.FindAllString(string(indexHTML), -1)
	if len(assets) == 0 {
		return "", os.ErrNotExist
	}
	sort.Strings(assets)
	h.Write([]byte("dist:"))
	for _, a := range assets {
		h.Write([]byte(a))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
