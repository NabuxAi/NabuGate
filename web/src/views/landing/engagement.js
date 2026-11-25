import { useEffect } from 'react';

/*
 * Time on page, measured by the page itself and reported to the gateway
 * (POST /api/public/engagement, internal/server/engagement.go).
 *
 * Only visible time counts: a tab left open in the background is not a reader.
 * The report goes out when the view ends — the tab is hidden or the page is
 * left — through sendBeacon, which the browser delivers even as the page
 * unloads. A visitor who comes back to the tab starts a new view, so no time
 * is ever reported twice.
 *
 * Nothing identifies the visitor; the server keeps daily counters only.
 */

// Landing sections worth knowing whether people reach, keyed by the selector
// that finds each one. The server accepts only these names.
const SECTIONS = {
  try: '#try',
  stats: '.lp-stats-band',
  'iran-claude': '#iran-claude',
  'claude-advisor': '#claude-advisor',
  features: '#features',
  how: '#how',
  developers: '#developers',
  pricing: '#pricing',
  faq: '#faq',
  cta: '.lp-cta-wrap',
  'docs-article': '.docs-body',
};

const flags = { tried: false, cta: false };

// markEngagement records something a visitor did on this view: 'tried' (used
// the live demo) or 'cta' (clicked a sign-up button).
export function markEngagement(what) {
  if (what in flags) flags[what] = true;
}

function send(payload) {
  try {
    const body = JSON.stringify(payload);
    if (navigator.sendBeacon && navigator.sendBeacon('/api/public/engagement', new Blob([body], { type: 'application/json' }))) return;
    fetch('/api/public/engagement', { method: 'POST', body, headers: { 'Content-Type': 'application/json' }, keepalive: true }).catch(() => {});
  } catch {
    /* measurement must never break the page */
  }
}

export function useEngagement(page, lang) {
  useEffect(() => {
    if (!page || typeof window === 'undefined' || typeof document === 'undefined') return undefined;

    let visibleMs = 0;
    let since = document.visibilityState === 'visible' ? performance.now() : null;
    let maxScroll = 0;
    const seen = new Set();
    flags.tried = false;
    flags.cta = false;

    const scrollPct = () => {
      const doc = document.documentElement;
      const reachable = doc.scrollHeight - window.innerHeight;
      const pct = reachable <= 0 ? 100 : ((window.scrollY + 1) / reachable) * 100;
      maxScroll = Math.max(maxScroll, Math.min(100, Math.round(pct)));
    };
    scrollPct();
    window.addEventListener('scroll', scrollPct, { passive: true });

    // A section counts as seen once a fifth of it has been on screen.
    let io = null;
    const observe = () => {
      if (typeof IntersectionObserver === 'undefined') return;
      io?.disconnect();
      io = new IntersectionObserver((entries) => entries.forEach((e) => {
        if (e.isIntersecting) { seen.add(e.target.dataset.engage); io.unobserve(e.target); }
      }), { threshold: 0.2 });
      for (const [name, sel] of Object.entries(SECTIONS)) {
        if (seen.has(name)) continue;
        const el = document.querySelector(sel);
        if (el) { el.dataset.engage = name; io.observe(el); }
      }
    };
    observe();
    // Sections that render late (the docs article, lazy parts) get picked up.
    const late = window.setTimeout(observe, 2500);

    // Any sign-up or console link counts as a call to action taken.
    const onClick = (e) => {
      const a = e.target.closest?.('a[href]');
      if (a && /\/panel(\/|$)/.test(a.getAttribute('href') || '')) flags.cta = true;
    };
    document.addEventListener('click', onClick, true);

    let sent = false;
    const flush = () => {
      if (since !== null) { visibleMs += performance.now() - since; since = null; }
      if (sent || visibleMs < 1000) return;
      sent = true;
      send({
        page,
        lang,
        seconds: Math.round(visibleMs / 1000),
        scroll: maxScroll,
        sections: [...seen],
        tried: flags.tried,
        cta: flags.cta,
      });
    };
    const onVisibility = () => {
      if (document.visibilityState === 'hidden') {
        flush();
      } else if (since === null) {
        // Back on the tab: a new view starts.
        if (sent) { visibleMs = 0; sent = false; flags.tried = false; flags.cta = false; }
        since = performance.now();
      }
    };
    document.addEventListener('visibilitychange', onVisibility);
    window.addEventListener('pagehide', flush);

    return () => {
      flush();
      window.clearTimeout(late);
      io?.disconnect();
      window.removeEventListener('scroll', scrollPct);
      document.removeEventListener('click', onClick, true);
      document.removeEventListener('visibilitychange', onVisibility);
      window.removeEventListener('pagehide', flush);
    };
  }, [page, lang]);
}
