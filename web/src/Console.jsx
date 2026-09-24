import { lazy, Suspense } from 'react';
import Sidebar from './components/Sidebar.jsx';
import ErrorBoundary from './components/ErrorBoundary.jsx';
import { SkeletonText } from './components/Skeleton.jsx';
import Tokens from './views/Tokens.jsx';
import Dashboard from './views/Dashboard.jsx';
import Providers from './views/Providers.jsx';
import ProviderRequests from './views/ProviderRequests.jsx';
import Models from './views/Models.jsx';
import Keys from './views/Keys.jsx';
import Usage from './views/Usage.jsx';
import Agents from './views/Agents.jsx';
import Users from './views/Users.jsx';
import Opsless from './views/Opsless.jsx';
import Profile from './views/Profile.jsx';
import Payments from './views/Payments.jsx';
import Integration from './views/Integration.jsx';
import Account from './views/Account.jsx';
import Plans from './views/Plans.jsx';
import Security from './views/Security.jsx';
import Requests from './views/Requests.jsx';

/*
 * The signed-in console: sidebar plus every console view.
 *
 * This is its own chunk, loaded only once someone is signed in on /admin or
 * /panel, so the landing page and the docs never download twenty screens their
 * visitors cannot open. The views stay together in one chunk on purpose:
 * moving between them should be instant, not a network round trip per click.
 */
const Docs = lazy(() => import('./views/Docs.jsx'));

const VIEWS = {
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

export default function Console({ view, session, navigate, isPanel, isAdminPath }) {
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
    // The landing page is a public page, not a console view (and is not in
    // VIEWS): /admin/ with no view used to render it inside the sidebar layout.
    allowed = Object.keys(VIEWS);
  }

  const safeView = allowed.includes(view) ? view : 'dashboard';
  const render = VIEWS[safeView] || VIEWS.dashboard;

  return (
    <div className="app">
      <Sidebar current={safeView} onNavigate={navigate} effectivelyAdmin={effectivelyAdmin} isPanel={isPanel} />
      <div className="nav-backdrop" onClick={(e) => e.currentTarget.parentElement.classList.remove('nav-open')} />
      {/* Keyed on the view so each page mounts fresh and plays its entrance. */}
      <div key={safeView} className="view-enter" style={{ flex: 1, minWidth: 0, display: 'flex' }}>
        <ErrorBoundary resetKey={safeView}>
          <Suspense fallback={<div className="main"><div className="content"><SkeletonText lines={6} /></div></div>}>
            {render()}
          </Suspense>
        </ErrorBoundary>
      </div>
    </div>
  );
}
