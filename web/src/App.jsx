import { useEffect, useState } from 'react';

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
import Placeholder from './views/Placeholder.jsx';
import Integration from './views/Integration.jsx';
import Account from './views/Account.jsx';
import Plans from './views/Plans.jsx';
import Teams from './views/Teams.jsx';

const VIEWS = {
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
  teams: () => <Teams />,
  logs: () => (
    <Placeholder
      title="لاگ‌ها"
      subtitle="لاگ‌های ساخت‌یافتهٔ JSON: تأخیر، توکن، هزینه، وضعیت"
      icon="➤"
      body="نمایش لاگ‌های زندهٔ دروازه به‌زودی در این نما اضافه می‌شود."
    />
  ),
};

function viewFromHash() {
  const id = (window.location.hash || '').replace(/^#\/?/, '');
  return VIEWS[id] ? id : 'dashboard';
}

export default function App() {
  const [view, setView] = useState(viewFromHash);
  const [session, setSession] = useState(null);

  const refresh = () =>
    api
      .status()
      .then(setSession)
      .catch(() => setSession({ authenticated: false, needs_setup: false }));

  useEffect(refresh, []);

  useEffect(() => {
    const onHash = () => setView(viewFromHash());
    window.addEventListener('hashchange', onHash);
    return () => window.removeEventListener('hashchange', onHash);
  }, []);

  const navigate = (id) => {
    window.location.hash = '#/' + id;
  };

  if (session === null) return <div className="app-boot">…</div>;
  if (!session.authenticated) {
    return <SignIn needsSetup={session.needs_setup} onAuthenticated={refresh} />;
  }

  let allowed = ['dashboard', 'integration', 'tokens', 'usage', 'logs', 'account', 'models', 'plans', 'teams'];
  if (session.is_admin) allowed = Object.keys(VIEWS);
  const safeView = allowed.includes(view) ? view : 'dashboard';
  const render = VIEWS[safeView] || VIEWS.dashboard;

  return (
    <div className="app">
      <Sidebar current={safeView} onNavigate={navigate} isAdmin={session.is_admin} />
      {render()}
    </div>
  );
}
