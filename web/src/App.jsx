import { lazy, Suspense, useEffect, useState } from 'react';
import { useTheme } from './useTheme.js';
import { getLang, stripLangPrefix, useI18n, useT } from './i18n/index.jsx';
import { NAV_T, navGroups } from './nav.js';

import * as api from './api.js';
import Landing from './views/Landing.jsx';
import { BootShell } from './components/Skeleton.jsx';
import { CONTENT_LOADERS } from './views/docs/load.js';

/*
 * Only the landing page is in the first download: it is where a new visitor
 * arrives, so it must not wait on a second round trip. Everything else is its
 * own chunk, fetched when its route is opened — the docs (whose prose is split
 * again per language), the sign-in form, and the console with all its views.
 * Before, all of it was one 400 KB script that a visitor to the home page had
 * to download and parse before seeing anything.
 */
const loadConsole = () => import('./Console.jsx');
const loadDocs = () => import('./views/Docs.jsx');
const loadSignIn = () => import('./views/SignIn.jsx');
const Console = lazy(loadConsole);
const Docs = lazy(loadDocs);
const SignIn = lazy(loadSignIn);

// Every console view has a sidebar entry, so navGroups is the list of them.
const KNOWN_VIEWS = new Set(['landing', ...navGroups.flatMap((g) => g.items.map((i) => i.id))]);

function viewFromPath() {
  // /en and /fa are language prefixes on the public pages, not views.
  let path = stripLangPrefix(window.location.pathname);
  if (path.startsWith('/admin/')) path = path.replace('/admin/', '');
  else if (path.startsWith('/panel/')) path = path.replace('/panel/', '');
  else if (path === '/admin') path = '';
  else if (path === '/panel') path = '';
  else if (path.startsWith('/')) path = path.replace('/', '');

  if (path === '' && window.location.hash && window.location.hash.startsWith('#/')) {
    path = window.location.hash.replace(/^#\/?/, '');
  }

  return path ? (KNOWN_VIEWS.has(path) ? path : 'dashboard') : 'landing';
}

// Start fetching the chunks this URL needs while React is still booting, all
// at once, rather than one after another as each render asks for the next: the
// docs page and its prose; or, on a console URL, the console and the sign-in
// form (which one shows depends on /api/status, and waiting for it to decide
// would add its round trip to the chain).
(function preloadRoute() {
  const p = window.location.pathname;
  if (p.startsWith('/admin') || p.startsWith('/panel')) {
    loadConsole();
    loadSignIn();
  } else if (viewFromPath() === 'docs') {
    loadDocs();
    CONTENT_LOADERS[getLang()]?.();
  }
})();

export default function App() {
  useTheme();
  // Reading the language here re-renders the whole tree on a switch, which is
  // what the module-level formatters (fmtInt, usd …) rely on.
  const { lang } = useI18n();
  const tNav = useT(NAV_T);

  const [view, setView] = useState(viewFromPath);
  const [session, setSession] = useState(null);

  const isPanel = window.location.pathname.startsWith('/panel');
  const isAdminPath = window.location.pathname.startsWith('/admin');
  const isPublic = (view === 'landing' || view === 'docs') && !isPanel && !isAdminPath;

  const refresh = () =>
    api
      .status()
      .then(setSession)
      .catch(() => setSession({ authenticated: false, needs_setup: false }));

  // Braces: refresh returns a promise, which React would take for a cleanup.
  useEffect(() => { refresh(); }, []);

  useEffect(() => {
    const onPopState = () => setView(viewFromPath());
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, []);

  // Any request answered 401 means the session ended while the page was open.
  // Re-checking is enough: /api/status answers 200 with authenticated false,
  // so this cannot loop, and the render below then shows the sign-in form.
  useEffect(() => {
    const onExpired = () => refresh();
    window.addEventListener('nabu:unauthenticated', onExpired);
    return () => window.removeEventListener('nabu:unauthenticated', onExpired);
  }, []);

  const navigate = (id) => {
    const basePath = isAdminPath ? '/admin' : (isPanel ? '/panel' : '');
    const newUrl = `${basePath}/${id}`;
    window.history.pushState(null, '', newUrl);
    window.dispatchEvent(new PopStateEvent('popstate'));
  };

  useEffect(() => {
    const brand = 'NabuGate';
    if (view === 'landing') document.title = lang === 'fa' ? 'نبوگیت — دروازهٔ یکپارچهٔ هوش مصنوعی' : 'NabuGate — one API for every AI model';
    else if (view === 'docs') document.title = `${lang === 'fa' ? 'مستندات' : 'Docs'} · ${brand}`;
    else document.title = `${tNav(view)} · ${brand}`;
  }, [view, lang, tNav]);

  if (isPublic) {
    // The public pages look the same signed in or out; only the header's call
    // to action changes. So they render at once rather than waiting for
    // /api/status, and the button relabels when it answers.
    const signedIn = !!session?.authenticated;
    return view === 'docs'
      ? <Suspense fallback={null}><Docs signedIn={signedIn} /></Suspense>
      : <Landing signedIn={signedIn} />;
  }
  if (session === null) return <BootShell />;
  if (!session.authenticated) {
    return (
      <Suspense fallback={<BootShell />}>
        <SignIn needsSetup={session.needs_setup} onAuthenticated={refresh} />
      </Suspense>
    );
  }
  return (
    <Suspense fallback={<BootShell />}>
      <Console view={view} session={session} navigate={navigate} isPanel={isPanel} isAdminPath={isAdminPath} />
    </Suspense>
  );
}
