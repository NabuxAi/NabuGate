package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"nabugate/internal/config"
	"nabugate/internal/provider"
	"nabugate/internal/usage"
)

// The landing page's live demo: a visitor types a prompt and a real model
// answers through the real fallback chain, with the route, latency and cost
// shown. It is the one thing on the page a visitor can do rather than read,
// and the most direct answer to "does this work" a gateway can give.
//
// It runs on the gateway's own credentials, so it is fenced on every side:
//   - one alias (NABU_DEMO_ALIAS, nabu-fast by default), never a caller's choice;
//   - a short prompt in and a short answer out;
//   - a few tries per visitor per hour, and a few in flight at once;
//   - a daily spend cap in dollars (NABU_DEMO_DAILY_USD, $2 by default; 0 turns
//     the demo off). Spend is the vendor's list price for what was served.
//
// The counters live in memory. A restart forgets today's spend, so a deploy
// late in a day can let that day reach at most twice the cap; that bounded
// overrun is cheaper than a disk write on every try. Every try is still
// metered under the project "landing-demo", so the console shows what it cost.

const (
	demoProject       = "landing-demo"
	demoMaxPromptRune = 400
	demoMaxTokens     = 320
	demoTriesPerHour  = 5
	demoMaxInFlight   = 4
	demoTimeout       = 30 * time.Second
)

type demoGate struct {
	mu       sync.Mutex
	day      string
	spentUSD float64
	inFlight int
	tries    map[string][]time.Time // client IP → try times within the last hour
}

func newDemoGate() *demoGate { return &demoGate{tries: map[string][]time.Time{}} }

func demoAlias() string { return envOrDefault("NABU_DEMO_ALIAS", "nabu-fast") }

// demoDailyCap is the day's budget in dollars; zero or less disables the demo.
func demoDailyCap() float64 {
	v := strings.TrimSpace(os.Getenv("NABU_DEMO_DAILY_USD"))
	if v == "" {
		return 2
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 2
	}
	return f
}

// admit reserves a try for ip, or says why not. remaining is how many tries the
// visitor has left this hour after this one.
func (g *demoGate) admit(ip string, now time.Time, dailyCap float64) (code string, remaining int, retryAfter time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rollDay(now)
	if g.spentUSD >= dailyCap {
		return "demo_budget", 0, 0
	}
	recent := g.recent(ip, now)
	if len(recent) >= demoTriesPerHour {
		return "demo_rate_limited", 0, recent[0].Add(time.Hour).Sub(now)
	}
	if g.inFlight >= demoMaxInFlight {
		return "demo_busy", demoTriesPerHour - len(recent), 5 * time.Second
	}
	g.tries[ip] = append(recent, now)
	g.inFlight++
	// Keep the map from growing with every visitor who ever came by.
	if len(g.tries) > 10000 {
		for k, v := range g.tries {
			if len(g.recent(k, now)) == 0 || len(v) == 0 {
				delete(g.tries, k)
			}
		}
	}
	return "", demoTriesPerHour - len(recent) - 1, 0
}

// done releases the in-flight slot and books what the try cost.
func (g *demoGate) done(now time.Time, costUSD float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rollDay(now)
	g.inFlight--
	g.spentUSD += costUSD
}

// refund gives a visitor back a try that produced nothing, so an outage does
// not spend their hour.
func (g *demoGate) refund(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if t := g.tries[ip]; len(t) > 0 {
		g.tries[ip] = t[:len(t)-1]
	}
}

