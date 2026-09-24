import { createContext, useCallback, useContext, useMemo, useState } from 'react';

/*
 * Languages for the landing page, the docs and the console.
 *
 * Strings live next to the component that renders them, as a { fa, en } pair
 * passed to useT(). Keeping them co-located means a new screen never has to
 * touch a central catalogue, and a missing key falls back to Persian — the
 * language every string was first written in — instead of rendering blank.
 *
 * The formatters below (fmtInt, usd, fmtDate …) are plain functions rather than
 * hooks so they can be called anywhere, including outside render. They read the
 * active language from module state, which the provider sets before its
 * children render; a switch re-renders the whole tree from the root.
 */

export const LANGS = {
  fa: { code: 'fa', dir: 'rtl', name: 'فارسی', short: 'فا', locale: 'fa-IR' },
  en: { code: 'en', dir: 'ltr', name: 'English', short: 'EN', locale: 'en-US' },
};
export const DEFAULT_LANG = 'fa';
const STORAGE_KEY = 'nabugate_lang';
const PREFIX_RE = /^\/(fa|en)(?=\/|$)/;

const supported = (l) => (l && Object.prototype.hasOwnProperty.call(LANGS, l) ? l : null);

function readStored() {
  try { return localStorage.getItem(STORAGE_KEY); } catch { return null; }
}
function writeStored(l) {
  try { localStorage.setItem(STORAGE_KEY, l); } catch { /* private mode */ }
}

// Public pages can carry the language in the path (/en, /fa/docs) so a link
// shared in one language opens in that language.
export function langFromPath(pathname = window.location.pathname) {
  const m = PREFIX_RE.exec(pathname);
  return m ? m[1] : null;
}
export function stripLangPrefix(pathname) {
  return pathname.replace(PREFIX_RE, '') || '/';
}

// Order: an explicit choice in the URL, then the one remembered from last
// time, then the browser's own preference list, then Persian.
export function detectLang() {
  const explicit = supported(new URLSearchParams(window.location.search).get('lang')) || langFromPath();
  if (explicit) {
    writeStored(explicit);
    return explicit;
  }
  const stored = supported(readStored());
  if (stored) return stored;
  const prefs = navigator.languages && navigator.languages.length ? navigator.languages : [navigator.language];
  for (const p of prefs) {
    const base = supported(String(p || '').toLowerCase().split('-')[0]);
    if (base) return base;
  }
  return DEFAULT_LANG;
}

let current = DEFAULT_LANG;
export const getLang = () => current;
export const getLocale = () => LANGS[current].locale;
export const isRTL = () => LANGS[current].dir === 'rtl';

function apply(l) {
  current = l;
  const el = document.documentElement;
  el.setAttribute('lang', l);
  el.setAttribute('dir', LANGS[l].dir);
}

// ---- formatting -------------------------------------------------------------

const FA_DIGITS = '۰۱۲۳۴۵۶۷۸۹';

/* Digits in the active script; everything else is left as is. */
export function fmtDigits(s) {
  if (s == null) return '';
  const str = String(s);
  return current === 'fa' ? str.replace(/[0-9]/g, (d) => FA_DIGITS[d]) : str;
}

/* A whole number with the locale's grouping, e.g. ۱٬۲۳۴ or 1,234. */
export function fmtInt(n) {
  return Number(n || 0).toLocaleString(getLocale(), { maximumFractionDigits: 0 });
}

/* Any number with locale grouping and the given Intl options. */
export function fmtNum(n, opts) {
  return Number(n || 0).toLocaleString(getLocale(), opts);
}

// Money is USD everywhere, because that is the unit the gateway prices models
// in and stores as cost_usd. Parts of the panel used to render the same
// `balance` field labelled "تومان" while others labelled it "$" — one of the
// two was wrong by a factor of tens of thousands, and neither said which.
export const usd = (n) => fmtDigits('$' + Number(n || 0).toFixed(2));

/* Dates follow the language: the Solar Hijri calendar in Persian. */
export function fmtDate(d, kind = 'date') {
  if (!d) return '—';
  const date = d instanceof Date ? d : new Date(d);
  if (Number.isNaN(date.getTime())) return '—';
  const loc = getLocale();
  if (kind === 'time') return date.toLocaleTimeString(loc);
  if (kind === 'datetime') return date.toLocaleString(loc);
  return date.toLocaleDateString(loc);
}

// ---- translation ------------------------------------------------------------

function interpolate(str, vars) {
  if (!vars) return str;
  return str.replace(/\{(\w+)\}/g, (m, k) => (vars[k] !== undefined && vars[k] !== null ? vars[k] : m));
}

/*
 * Look a key up in a { fa: {...}, en: {...} } dictionary. A value may be a
 * string with {placeholders} or a function of the vars that returns JSX, for
 * sentences that carry a link or a <code> in the middle.
 */
export function translate(dict, lang, key, vars) {
  let v = dict?.[lang]?.[key];
  if (v === undefined) v = dict?.[DEFAULT_LANG]?.[key];
  if (v === undefined) v = dict?.en?.[key];
  if (v === undefined) {
    if (import.meta.env?.DEV) console.warn(`[i18n] missing key "${key}"`);
    return key;
  }
  if (typeof v === 'function') return v(vars || {});
  return interpolate(v, vars);
}

const I18nContext = createContext({
  lang: DEFAULT_LANG,
  dir: LANGS[DEFAULT_LANG].dir,
  locale: LANGS[DEFAULT_LANG].locale,
  isRTL: true,
  setLang: () => {},
});

export function I18nProvider({ children }) {
  const [lang, setLangState] = useState(() => {
    const l = detectLang();
    apply(l);
    return l;
  });

  const setLang = useCallback((l) => {
    if (!supported(l)) return;
    writeStored(l);
    apply(l);
    // Keep an explicit URL in step with the switch, or a reload would put the
    // old language back.
    const url = new URL(window.location.href);
    let changed = false;
    if (url.searchParams.has('lang')) {
      url.searchParams.set('lang', l);
      changed = true;
    }
    if (langFromPath(url.pathname)) {
      url.pathname = url.pathname.replace(PREFIX_RE, '/' + l);
      changed = true;
    }
    if (changed) window.history.replaceState(window.history.state, '', url.toString());
    setLangState(l);
  }, []);

  const value = useMemo(
    () => ({ lang, dir: LANGS[lang].dir, locale: LANGS[lang].locale, isRTL: LANGS[lang].dir === 'rtl', setLang }),
    [lang, setLang],
  );
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n() {
  return useContext(I18nContext);
}

/* t(key, vars) bound to a component's own dictionary and the active language. */
export function useT(dict) {
  const { lang } = useContext(I18nContext);
  return useCallback((key, vars) => translate(dict, lang, key, vars), [dict, lang]);
}
