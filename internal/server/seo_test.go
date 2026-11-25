package server

import (
	"encoding/json"
	"encoding/xml"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"nabugate/web"
)

// shellLikeTheBuild has the parts of web/dist/index.html the server rewrites.
const shellLikeTheBuild = `<!doctype html>
<html lang="fa" dir="rtl" data-theme="dark">
  <head>
    <meta charset="UTF-8" />
    <title>NabuGate</title>
    <meta name="description" content="placeholder" />
    <meta name="theme-color" content="#070b16" />
  </head>
  <body><div id="root"></div></body>
</html>`

func seoSite(t *testing.T) *staticSite {
	t.Helper()
	t.Setenv("NABU_PUBLIC_URL", "")
	site, err := newStaticSite(fstest.MapFS{
		"index.html":            {Data: []byte(shellLikeTheBuild)},
		"intro/nabugate-fa.jpg": {Data: []byte("JPEG")},
		"assets/app-abc123.js":  {Data: []byte("console.log('app')")},
	})
	if err != nil {
		t.Fatal(err)
	}
	return site
}

func on(host string) map[string]string { return map[string]string{"X-Nabu-Mirror-Host": host} }

// Each public URL gets its own title, canonical, alternates and preview card,
// with Persian canonical on .ir and English and Arabic on .com, whichever host
// served the request.
func TestPublicPagesCarryTheirOwnHead(t *testing.T) {
	site := seoSite(t)
	cases := []struct {
		host, path, lang, dir, canonical, title string
	}{
		{"gate.nabuxai.ir", "/", "fa", "rtl", "https://gate.nabuxai.ir/", seoCopy["home"]["fa"].title},
		{"gate.nabuxai.com", "/", "fa", "rtl", "https://gate.nabuxai.ir/", seoCopy["home"]["fa"].title},
		{"gate.nabuxai.ir", "/fa", "fa", "rtl", "https://gate.nabuxai.ir/", seoCopy["home"]["fa"].title},
		{"gate.nabuxai.com", "/en", "en", "ltr", "https://gate.nabuxai.com/en", seoCopy["home"]["en"].title},
		{"gate.nabuxai.ir", "/en/", "en", "ltr", "https://gate.nabuxai.com/en", seoCopy["home"]["en"].title},
		{"gate.nabuxai.com", "/ar", "ar", "rtl", "https://gate.nabuxai.com/ar", seoCopy["home"]["ar"].title},
		{"gate.nabuxai.com", "/?lang=en", "en", "ltr", "https://gate.nabuxai.com/en", seoCopy["home"]["en"].title},
		{"gate.nabuxai.ir", "/docs", "fa", "rtl", "https://gate.nabuxai.ir/docs", seoCopy["docs"]["fa"].title},
		{"gate.nabuxai.com", "/en/docs", "en", "ltr", "https://gate.nabuxai.com/en/docs", seoCopy["docs"]["en"].title},
		{"gate.nabuxai.com", "/ar/docs", "ar", "rtl", "https://gate.nabuxai.com/ar/docs", seoCopy["docs"]["ar"].title},
	}
	for _, c := range cases {
		resp, body := get(t, site, c.path, on(c.host))
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s%s = %d, want 200", c.host, c.path, resp.StatusCode)
		}
		want := []string{
			`<html lang="` + c.lang + `" dir="` + c.dir + `"`,
			`<title>` + c.title + `</title>`,
			`<link rel="canonical" href="` + c.canonical + `" />`,
			`<meta property="og:url" content="` + c.canonical + `" />`,
			`<meta name="twitter:card" content="summary_large_image" />`,
			`/intro/nabugate-` + c.lang + `.jpg" />`,
		}
		for _, w := range want {
			if !strings.Contains(body, w) {
				t.Fatalf("%s%s lacks %q:\n%s", c.host, c.path, w, body)
			}
		}
		if n := strings.Count(body, "<title>"); n != 1 {
			t.Fatalf("%s%s has %d titles, want 1", c.host, c.path, n)
		}
		if strings.Contains(body, `content="placeholder"`) {
			t.Fatalf("%s%s still carries the build's placeholder description", c.host, c.path)
		}
		kind := "home"
		if strings.HasSuffix(strings.TrimSuffix(c.path, "/"), "docs") {
			kind = "docs"
		}
		for _, l := range publicLangs {
			alt := `<link rel="alternate" hreflang="` + l + `" href="` + pageURL(kind, l, "https://gate.nabuxai.ir", "https://gate.nabuxai.com") + `" />`
			if !strings.Contains(body, alt) {
				t.Fatalf("%s%s lacks the %s alternate %q", c.host, c.path, l, alt)
			}
		}
		if !strings.Contains(body, `hreflang="x-default"`) {
			t.Fatalf("%s%s has no x-default alternate", c.host, c.path)
		}
	}
}