func (g *demoGate) recent(ip string, now time.Time) []time.Time {
	kept := g.tries[ip][:0]
	for _, t := range g.tries[ip] {
		if now.Sub(t) < time.Hour {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(g.tries, ip)
		return nil
	}
	g.tries[ip] = kept
	return kept
}

func (g *demoGate) rollDay(now time.Time) {
	if d := now.UTC().Format("2006-01-02"); d != g.day {
		g.day, g.spentUSD = d, 0
	}
}

type demoRequest struct {
	Prompt string `json:"prompt"`
	Lang   string `json:"lang"`
}

var demoSystem = map[string]string{
	"fa": "تو دموی زندهٔ صفحهٔ نبوگیت هستی. کوتاه، دقیق و مفید جواب بده (حداکثر حدود ۱۲۰ کلمه)، به فارسی مگر کاربر به زبان دیگری بنویسد. اگر چیزی دربارهٔ نبوگیت پرسیده شد: یک API سازگار با OpenAI که GPT، Claude، Gemini و مدل‌های دیگر را با یک کلید و fallback خودکار می‌دهد.",
	"en": "You are the live demo on NabuGate's landing page. Answer briefly, precisely and helpfully (about 120 words at most), in the user's language. If asked about NabuGate: it is one OpenAI-compatible API for GPT, Claude, Gemini and other models, with one key and automatic fallback across vendors.",
	"ar": "أنت العرض الحيّ في صفحة نابوغيت. أجب باختصار ودقة (نحو ١٢٠ كلمة كحدّ أقصى)، بلغة المستخدم. إن سُئلت عن نابوغيت: واجهة API واحدة متوافقة مع OpenAI لنماذج GPT وClaude وGemini وغيرها، بمفتاح واحد وتحويل تلقائي بين المزوّدين.",
}

// handleDemoTry — POST /api/public/try.
func (s *Server) handleDemoTry(w http.ResponseWriter, r *http.Request) {
	dailyCap := demoDailyCap()
	alias := demoAlias()
	if dailyCap <= 0 || s.demo == nil || !s.router.KnowsAlias(alias) {
		writeErrorCode(w, http.StatusServiceUnavailable, "demo_disabled", "the live demo is not available on this deployment", nil)
		return
	}
	var body demoRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<13)).Decode(&body); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "bad_prompt", "invalid JSON body", nil)
		return
	}
	prompt := strings.TrimSpace(body.Prompt)
	if prompt == "" || utf8.RuneCountInString(prompt) > demoMaxPromptRune {
		writeErrorCode(w, http.StatusBadRequest, "bad_prompt", "prompt must be 1 to 400 characters", map[string]any{"max_chars": demoMaxPromptRune})
		return
	}
	system, ok := demoSystem[body.Lang]
	if !ok {
		system = demoSystem["fa"]
	}

	ip := clientIP(r)
	now := time.Now()
	code, remaining, retry := s.demo.admit(ip, now, dailyCap)
	if code != "" {
		extra := map[string]any{}
		if retry > 0 {
			secs := int(retry.Seconds()) + 1
			w.Header().Set("Retry-After", strconv.Itoa(secs))
			extra["retry_after_seconds"] = secs
		}
		msg := map[string]string{
			"demo_budget":       "today's demo budget is used up; it resets at 00:00 UTC",
			"demo_rate_limited": "five tries per hour; create a free account to keep going",
			"demo_busy":         "the demo is busy; try again in a few seconds",
		}[code]
		writeErrorCode(w, http.StatusTooManyRequests, code, msg, extra)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), demoTimeout)
	defer cancel()
	maxTokens := demoMaxTokens
	temperature := 0.6
	started := time.Now()
	res, err := s.router.Chat(ctx, alias, provider.ChatRequest{
		Messages: []provider.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: prompt},
		},
		MaxTokens:   &maxTokens,
		Temperature: &temperature,
	})
	latency := time.Since(started)
	if err != nil {
		s.demo.done(time.Now(), 0)
		s.demo.refund(ip)
		s.log.Warn("demo try failed", "alias", alias, "error", err.Error())
		writeErrorCode(w, http.StatusBadGateway, "upstream_failed", "every model behind the demo failed; try again shortly", nil)
		return
	}
	u := res.Response.Usage
	list, _ := s.usage.PriceOf(res.Provider, res.Model, usage.Tokens(u))
	s.demo.done(time.Now(), list)
	s.recordForProject(r, demoProject, res.Provider, res.Model, u)

	writeJSON(w, http.StatusOK, map[string]any{
		"text":              res.Response.Content,
		"alias":             alias,
		"provider":          res.Provider,
		"model":             res.Model,
		"chain":             s.demoChain(alias),
		"latency_ms":        latency.Milliseconds(),
		"prompt_tokens":     u.PromptTokens,
		"completion_tokens": u.CompletionTokens,
		"cost_usd":          list,
		"remaining":         remaining,
	})
}

// demoChain is the alias's configured models in fallback order, so the page
// can show where the answer came from in the chain.
func (s *Server) demoChain(alias string) []string {
	_, route := s.router.RouteKind(alias)
	var out []string
	seen := map[string]bool{}
	for _, t := range append([]config.Target{route.Primary}, route.Fallback...) {
		name := t.Model
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
		if len(out) == 6 {
			break
		}
	}
	return out
}
