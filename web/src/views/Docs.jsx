import { lazy, Suspense, useEffect, useMemo, useRef, useState } from 'react';
import Layout from '../components/Layout.jsx';
import Icon from '../components/Icon.jsx';
import { SkeletonText } from '../components/Skeleton.jsx';
import { LangSwitch, Logo, ThemeToggle } from '../components/Brand.jsx';
import { DEFAULT_LANG, useI18n, useT } from '../i18n/index.jsx';
import { ALL, ALL_IDS, GROUPS } from './docs/toc.js';
import { CONTENT_LOADERS } from './docs/load.js';
import '../styles/docs.css';

/*
 * Documentation, served publicly at /docs and inside the console.
 *
 * Every section is addressable (#billing, #cursor …) so a support reply can
 * point at one answer. The base URL is the host the page is served from, so a
 * self-hosted gateway documents itself without an edit.
 *
 * The prose lives in docs/content.<lang>.jsx, one file per language, picked up
 * by glob: a language whose file does not exist yet falls back to Persian
 * instead of failing the build. Each is its own chunk, loaded when that
 * language is shown, so a reader downloads one language's prose, not all of
 * them.
 */
const CONTENT = Object.fromEntries(
  Object.entries(CONTENT_LOADERS).map(([lang, load]) => [lang, lazy(load)]),
);

// Runs once its children have mounted: the "on this page" list reads the
// headings out of the rendered prose, which for a lazily loaded language exists
// only after the chunk arrives, not when the section first changes.
function Mounted({ onMount, children }) {
  useEffect(onMount, []);
  return children;
}

const ORIGIN = typeof window !== 'undefined' && window.location.origin.startsWith('http')
  ? window.location.origin
  : 'https://gate.nabuxai.com';
const BASE = `${ORIGIN}/v1`;
const KEY = 'ng_xxxxxxxxxxxxxxxxxxxx';

const T = {
  fa: {
    docs: 'مستندات',
    subtitle: 'راهنمای اتصال، پرداخت و مرجع API',
    home: 'صفحهٔ اصلی',
    pricing: 'قیمت‌ها',
    console: 'ورود به پنل',
    openConsole: 'رفتن به پنل',
    search: 'جست‌وجو در مستندات…',
    noResults: 'چیزی پیدا نشد.',
    onThisPage: 'در این صفحه',
    prev: 'قبلی',
    next: 'بعدی',
    sections: 'بخش‌ها',
    help: 'پاسخ را پیدا نکردید؟',
    helpBody: 'شناسهٔ درخواست یا فاکتور را از پنل بردارید و به پشتیبانی بفرستید.',
    baseUrl: 'آدرس پایه',
  },
  en: {
    docs: 'Docs',
    subtitle: 'Connecting, billing and the API reference',
    home: 'Home',
    pricing: 'Pricing',
    console: 'Sign in',
    openConsole: 'Open console',
    search: 'Search the docs…',
    noResults: 'Nothing matches.',
    onThisPage: 'On this page',
    prev: 'Previous',
    next: 'Next',
    sections: 'Sections',
    help: 'Didn’t find your answer?',
    helpBody: 'Grab the request or invoice id from the console and send it to support.',
    baseUrl: 'Base URL',
  },
};

