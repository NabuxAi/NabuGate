import { useEffect, useState } from 'react';
import { useTheme } from './useTheme.js';
import { stripLangPrefix, useI18n, useT } from './i18n/index.jsx';
import { NAV_T } from './nav.js';

import * as api from './api.js';
import SignIn from './views/SignIn.jsx';
import Tokens from './views/Tokens.jsx';
import Sidebar from './components/Sidebar.jsx';
import Dashboard from './views/Dashboard.jsx';
import Providers from './views/Providers.jsx';
import ProviderRequests from './views/ProviderRequests.jsx';
import Models from './views/Models.jsx';
import Keys from './views/Keys.jsx';
import Usage from './views/Usage.jsx';
import Agents from './views/Agents.jsx';
import Users from './views/Users.jsx';
import Opsless from './views/Opsless.jsx';
import Profile from "./views/Profile.jsx";
import Payments from "./views/Payments.jsx";
import Integration from './views/Integration.jsx';
import Account from './views/Account.jsx';
import Plans from './views/Plans.jsx';
import Landing from './views/Landing.jsx';
import Docs from './views/Docs.jsx';
import Security from './views/Security.jsx';
import Requests from './views/Requests.jsx';
import { BootShell } from './components/Skeleton.jsx';
import ErrorBoundary from './components/ErrorBoundary.jsx';

const VIEWS = {
  landing: () => <Landing />,
  docs: () => <Docs embedded />,

  dashboard: () => <Dashboard />,
  providers: () => <Providers />,
  'provider-requests': () => <ProviderRequests />,
  opsless: () => <Opsless />,
  models: () => <Models />,
  keys: () => <Keys />,
  tokens: () => <Tokens />,
  usage: () => <Usage />,
  agents: () => <Agents />,
  users: () => <Users />,
  integration: () => <Integration />,
  account: () => <Account />,
  plans: () => <Plans />,
  payments: () => <Payments />,
  profile: () => <Profile />,
  security: () => <Security />,
  requests: () => <Requests />,
};

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

  return path ? (VIEWS[path] ? path : 'dashboard') : 'landing';
}

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

  const refresh = () =>
    api
      .status()
      .then(setSession)
      .catch(() => setSession({ authenticated: false, needs_setup: false }));

  useEffect(refresh, []);

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

  if (session === null) return <BootShell />;
  if ((view === 'landing' || view === 'docs') && !isPanel && !isAdminPath) {
    // The public pages look the same signed in or out; only the header's
    // call to action changes.
    return view === 'docs' ? <Docs signedIn={!!session.authenticated} /> : <Landing signedIn={!!session.authenticated} />;
  }
  if (!session.authenticated) {
    return <SignIn needsSetup={session.needs_setup} onAuthenticated={refresh} />;
  }

  // Determine allowed views based on whether they are in /panel/ or /admin/
  // Every id here has a sidebar entry in navGroups, and every navGroups entry
  // is here. The two lists drifted apart before: eleven views were routable
  // with nothing linking to them, and the views that had no data behind them
  // rendered an apology.
  let allowed = [
    'dashboard', 'account', 'plans', 'payments',
    'tokens', 'models', 'providers', 'requests', 'integration', 'docs',
    'profile', 'security',
  ];

  const effectivelyAdmin = isAdminPath && session.is_admin;
  if (effectivelyAdmin) {
    // The landing page is a public page, not a console view: /admin/ with no
    // view used to render it inside the sidebar layout.
    allowed = Object.keys(VIEWS).filter((v) => v !== 'landing');
  }

  const safeView = allowed.includes(view) ? view : 'dashboard';
  const render = VIEWS[safeView] || VIEWS.dashboard;

  return (
    <div className="app">
      <Sidebar current={safeView} onNavigate={navigate} effectivelyAdmin={effectivelyAdmin} isPanel={isPanel} />
      <div className="nav-backdrop" onClick={(e) => e.currentTarget.parentElement.classList.remove('nav-open')} />
      {/* Keyed on the view so each page mounts fresh and plays its entrance. */}
      <div key={safeView} className="view-enter" style={{ flex: 1, minWidth: 0, display: 'flex' }}>
        <ErrorBoundary resetKey={safeView}>{render()}</ErrorBoundary>
      </div>
    </div>
  );
}
