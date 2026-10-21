package server

import (
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// canonicalPublicHosts are the browser-facing names of the hosted gateway.
// A reverse proxy in front of one of them (the Iran mirror at gate.nabuxai.ir
// is the case this exists for) sends that name in X-Forwarded-Host while the
// upstream Host stays gate.nabuxai.com, because that is what the TLS
// certificate and the vhost expect. On the Coolify mirror, Traefik terminates
// TLS and the internal nginx is what sets those two headers: Host is .com,
// X-Forwarded-Host is .ir, X-Forwarded-Proto is https. Those names are allowed
// to win over NABU_PUBLIC_URL so a payer, and an OAuth callback, return to the
// host the browser actually opened.
var canonicalPublicHosts = map[string]bool{
	"gate.nabuxai.com": true,
	"gate.nabuxai.ir":  true,
}

// firstHeaderValue returns the left-most entry of a comma-separated
// forwarding header. That entry is the client-facing one; later entries are
// proxies that appended themselves.
func firstHeaderValue(v string) string {
	if v == "" {
		return ""
	}
	return strings.TrimSpace(strings.Split(v, ",")[0])
}

// validForwardedHost reports whether v is a host or host:port and nothing
// else. A scheme, a path, a userinfo or whitespace would turn a payment
// callback or an OAuth redirect_uri into an open redirect.
func validForwardedHost(v string) bool {
	if v == "" || len(v) > 255 || strings.ContainsAny(v, " /\\@?#") || strings.Contains(v, "://") {
		return false
	}
	host := v
	if strings.HasPrefix(v, "[") {
		end := strings.LastIndex(v, "]")
		if end <= 1 {
			return false
		}
		host = v[1:end]
		rest := v[end+1:]
		if rest != "" && !validPort(strings.TrimPrefix(rest, ":")) {
			return false
		}
	} else if i := strings.LastIndex(v, ":"); i >= 0 {
		if !validPort(v[i+1:]) {
			return false
		}
		host = v[:i]
	}
	if host == "" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return true
	}
	return !strings.HasPrefix(host, ".") && !strings.HasSuffix(host, ".") && !strings.Contains(host, "..")
}

func validPort(p string) bool {
	n, err := strconv.Atoi(p)
	return err == nil && n > 0 && n < 65536
}

func hostName(hostport string) string {
	if strings.HasPrefix(hostport, "[") {
		if end := strings.LastIndex(hostport, "]"); end > 1 {
			return hostport[1:end]
		}
	}
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return hostport
}

// requestScheme is https when the connection is TLS or the proxy says the
// browser used https. The left-most X-Forwarded-Proto wins, same as the host.
func requestScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if strings.EqualFold(firstHeaderValue(r.Header.Get("X-Forwarded-Proto")), "https") {
		return "https"
	}
	return "http"
}

func requestIsHTTPS(r *http.Request) bool {
	return requestScheme(r) == "https"
}

// forwardedHostMayOverride reports whether X-Forwarded-Host may replace a
// pinned NABU_PUBLIC_URL. With nothing pinned, any syntactically valid host
// is the public host — that is how a self-hosted proxy works. With a pin, only
// the pinned host itself and the known public names may override it, so a
// caller who can reach the process directly cannot aim the callback at an
// arbitrary site.
func forwardedHostMayOverride(host, pinned string) bool {
	name := strings.ToLower(hostName(host))
	if canonicalPublicHosts[name] {
		return true
	}
	if pinned == "" {
		return true
	}
	u, err := url.Parse(pinned)
	return err == nil && strings.EqualFold(u.Hostname(), name)
}

// publicBase is the origin the caller's browser used: scheme plus host, no
// path. OAuth redirect URIs and payment return URLs are built from it, so a
// request that arrived through gate.nabuxai.ir goes back there.
func publicBase(r *http.Request) string {
	pinned := strings.TrimRight(os.Getenv("NABU_PUBLIC_URL"), "/")
	if host := firstHeaderValue(r.Header.Get("X-Forwarded-Host")); validForwardedHost(host) && forwardedHostMayOverride(host, pinned) {
		return requestScheme(r) + "://" + host
	}
	if pinned != "" {
		return pinned
	}
	return requestScheme(r) + "://" + r.Host
}

// browserCookie is a host-only cookie. Domain is left empty on purpose: a
// Domain attribute would pin the cookie to one site, and a browser that
// reached the gateway through gate.nabuxai.ir would never store or send a
// cookie scoped to gate.nabuxai.com. Secure follows the scheme the browser
// used, which behind the mirror is X-Forwarded-Proto rather than the
// connection to the upstream.
func browserCookie(r *http.Request, c http.Cookie) *http.Cookie {
	c.Path = "/"
	c.Domain = ""
	c.Secure = requestIsHTTPS(r)
	if c.SameSite == 0 {
		c.SameSite = http.SameSiteLaxMode
	}
	return &c
}