// The structured data is valid JSON and names the product.
func TestPublicPageStructuredData(t *testing.T) {
	_, body := get(t, seoSite(t), "/en", on("gate.nabuxai.com"))
	m := regexp.MustCompile(`<script type="application/ld\+json">(.*?)</script>`).FindStringSubmatch(body)
	if m == nil {
		t.Fatal("no JSON-LD on the home page")
	}
	var ld struct {
		Graph []map[string]any `json:"@graph"`
	}
	if err := json.Unmarshal([]byte(m[1]), &ld); err != nil {
		t.Fatalf("JSON-LD does not parse: %v\n%s", err, m[1])
	}
	types := map[string]bool{}
	for _, node := range ld.Graph {
		types[node["@type"].(string)] = true
		if node["name"] != "NabuGate" {
			t.Fatalf("node %v is not named NabuGate", node)
		}
	}
	if !types["WebSite"] || !types["SoftwareApplication"] {
		t.Fatalf("JSON-LD types = %v, want WebSite and SoftwareApplication", types)
	}
}

// Off the two NabuGate hosts the deployment is self-hosted: its pages are
// canonical on its own origin, never on ours.
func TestSelfHostedCanonicalStaysOnItsOwnHost(t *testing.T) {
	site := seoSite(t)
	for path, canonical := range map[string]string{
		"/":        "http://example.com/",
		"/en":      "http://example.com/en",
		"/ar/docs": "http://example.com/ar/docs",
	} {
		_, body := get(t, site, path, nil)
		if !strings.Contains(body, `<link rel="canonical" href="`+canonical+`" />`) {
			t.Fatalf("self-hosted %s: canonical is not %s:\n%s", path, canonical, body)
		}
		if strings.Contains(body, "nabuxai") {
			t.Fatalf("self-hosted %s points at a NabuGate host", path)
		}
	}
}

// Anything that is not a page, a file or a console view is a real 404, so a
// search engine stops counting every typo as a copy of the home page. The body
// is still the app, so a visitor is not stranded.
func TestUnknownPathsAreNotFound(t *testing.T) {
	site := seoSite(t)
	for _, p := range []string{"/nope", "/docs/nope", "/fa/nope", "/en/docs/x", "/wp-login.php", "/sitemap_index.xml"} {
		resp, body := get(t, site, p, on("gate.nabuxai.com"))
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("GET %s = %d, want 404", p, resp.StatusCode)
		}
		if !strings.Contains(body, `id="root"`) {
			t.Fatalf("GET %s: 404 body is not the app shell", p)
		}
		if resp.Header.Get("X-Robots-Tag") != "noindex" {
			t.Fatalf("GET %s: 404 lacks X-Robots-Tag: noindex", p)
		}
	}

	// Files of the bundle are still files.
	if resp, body := get(t, site, "/intro/nabugate-fa.jpg", nil); resp.StatusCode != http.StatusOK || body != "JPEG" {
		t.Fatalf("GET /intro/nabugate-fa.jpg = %d %q, want the image", resp.StatusCode, body)
	}

	// A console view opened at the root is the app, but not a page to index.
	resp, _ := get(t, site, "/tokens", nil)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("X-Robots-Tag") != "noindex" {
		t.Fatalf("GET /tokens = %d (X-Robots-Tag %q), want 200 noindex", resp.StatusCode, resp.Header.Get("X-Robots-Tag"))
	}
}

// Under /admin/ and /panel/ every path is a console route: the shell with a
// 200, kept out of search results.
func TestConsoleShellIsNotIndexed(t *testing.T) {
	console := seoSite(t).console()
	for _, p := range []string{"/", "/login", "/does/not/exist"} {
		resp, body := get(t, console, p, nil)
		if resp.StatusCode != http.StatusOK || !strings.Contains(body, `id="root"`) {
			t.Fatalf("console %s = %d, want the shell", p, resp.StatusCode)
		}
		if resp.Header.Get("X-Robots-Tag") != "noindex, nofollow" {
			t.Fatalf("console %s: X-Robots-Tag = %q, want noindex, nofollow", p, resp.Header.Get("X-Robots-Tag"))
		}
		if strings.Contains(body, `rel="canonical"`) {
			t.Fatalf("console %s carries the public page's head", p)
		}
	}
	if resp, _ := get(t, console, "/assets/app-oldhash.js", nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("console missing asset = %d, want 404", resp.StatusCode)
	}
}

