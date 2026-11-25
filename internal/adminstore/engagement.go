package adminstore

import (
	"sort"
	"time"
)

// Engagement is how long visitors stay on the public pages and how far they
// get, aggregated per UTC day and page. Nothing about a single visitor is kept:
// no address, no identifier, no event stream — only counters, so the numbers
// can answer "did the change keep people longer" without the store becoming a
// record of who read what.
type EngagementDay struct {
	Views   int64 `json:"views"`
	Seconds int64 `json:"seconds"`
	// Time is the number of views whose visible time fell in each bucket:
	// under 10 s, 10–30 s, 30–60 s, 1–3 min, 3 min and over.
	Time [5]int64 `json:"time"`
	// Scroll counts views that reached 25, 50, 75 and 100 percent of the page.
	Scroll   [4]int64         `json:"scroll"`
	Sections map[string]int64 `json:"sections,omitempty"`
	Langs    map[string]int64 `json:"langs,omitempty"`
	Tries    int64            `json:"tries"` // views that used the live demo
	CTA      int64            `json:"cta"`   // views that clicked a sign-up button
}

// EngagementView is one page view as the page reported it on leaving.
type EngagementView struct {
	Page     string
	Lang     string
	Seconds  int
	Scroll   int // 0–100
	Sections []string
	Tried    bool
	CTA      bool
}

// EngagementReportDay is one day of one page, for the console.
type EngagementReportDay struct {
	Day  string `json:"day"`
	Page string `json:"page"`
	EngagementDay
}

// engagementKeepDays bounds the history so the state file cannot grow without
// limit.
const engagementKeepDays = 90

var timeBuckets = [4]int{10, 30, 60, 180}

// RecordEngagement adds one view to its day's counters. Like usage, it rides
// the periodic flush rather than writing the file per view.
func (s *Store) RecordEngagement(now time.Time, v EngagementView) {
	day := now.UTC().Format("2006-01-02")
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.st.Engagement == nil {
		s.st.Engagement = map[string]map[string]*EngagementDay{}
	}
	pages := s.st.Engagement[day]
	if pages == nil {
		pages = map[string]*EngagementDay{}
		s.st.Engagement[day] = pages
		s.pruneEngagementLocked(now)
	}
	d := pages[v.Page]
	if d == nil {
		d = &EngagementDay{}
		pages[v.Page] = d
	}
	d.Views++
	d.Seconds += int64(v.Seconds)
	b := len(timeBuckets)
	for i, limit := range timeBuckets {
		if v.Seconds < limit {
			b = i
			break
		}
	}
	d.Time[b]++
	for i, mark := range []int{25, 50, 75, 100} {
		if v.Scroll >= mark {
			d.Scroll[i]++
		}
	}
	if len(v.Sections) > 0 && d.Sections == nil {
		d.Sections = map[string]int64{}
	}
	for _, sec := range v.Sections {
		d.Sections[sec]++
	}
	if d.Langs == nil {
		d.Langs = map[string]int64{}
	}
	d.Langs[v.Lang]++
	if v.Tried {
		d.Tries++
	}
	if v.CTA {
		d.CTA++
	}
	s.dirty = true
}

func (s *Store) pruneEngagementLocked(now time.Time) {
	cutoff := now.UTC().AddDate(0, 0, -engagementKeepDays).Format("2006-01-02")
	for day := range s.st.Engagement {
		if day < cutoff {
			delete(s.st.Engagement, day)
		}
	}
}

// EngagementReport returns the last days of counters, newest first.
func (s *Store) EngagementReport(now time.Time, days int) []EngagementReportDay {
	if days <= 0 || days > engagementKeepDays {
		days = 14
	}
	cutoff := now.UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []EngagementReportDay{}
	for day, pages := range s.st.Engagement {
		if day < cutoff {
			continue
		}
		for page, d := range pages {
			c := *d
			c.Sections = copyCounts(d.Sections)
			c.Langs = copyCounts(d.Langs)
			out = append(out, EngagementReportDay{Day: day, Page: page, EngagementDay: c})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Day != out[j].Day {
			return out[i].Day > out[j].Day
		}
		return out[i].Page < out[j].Page
	})
	return out
}

func copyCounts(m map[string]int64) map[string]int64 {
	if m == nil {
		return nil
	}
	out := make(map[string]int64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
