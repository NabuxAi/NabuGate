package web

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The bundle under dist/ is committed and the Dockerfile never runs npm, so a
// change under src/ that was not rebuilt produces a deploy byte-identical to
// the one before it. Nothing catches that on its own: Go cannot see the
// frontend and there are no JS tests, so the change silently never ships.
//
// Three commits in this repo did exactly that — two dark-mode colour fixes and
// a CSS typo — and only reached production when somebody rebuilt for an
// unrelated reason months later.
//
// `npm run build` stamps src.sha256 with a hash of src/. This recomputes it. A
// mismatch means the sources moved after the last build.
func TestBundleWasBuiltFromTheCurrentSources(t *testing.T) {
	recorded, err := os.ReadFile("src.sha256")
	if err != nil {
		t.Fatalf("no src.sha256: run `cd web && npm run build` to create it (%v)", err)
	}

	got, err := hashSources("src")
	if err != nil {
		t.Fatalf("hash sources: %v", err)
	}
	if want := strings.TrimSpace(string(recorded)); got != want {
		t.Fatalf("web/src has changed since the bundle was last built.\n"+
			"  recorded %s\n  actual   %s\n"+
			"Run `cd web && npm run build` and commit web/dist and web/src.sha256, "+
			"or the change will not reach production.", want, got)
	}
}

// hashSources mirrors web/stamp-src-hash.mjs exactly: files in sorted order by
// path relative to root, each contributing its slash-separated path and its
// bytes, both NUL-terminated.
func hashSources(root string) (string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	// WalkDir already visits lexically, which is what the Node side sorts to.

	h := sha256.New()
	for _, path := range files {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return "", err
		}
		h.Write([]byte(filepath.ToSlash(rel)))
		h.Write([]byte{0})
		body, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		h.Write(body)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