func TestRobotsPointsAtThisHostsSitemap(t *testing.T) {
	site := seoSite(t)
	for host, want := range map[string]string{
		"gate.nabuxai.ir":  "Sitemap: https://gate.nabuxai.ir/sitemap.xml",
		"gate.nabuxai.com": "Sitemap: https://gate.nabuxai.com/sitemap.xml",
	} {
		resp, body := get(t, site, "/robots.txt", on(host))
		if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain") {
			t.Fatalf("%s/robots.txt = %d %s", host, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
		if !strings.Contains(body, want) || !strings.Contains(body, "User-agent: *") {
			t.Fatalf("%s/robots.txt:\n%s", host, body)
		}
		if strings.Contains(body, "Disallow: /panel") || strings.Contains(body, "Disallow: /admin") {
			t.Fatalf("%s/robots.txt blocks the console, so its noindex would never be read", host)
		}
	}
}

// A sitemap lists only URLs on its own host: Persian pages on .ir, English
// and Arabic on .com.
func TestSitemapListsThisHostsPages(t *testing.T) {
	site := seoSite(t)
	cases := map[string][]string{
		"gate.nabuxai.ir":  {"https://gate.nabuxai.ir/", "https://gate.nabuxai.ir/docs"},
		"gate.nabuxai.com": {"https://gate.nabuxai.com/en", "https://gate.nabuxai.com/ar", "https://gate.nabuxai.com/en/docs", "https://gate.nabuxai.com/ar/docs"},
	}
	for host, want := range cases {
		resp, body := get(t, site, "/sitemap.xml", on(host))
		if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/xml") {
			t.Fatalf("%s/sitemap.xml = %d %s", host, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
		var doc struct {
			URLs []struct {
				Loc string `xml:"loc"`
			} `xml:"url"`
		}
		if err := xml.Unmarshal([]byte(body), &doc); err != nil {
			t.Fatalf("%s/sitemap.xml is not XML: %v", host, err)
		}
		var got []string
		for _, u := range doc.URLs {
			got = append(got, u.Loc)
		}
		sort.Strings(got)
		sort.Strings(want)
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Fatalf("%s/sitemap.xml lists %v, want %v", host, got, want)
		}
		if !strings.Contains(body, `hreflang="x-default"`) {
			t.Fatalf("%s/sitemap.xml has no alternates", host)
		}
	}
}

// The server answers the console's views at the root with a 200; the list has
// to follow the sidebar, or a new view would start answering 404.
func TestConsoleViewsMatchTheSidebar(t *testing.T) {
	src, err := os.ReadFile("../../web/src/nav.js")
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, m := range regexp.MustCompile(`id: '([a-z0-9-]+)'`).FindAllStringSubmatch(string(src), -1) {
		if m[1] != "docs" { // docs is a public page, answered as one
			ids[m[1]] = true
		}
	}
	if len(ids) == 0 {
		t.Fatal("found no view ids in web/src/nav.js")
	}
	for id := range ids {
		if !consoleViews[id] {
			t.Errorf("nav.js has view %q that consoleViews lacks: /%s would answer 404", id, id)
		}
	}
	for id := range consoleViews {
		if !ids[id] {
			t.Errorf("consoleViews has %q, which nav.js no longer has", id)
		}
	}
}

// The rewrite works on the index.html the build actually ships, not only on
// the fixture above.
func TestBuiltShellGetsTheHead(t *testing.T) {
	assets, ok := web.Assets()
	if !ok {
		t.Skip("binary built without the web bundle")
	}
	site, err := newStaticSite(assets)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("NABU_PUBLIC_URL", "")
	_, body := get(t, site, "/en", on("gate.nabuxai.com"))
	for _, w := range []string{
		`<html lang="en" dir="ltr"`,
		`<title>` + seoCopy["home"]["en"].title + `</title>`,
		`<link rel="canonical" href="https://gate.nabuxai.com/en" />`,
	} {
		if !strings.Contains(body, w) {
			t.Fatalf("built shell lacks %q", w)
		}
	}
	if n := strings.Count(body, `<meta name="description"`); n != 1 {
		t.Fatalf("built shell has %d descriptions, want 1", n)
	}
}