function fromHash() {
  const h = window.location.hash.replace(/^#\/?/, '');
  return ALL_IDS.includes(h) ? h : 'intro';
}

export default function Docs({ embedded = false, signedIn = false }) {
  const t = useT(T);
  const { lang } = useI18n();
  const [active, setActive] = useState(fromHash);
  const [query, setQuery] = useState('');
  const [menuOpen, setMenuOpen] = useState(false);
  const [toc, setToc] = useState([]);
  const [current, setCurrent] = useState(null);
  const [rendered, setRendered] = useState(0);
  const body = useRef(null);

  useEffect(() => {
    const onHash = () => setActive(fromHash());
    window.addEventListener('hashchange', onHash);
    return () => window.removeEventListener('hashchange', onHash);
  }, []);

  const go = (id) => {
    window.history.replaceState(null, '', '#' + id);
    setActive(id);
    setMenuOpen(false);
    window.scrollTo({ top: 0, behavior: 'smooth' });
  };

  // "On this page": read the section's own h2s after it renders, so the
  // content files never have to declare their headings twice.
  useEffect(() => {
    const el = body.current;
    if (!el) return undefined;
    const hs = [...el.querySelectorAll('h2.docs-h2')];
    hs.forEach((h, i) => { h.id = `${active}--${i}`; });
    setToc(hs.map((h) => ({ id: h.id, text: h.textContent })));
    setCurrent(hs[0]?.id || null);
    if (!hs.length || typeof IntersectionObserver === 'undefined') return undefined;
    const io = new IntersectionObserver(
      (entries) => {
        const vis = entries.filter((e) => e.isIntersecting).sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top);
        if (vis[0]) setCurrent(vis[0].target.id);
      },
      { rootMargin: '-80px 0px -65% 0px' },
    );
    hs.forEach((h) => io.observe(h));
    return () => io.disconnect();
  }, [active, lang, rendered]);

  const title = (item) => item[lang] || item[DEFAULT_LANG];
  const q = query.trim().toLowerCase();
  const groups = useMemo(
    () => GROUPS.map((g) => ({
      ...g,
      items: q ? g.items.filter((i) => (i.fa + ' ' + i.en + ' ' + i.id).toLowerCase().includes(q)) : g.items,
    })).filter((g) => g.items.length),
    [q],
  );

  const idx = ALL_IDS.indexOf(active);
  const prev = idx > 0 ? ALL[idx - 1] : null;
  const next = idx < ALL.length - 1 ? ALL[idx + 1] : null;
  const activeItem = ALL[idx] || ALL[0];
  const activeGroup = GROUPS.find((g) => g.items.some((i) => i.id === active)) || GROUPS[0];

  const Content = CONTENT[lang] || CONTENT[DEFAULT_LANG];

  const sidebar = (
    <aside className={'docs-nav' + (menuOpen ? ' open' : '')}>
      <label className="docs-search">
        <Icon name="search" size={16} />
        <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder={t('search')} aria-label={t('search')} />
      </label>
      <div className="docs-nav-groups">
        {groups.length === 0 && <p className="docs-empty">{t('noResults')}</p>}
        {groups.map((g) => (
          <div key={g.title.en} className="docs-nav-group">
            <h3><Icon name={g.icon} size={14} />{g.title[lang] || g.title[DEFAULT_LANG]}</h3>
            <ul>
              {g.items.map((item) => (
                <li key={item.id}>
                  <a
                    href={'#' + item.id}
                    className={'docs-link' + (active === item.id ? ' active' : '')}
                    aria-current={active === item.id ? 'page' : undefined}
                    onClick={(e) => { e.preventDefault(); go(item.id); }}
                  >
                    {title(item)}
                  </a>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
    </aside>
  );

  // On a phone the section nav collapses behind this bar, which also says
  // where you are.
  const menuBtn = (
    <button type="button" className="docs-menu-btn" onClick={() => setMenuOpen((o) => !o)} aria-expanded={menuOpen}>
      <Icon name="menu" size={16} />
      <span>{activeGroup.title[lang] || activeGroup.title[DEFAULT_LANG]}</span>
      <Icon name="chevron" size={14} className="flip-rtl" />
      <strong>{title(activeItem)}</strong>
      <Icon name="chevronDown" size={16} style={{ marginInlineStart: 'auto' }} />
    </button>
  );

  const article = (
    <div className="docs-main">
      <div className="docs-crumbs">
        <span>{activeGroup.title[lang] || activeGroup.title[DEFAULT_LANG]}</span>
        <Icon name="chevron" size={13} className="flip-rtl" />
        <span>{title(activeItem)}</span>
      </div>
      <article key={active + lang} ref={body} className="docs-body view-enter">
        <Suspense fallback={<SkeletonText lines={8} />}>
          <Mounted onMount={() => setRendered((n) => n + 1)}>
            <Content active={active} go={go} BASE={BASE} ORIGIN={ORIGIN} KEY={KEY} />
          </Mounted>
        </Suspense>
      </article>
      <nav className="docs-pager">
        {prev ? (
          <a href={'#' + prev.id} className="docs-pager-card" onClick={(e) => { e.preventDefault(); go(prev.id); }}>
            <small><Icon name="arrow" size={13} className="flip-back" />{t('prev')}</small>
            <strong>{title(prev)}</strong>
          </a>
        ) : <span />}
        {next ? (
          <a href={'#' + next.id} className="docs-pager-card next" onClick={(e) => { e.preventDefault(); go(next.id); }}>
            <small>{t('next')}<Icon name="arrow" size={13} className="flip-rtl" /></small>
            <strong>{title(next)}</strong>
          </a>
        ) : <span />}
      </nav>
    </div>
  );

  const rail = (
    <aside className="docs-rail">
      {toc.length > 0 && (
        <>
          <h4>{t('onThisPage')}</h4>
          <ul>
            {toc.map((h) => (
              <li key={h.id}>
                <a
                  href={'#' + active}
                  className={current === h.id ? 'active' : ''}
                  onClick={(e) => {
                    e.preventDefault();
                    document.getElementById(h.id)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
                  }}
                >
                  {h.text}
                </a>
              </li>
            ))}
          </ul>
        </>
      )}
      <div className="docs-rail-card">
        <span className="lbl">{t('baseUrl')}</span>
        <code dir="ltr">{BASE}</code>
      </div>
      <div className="docs-rail-card">
        <strong>{t('help')}</strong>
        <p>{t('helpBody')}</p>
      </div>
    </aside>
  );

  if (embedded) {
    return (
      <Layout title={t('docs')} subtitle={t('subtitle')}>
        <div className="docs-grid docs-embedded">{menuBtn}{sidebar}{article}{rail}</div>
      </Layout>
    );
  }

  return (
    <div className="docs-page">
      <header className="docs-header">
        <div className="docs-header-in">
          <a href="/" className="docs-brand">
            <Logo size={32} />
            <span>NabuGate</span>
            <span className="docs-badge">{t('docs')}</span>
          </a>
          <nav className="docs-top-nav">
            <a href="/">{t('home')}</a>
            <a href="/#pricing">{t('pricing')}</a>
          </nav>
          <div className="docs-top-actions">
            <LangSwitch size="sm" />
            <ThemeToggle />
            <a href="/panel/" className="btn btn-primary btn-sm">{signedIn ? t('openConsole') : t('console')}</a>
          </div>
        </div>
      </header>
      <div className="docs-grid">{menuBtn}{sidebar}{article}{rail}</div>
    </div>
  );
}
