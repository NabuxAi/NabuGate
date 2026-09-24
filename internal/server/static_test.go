package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func testSite(t *testing.T) *staticSite {
	t.Helper()
	site, err := newStaticSite(fstest.MapFS{
		"index.html":              {Data: []byte(`<!doctype html><div id="root"></div>`)},
		"assets/app-abc123.js":    {Data: []byte("console.log('app')")},
		"assets/app-abc123.js.br": {Data: []byte("BR-BYTES")},
		"assets/app-abc123.js.gz": {Data: []byte("GZ-BYTES")},
		"assets/font-xyz.woff2":   {Data: []byte("wOF2")},
	})
	if err != nil {
		t.Fatal(err)
	}
	return site
}

func get(t *testing.T, h http.Handler, path string, headers map[string]string) (*http.Response, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body, _ := io.ReadAll(rec.Result().Body)
	return rec.Result(), string(body)
}

// Hashed assets never change meaning, so a browser may keep them for a year
// without asking; the shell that names them must be re-checked every time.
func TestStaticSiteCachePolicy(t *testing.T) {
	site := testSite(t)

	resp, _ := get(t, site, "/assets/font-xyz.woff2", nil)
	if cc := resp.Header.Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Fatalf("hashed asset Cache-Control = %q, want a year, immutable", cc)
	}

	for _, p := range []string{"/", "/index.html", "/docs", "/tokens"} {
		resp, body := get(t, site, p, nil)
		if resp.StatusCode != http.StatusOK || body != `<!doctype html><div id="root"></div>` {
			t.Fatalf("GET %s = %d %q, want the app shell", p, resp.StatusCode, body)
		}
		if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
			t.Fatalf("GET %s Cache-Control = %q, want no-cache (a deploy must reach the browser)", p, cc)
		}
		if resp.Header.Get("ETag") == "" {
			t.Fatalf("GET %s has no ETag, so every re-check downloads the body again", p)
		}
	}
}

// The re-check of an unchanged shell is a 304 with no body.
func TestStaticSiteRevalidates(t *testing.T) {
	site := testSite(t)
	first, _ := get(t, site, "/", nil)
	etag := first.Header.Get("ETag")

	again, body := get(t, site, "/", map[string]string{"If-None-Match": etag})
	if again.StatusCode != http.StatusNotModified || body != "" {
		t.Fatalf("revalidation = %d with %d bytes, want 304 and no body", again.StatusCode, len(body))
	}
}

// The build's precompressed copies are served to clients that accept them,
// under their own ETag, and never to a client that did not ask.
func TestStaticSitePrecompressed(t *testing.T) {
	site := testSite(t)
	cases := []struct {
		accept, wantEnc, wantBody string
	}{
		{"gzip, deflate, br", "br", "BR-BYTES"},
		{"gzip", "gzip", "GZ-BYTES"},
		{"br;q=0, gzip", "gzip", "GZ-BYTES"},
		{"", "", "console.log('app')"},
	}
	etags := map[string]bool{}
	for _, c := range cases {
		resp, body := get(t, site, "/assets/app-abc123.js", map[string]string{"Accept-Encoding": c.accept})
		if enc := resp.Header.Get("Content-Encoding"); enc != c.wantEnc || body != c.wantBody {
			t.Fatalf("Accept-Encoding %q: got encoding %q body %q, want %q %q", c.accept, enc, body, c.wantEnc, c.wantBody)
		}
		if resp.Header.Get("Vary") != "Accept-Encoding" {
			t.Fatalf("Accept-Encoding %q: missing Vary, so a shared cache could hand Brotli to a client that cannot read it", c.accept)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "text/javascript; charset=utf-8" {
			t.Fatalf("Content-Type = %q, want the original file's type, not the encoding's", ct)
		}
		etags[resp.Header.Get("ETag")] = true
	}
	if len(etags) != 3 {
		t.Fatalf("got %d distinct ETags for identity/br/gzip, want 3 (one per representation)", len(etags))
	}
}

// A chunk from a previous deploy is a 404, not the HTML shell, which the
// browser would try to run as JavaScript.
func TestStaticSiteMissingAssetIs404(t *testing.T) {
	resp, _ := get(t, testSite(t), "/assets/app-oldhash.js", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing hashed asset = %d, want 404", resp.StatusCode)
	}
}

func TestAcceptsEncoding(t *testing.T) {
	for _, c := range []struct {
		header, coding string
		want           bool
	}{
		{"gzip, br", "br", true},
		{"GZIP", "gzip", true},
		{"br;q=0", "br", false},
		{"br; q=0.0, gzip", "br", false},
		{"br;q=0.5", "br", true},
		{"deflate", "gzip", false},
		{"", "gzip", false},
	} {
		if got := acceptsEncoding(c.header, c.coding); got != c.want {
			t.Errorf("acceptsEncoding(%q, %q) = %v, want %v", c.header, c.coding, got, c.want)
		}
	}
}
