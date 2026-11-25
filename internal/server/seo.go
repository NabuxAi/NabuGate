package server

import (
	"encoding/json"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// The public pages (home and docs, in Persian, English and Arabic) are one
// app shell, index.html. Served as it was built, every one of them had the
// same `<title>NabuGate</title>`, no canonical, no hreflang and no preview
// image, and both hostnames served the same page — so a search engine saw one
// thin page duplicated across two domains and three languages, and a link
// pasted into a chat showed no card. The server knows the URL before any
// script runs, so it writes the head for that URL itself; crawlers and link
// previews that never run JavaScript get it too.
//
// Each language has one home host: Persian on gate.nabuxai.ir (where its
// readers are, and what the Persian articles link to), English and Arabic on
// gate.nabuxai.com. Both hosts still serve every language — a canonical only
// tells a search engine which copy to list. A deployment on any other host is
// self-hosted, and points every canonical at itself.

// publicLangs are the languages with a public page, in sitemap order.
var publicLangs = []string{"fa", "en", "ar"}

// seoPage is one public URL: which page, in which language.
type seoPage struct {
	kind string // "home" or "docs"
	lang string
}

type seoText struct{ title, description string }

// seoCopy is each page's title and description. App.jsx sets the same titles
// once React is running (search engines read the title after scripts run);
// change one and change the other.
var seoCopy = map[string]map[string]seoText{
	"home": {
		"fa": {
			title:       "نبوگیت — API هوش مصنوعی در ایران با یک کلید",
			description: "یک API سازگار با OpenAI برای GPT، Claude، Gemini و مدل‌های ایرانی: یک کلید، پرداخت ریالی، fallback خودکار بین سرویس‌ها و گزارش هزینهٔ هر پروژه. بدون تحریم‌شکن و بدون کارت ارزی.",
		},
		"en": {
			title:       "NabuGate — OpenAI-compatible AI API gateway with fallback",
			description: "One OpenAI-compatible API for GPT, Claude, Gemini and more: one key, automatic fallback across vendors, per-project quotas and cost tracking. Works with every OpenAI SDK, Cursor and Claude Code.",
		},
		"ar": {
			title:       "نابوغيت — بوابة API للذكاء الاصطناعي متوافقة مع OpenAI",
			description: "واجهة API واحدة متوافقة مع OpenAI لنماذج GPT وClaude وGemini وغيرها: مفتاح واحد، تحويل تلقائي بين المزوّدين عند التعطّل، وحصص وتكاليف لكل مشروع.",
		},
	},
	"docs": {
		"fa": {
			title:       "مستندات نبوگیت — اتصال Cursor، Claude Code و SDKها",
			description: "راهنمای قدم‌به‌قدم اتصال Cursor، Claude Code، Cline، Codex، SDK پایتون و Node و LangChain به نبوگیت؛ مدل‌ها و aliasها، خطاها، کلیدها و پرداخت.",
		},
		"en": {
			title:       "NabuGate Docs — connect Cursor, Claude Code and SDKs",
			description: "Step-by-step guides for connecting Cursor, Claude Code, Cline, Codex, the Python and Node SDKs and LangChain to NabuGate, plus models and aliases, errors, keys and billing.",
		},
		"ar": {
			title:       "توثيق نابوغيت — ربط Cursor وClaude Code وحزم SDK",
			description: "أدلة خطوة بخطوة لربط Cursor وClaude Code وCline وCodex وحزم SDK للغتي Python وNode وLangChain بنابوغيت، مع النماذج والأسماء المستعارة والأخطاء والمفاتيح والدفع.",
		},
	},
}

var ogLocale = map[string]string{"fa": "fa_IR", "en": "en_US", "ar": "ar_AR"}

// publicPage reports which public page a cleaned path (no leading slash) is.
// A language prefix decides the language; without one, ?lang= does; without
// that, Persian — the same order the pre-paint script in index.html uses.
func publicPage(p string, query url.Values) (seoPage, bool) {
	lang := ""
	rest := p
	for _, l := range publicLangs {
		if p == l || strings.HasPrefix(p, l+"/") {
			lang, rest = l, strings.TrimPrefix(strings.TrimPrefix(p, l), "/")
			break
		}
	}
	kind := ""
	switch rest {
	case "", "index.html":
		kind = "home"
	case "docs":
		kind = "docs"
	default:
		return seoPage{}, false
	}
	if lang == "" {
		lang = "fa"
		if q := query.Get("lang"); q == "en" || q == "ar" || q == "fa" {
			lang = q
		}
	}
	return seoPage{kind: kind, lang: lang}, true
}

// siteOrigins returns the origin each language is canonical on, for the host
// this request reached. On either NabuGate host that is the split described
// above; anywhere else both are the request's own origin.
func siteOrigins(r *http.Request) (fa, intl string) {
	base := publicBase(r)
	fa = strings.TrimRight(envOrDefault("NABU_SITE_URL_FA", "https://gate.nabuxai.ir"), "/")
	intl = strings.TrimRight(envOrDefault("NABU_SITE_URL", "https://gate.nabuxai.com"), "/")
	host := originHost(base)
	if host != "" && (strings.EqualFold(host, originHost(fa)) || strings.EqualFold(host, originHost(intl))) {
		return fa, intl
	}
	return base, base
}

func originHost(origin string) string {
	u, err := url.Parse(origin)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// pageURL is the canonical address of a page.
func pageURL(kind, lang, faOrigin, intlOrigin string) string {
	suffix := ""
	if kind == "docs" {
		suffix = "docs"
	}
	if lang == "fa" {
		// Persian is the default language, so its pages need no prefix.
		return faOrigin + "/" + suffix
	}
	if suffix != "" {
		suffix = "/" + suffix
	}
	return intlOrigin + "/" + lang + suffix
}

// seoHead renders everything the head needs for one page.
func seoHead(pg seoPage, faOrigin, intlOrigin string) string {
	text := seoCopy[pg.kind][pg.lang]
	canonical := pageURL(pg.kind, pg.lang, faOrigin, intlOrigin)
	// The language's intro poster (1280×720), from the page's own host.
	image := originOf(canonical) + "/intro/nabugate-" + pg.lang + ".jpg"
	attr := html.EscapeString

	var b strings.Builder
	b.WriteString("<title>" + attr(text.title) + "</title>\n")
	b.WriteString(`    <meta name="description" content="` + attr(text.description) + "\" />\n")
	b.WriteString(`    <link rel="canonical" href="` + attr(canonical) + "\" />\n")
	for _, l := range publicLangs {
		b.WriteString(`    <link rel="alternate" hreflang="` + l + `" href="` + attr(pageURL(pg.kind, l, faOrigin, intlOrigin)) + "\" />\n")
	}
	b.WriteString(`    <link rel="alternate" hreflang="x-default" href="` + attr(pageURL(pg.kind, "en", faOrigin, intlOrigin)) + "\" />\n")
	meta := func(attrName, name, content string) {
		b.WriteString(`    <meta ` + attrName + `="` + name + `" content="` + attr(content) + "\" />\n")
	}
	meta("property", "og:type", "website")
	meta("property", "og:site_name", "NabuGate")
	meta("property", "og:locale", ogLocale[pg.lang])
	meta("property", "og:title", text.title)
	meta("property", "og:description", text.description)
	meta("property", "og:url", canonical)
	meta("property", "og:image", image)
	meta("property", "og:image:width", "1280")
	meta("property", "og:image:height", "720")
	meta("property", "og:image:alt", text.title)
	meta("name", "twitter:card", "summary_large_image")
	meta("name", "twitter:title", text.title)
	meta("name", "twitter:description", text.description)
	meta("name", "twitter:image", image)

	publisher := map[string]any{"@type": "Organization", "name": "NabuxAI"}
	ld := map[string]any{
		"@context": "https://schema.org",
		"@graph": []any{
			map[string]any{
				"@type":      "WebSite",
				"name":       "NabuGate",
				"url":        pageURL("home", pg.lang, faOrigin, intlOrigin),
				"inLanguage": pg.lang,
				"publisher":  publisher,
			},
			map[string]any{
				"@type":               "SoftwareApplication",
				"name":                "NabuGate",
				"applicationCategory": "DeveloperApplication",
				"operatingSystem":     "Web",
				"url":                 canonical,
				"description":         text.description,
				"inLanguage":          pg.lang,
				"image":               image,
				"publisher":           publisher,
			},
		},
	}
	// encoding/json escapes <, > and &, so nothing in the text can close the
	// script element.
	js, _ := json.Marshal(ld)
	b.WriteString(`    <script type="application/ld+json">` + string(js) + "</script>")
	return b.String()
}

func originOf(u string) string {
	p, err := url.Parse(u)
	if err != nil {
		return u
	}
	return p.Scheme + "://" + p.Host
}

var (
	titleRe       = regexp.MustCompile(`(?s)<title>.*?</title>`)
	descriptionRe = regexp.MustCompile(`\s*<meta name="description"[^>]*>`)
	htmlLangRe    = regexp.MustCompile(`<html lang="[a-z]+" dir="(?:rtl|ltr)"`)
)

// renderShell writes a page's head into the app shell. The built index.html
// carries a placeholder title and description; both are replaced, and the
// html element gets the page's language, which the pre-paint script would
// otherwise only set for visitors who run it.
func renderShell(shell []byte, pg seoPage, faOrigin, intlOrigin string) []byte {
	s := string(shell)
	if !titleRe.MatchString(s) {
		return shell
	}
	head := seoHead(pg, faOrigin, intlOrigin)
	s = descriptionRe.ReplaceAllString(s, "")
	s = titleRe.ReplaceAllLiteralString(s, head)
	dir := "rtl"
	if pg.lang == "en" {
		dir = "ltr"
	}
	s = htmlLangRe.ReplaceAllLiteralString(s, `<html lang="`+pg.lang+`" dir="`+dir+`"`)
	return []byte(s)
}

// robotsTxt allows the public site and points at this host's sitemap. The
// console (/admin, /panel) is not disallowed: a disallowed URL can still be
// listed from links to it, only without content. Its shell answers with
// X-Robots-Tag: noindex instead, which keeps it out.
func robotsTxt(r *http.Request) string {
	return "User-agent: *\n" +
		"Allow: /\n" +
		"Disallow: /api/\n" +
		"Disallow: /v1/\n" +
		"\n" +
		"Sitemap: " + publicBase(r) + "/sitemap.xml\n"
}

// sitemapXML lists the public pages whose canonical address is on the host
// this request reached — a sitemap may only list URLs on its own host — with
// every language version as an alternate.
func sitemapXML(r *http.Request) string {
	faOrigin, intlOrigin := siteOrigins(r)
	host := originHost(publicBase(r))
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">` + "\n")
	for _, kind := range []string{"home", "docs"} {
		for _, lang := range publicLangs {
			loc := pageURL(kind, lang, faOrigin, intlOrigin)
			if !strings.EqualFold(originHost(loc), host) {
				continue
			}
			b.WriteString("  <url>\n    <loc>" + html.EscapeString(loc) + "</loc>\n")
			for _, alt := range publicLangs {
				b.WriteString(`    <xhtml:link rel="alternate" hreflang="` + alt + `" href="` + html.EscapeString(pageURL(kind, alt, faOrigin, intlOrigin)) + "\"/>\n")
			}
			b.WriteString(`    <xhtml:link rel="alternate" hreflang="x-default" href="` + html.EscapeString(pageURL(kind, "en", faOrigin, intlOrigin)) + "\"/>\n")
			b.WriteString("  </url>\n")
		}
	}
	b.WriteString("</urlset>\n")
	return b.String()
}

// consoleViews are the console's views, which the app also opens from a bare
// /<view> path. They answer with the shell, never indexed. The list mirrors
// web/src/nav.js; TestConsoleViewsMatchTheSidebar keeps the two in step.
var consoleViews = map[string]bool{
	"dashboard": true, "account": true, "plans": true, "payments": true,
	"tokens": true, "providers": true, "models": true, "decisions": true,
	"requests": true, "integration": true, "profile": true, "security": true,
	"usage": true, "users": true, "provider-requests": true, "agents": true,
	"keys": true, "opsless": true, "templates": true,
}
