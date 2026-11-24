package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"nabugate/internal/adminstore"
)

// The public pages report, once as each view ends, how long the page was
// visible, how far it was scrolled, which sections were seen, and whether the
// live demo or a sign-up button was used. There was no measurement at all
// before this: "keep visitors longer" had nothing to be checked against.
//
// It is self-hosted on purpose. A third-party script would be another request
// the page waits on, another host filtered in Iran, and a copy of every visit
// handed to someone else. What is kept is a set of daily counters
// (adminstore/engagement.go) — no address and no identifier.

// engagementSections are the landing and docs sections the page may report;
// anything else is dropped, so the counters cannot be filled with junk keys.
var engagementSections = map[string]bool{
	"try": true, "stats": true, "iran-claude": true, "claude-advisor": true,
	"features": true, "how": true, "developers": true, "pricing": true,
	"faq": true, "cta": true, "docs-article": true,
}

const (
	beaconsPerHour      = 60
	engagementMaxSecond = 30 * 60
)

type beaconLimit struct {
	mu     sync.Mutex
	window time.Time
	counts map[string]int
}

func newBeaconLimit() *beaconLimit { return &beaconLimit{counts: map[string]int{}} }

// allow is a fixed hourly window per address: crude, and enough to stop one
// client from inflating the numbers.
func (b *beaconLimit) allow(ip string, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if now.Sub(b.window) >= time.Hour {
		b.window, b.counts = now, map[string]int{}
	}
	if b.counts[ip] >= beaconsPerHour {
		return false
	}
	b.counts[ip]++
	return true
}

type engagementBody struct {
	Page     string   `json:"page"`
	Lang     string   `json:"lang"`
	Seconds  float64  `json:"seconds"`
	Scroll   float64  `json:"scroll"`
	Sections []string `json:"sections"`
	Tried    bool     `json:"tried"`
	CTA      bool     `json:"cta"`
}

// looksLikeBot catches the crawlers that run scripts; they are not visitors.
func looksLikeBot(ua string) bool {
	ua = strings.ToLower(ua)
	if ua == "" {
		return true
	}
	for _, s := range []string{"bot", "crawl", "spider", "slurp", "headless", "lighthouse", "preview", "facebookexternalhit"} {
		if strings.Contains(ua, s) {
			return true
		}
	}
	return false
}

// handleEngagement — POST /api/public/engagement. Always 204: a beacon has no
// one to show an error to.
func (s *Server) handleEngagement(w http.ResponseWriter, r *http.Request) {
	defer w.WriteHeader(http.StatusNoContent)
	if s.admin == nil || s.beacons == nil || looksLikeBot(r.UserAgent()) {
		return
	}
	var body engagementBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body); err != nil {
		return
	}
	if body.Page != "home" && body.Page != "docs" {
		return
	}
	if !s.beacons.allow(clientIP(r), time.Now()) {
		return
	}
	lang := body.Lang
	if lang != "en" && lang != "ar" {
		lang = "fa"
	}
	secs := int(body.Seconds)
	if secs < 0 {
		secs = 0
	}
	if secs > engagementMaxSecond {
		secs = engagementMaxSecond
	}
	scroll := int(body.Scroll)
	if scroll < 0 {
		scroll = 0
	}
	if scroll > 100 {
		scroll = 100
	}
	var sections []string
	seen := map[string]bool{}
	for _, sec := range body.Sections {
		if engagementSections[sec] && !seen[sec] {
			seen[sec] = true
			sections = append(sections, sec)
		}
	}
	s.admin.RecordEngagement(time.Now(), adminstore.EngagementView{
		Page: body.Page, Lang: lang, Seconds: secs, Scroll: scroll,
		Sections: sections, Tried: body.Tried, CTA: body.CTA,
	})
}

// engagementReport — GET /api/engagement?days=N (administrators). Also says
// where today's demo budget stands, which is the other half of the question
// "is the demo worth what it costs".
func (s *Server) engagementReport(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	now := time.Now()
	demo := map[string]any{"alias": demoAlias(), "daily_cap_usd": demoDailyCap(), "spent_today_usd": 0.0}
	if s.demo != nil {
		s.demo.mu.Lock()
		s.demo.rollDay(now)
		demo["spent_today_usd"] = s.demo.spentUSD
		s.demo.mu.Unlock()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"days": s.admin.EngagementReport(now, days),
		"demo": demo,
	})
}
