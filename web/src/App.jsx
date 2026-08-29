import { useEffect, useState } from 'react';
import { useTheme } from "./useTheme.js";


import * as api from './api.js';
import SignIn from './views/SignIn.jsx';
import Tokens from './views/Tokens.jsx';
import Sidebar from './components/Sidebar.jsx';
import Dashboard from './views/Dashboard.jsx';
import Providers from './views/Providers.jsx';
import Models from './views/Models.jsx';
import Keys from './views/Keys.jsx';
import Usage from './views/Usage.jsx';
import Agents from './views/Agents.jsx';
import Users from './views/Users.jsx';
import Profile from "./views/Profile.jsx";
import Payments from "./views/Payments.jsx";
import Integration from './views/Integration.jsx';
import Account from './views/Account.jsx';
import Plans from './views/Plans.jsx';
import Landing from './views/Landing.jsx';
import Docs from './views/Docs.jsx';
import Security from './views/Security.jsx';
import Requests from './views/Requests.jsx';

const VIEWS = {
  landing: () => <Landing lang={window.location.pathname.startsWith('/fa') ? 'fa' : 'en'} />,
  docs: () => <Docs />,

  dashboard: () => <Dashboard />,
  providers: () => <Providers />,
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
  let path = window.location.pathname;
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

  const navigate = (id) => {
    const basePath = isAdminPath ? '/admin' : (isPanel ? '/panel' : '');
    const newUrl = `${basePath}/${id}`;
    window.history.pushState(null, '', newUrl);
    window.dispatchEvent(new PopStateEvent('popstate'));
  };

  if (session === null) return <div className="app-boot">…</div>;
  if (!session.authenticated && (view === 'landing' || view === 'docs') && !isPanel && !isAdminPath) {
    const View = VIEWS[view];
    return <View />;
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
    'tokens', 'models', 'requests', 'integration', 'docs',
    'profile', 'security',
  ];

  const effectivelyAdmin = isAdminPath && session.is_admin;
  if (effectivelyAdmin) {
    allowed = Object.keys(VIEWS);
  }

  const safeView = allowed.includes(view) ? view : 'dashboard';
  const render = VIEWS[safeView] || VIEWS.dashboard;

  return (
    <div className="app">
      <Sidebar current={safeView} onNavigate={navigate} effectivelyAdmin={effectivelyAdmin} isPanel={isPanel} />
      {render()}
    </div>
  );
}
