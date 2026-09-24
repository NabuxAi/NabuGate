package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

// staticSite serves the embedded web bundle (landing page, docs and console).
//
// It replaced a bare http.FileServer over the embed.FS, which sent no caching
// information at all: embedded files have no modification time, so there was
// no Last-Modified, and FileServer sets no ETag or Cache-Control. Every visit —
// a reload, a second tab, the next day — downloaded the whole bundle again.
//
//   - Files under assets/ carry a content hash in their name (Vite does that),
//     so a URL never changes meaning: they are cached for a year, immutable.
//   - Everything else — index.html above all, which names those hashed files —
//     must be re-checked on every load or a deploy would never reach a browser
//     that had the old one. It is sent no-cache with a strong ETag, so the
//     re-check is a 304 with no body when nothing changed.
//   - The build writes .br and .gz beside each text asset (web/compress-dist.mjs)
//     and the one the client accepts is served, so compression costs nothing at
//     request time and gets Brotli's better ratio, which the edge proxy does not
//     offer. A proxy leaves a response that is already encoded alone.
type staticSite struct {
	files map[string]*staticFile
}

type staticFile struct {
	body      []byte
	br, gz    []byte
	etag      string
	ctype     string
	immutable bool
}

// newStaticSite reads the bundle into memory once. It is small (well under a
// megabyte without the precompressed copies) and read on every page load, so
// holding it avoids re-reading the embed.FS and re-hashing on each request.
func newStaticSite(assets fs.FS) (*staticSite, error) {
	s := &staticSite{files: map[string]*staticFile{}}
	err := fs.WalkDir(assets, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if strings.HasSuffix(p, ".br") || strings.HasSuffix(p, ".gz") {
			return nil // attached to their original below
		}
		body, err := fs.ReadFile(assets, p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(body)
		f := &staticFile{
			body:      body,
			etag:      `"` + hex.EncodeToString(sum[:12]) + `"`,
			ctype:     mime.TypeByExtension(path.Ext(p)),
			immutable: strings.HasPrefix(p, "assets/"),
		}
		if f.ctype == "" {
			f.ctype = http.DetectContentType(body)
		}
		f.br, _ = fs.ReadFile(assets, p+".br")
		f.gz, _ = fs.ReadFile(assets, p+".gz")
		s.files[p] = f
		return nil
	})
	if err != nil {
		return nil, err
	}
	if s.files["index.html"] == nil {
		return nil, fs.ErrNotExist
	}
	return s, nil
}

func (s *staticSite) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	f := s.files[p]
	if f == nil {
		// A hashed asset that does not exist is a stale page asking for a chunk
		// from a previous deploy. Answering it with index.html (the SPA
		// fallback) hands the browser HTML where it expects JavaScript, which
		// fails with a MIME error that says nothing useful; a 404 is honest.
		if strings.HasPrefix(p, "assets/") {
			http.NotFound(w, r)
			return
		}
		// Any other path is a client-side route: serve the app shell.
		f = s.files["index.html"]
	}

	h := w.Header()
	h.Set("Content-Type", f.ctype)
	if f.immutable {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "no-cache")
	}

	body, etag := f.body, f.etag
	if f.br != nil || f.gz != nil {
		h.Add("Vary", "Accept-Encoding")
		switch ae := r.Header.Get("Accept-Encoding"); {
		case f.br != nil && acceptsEncoding(ae, "br"):
			body, etag = f.br, strings.TrimSuffix(f.etag, `"`)+`-br"`
			h.Set("Content-Encoding", "br")
		case f.gz != nil && acceptsEncoding(ae, "gzip"):
			body, etag = f.gz, strings.TrimSuffix(f.etag, `"`)+`-gz"`
			h.Set("Content-Encoding", "gzip")
		}
	}
	// ServeContent answers If-None-Match with a 304, and HEAD and Range
	// requests, from the ETag set here.
	h.Set("ETag", etag)
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body))
}

// acceptsEncoding reports whether an Accept-Encoding header allows coding,
// honouring an explicit q=0 refusal.
func acceptsEncoding(header, coding string) bool {
	for _, part := range strings.Split(header, ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(name), coding) {
			continue
		}
		q := strings.ReplaceAll(strings.TrimSpace(params), " ", "")
		return q != "q=0" && q != "q=0.0" && q != "q=0.00" && q != "q=0.000"
	}
	return false
}
