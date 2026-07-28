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

const VIEWS = {
  dashboard: () => <Dashboard />,
  providers: () => <Providers />,
  models: () => <Models />,
  keys: () => <Keys />,
  tokens: () => <Tokens />,
  usage: () => <Usage />,
  agents: () => <Agents />,
  users: () => <Users />,
  logs: () => (
    <Placeholder
      title="لاگ‌ها"
      subtitle="لاگ‌های ساخت‌یافتهٔ JSON: تأخیر، توکن، هزینه، وضعیت"
      icon="➤"
      body="نمایش لاگ‌های زندهٔ دروازه به‌زودی در این نما اضافه می‌شود."
    />
  ),
};

// The current view lives in the URL hash (#/tokens) so every menu click changes
// the address bar and each section is a real, shareable, back-button-able link.
function viewFromHash() {
  const id = (window.location.hash || '').replace(/^#\/?/, '');
  return VIEWS[id] ? id : 'dashboard';
}

export default function App() {
  const [view, setView] = useState(viewFromHash);
  // null while we ask the gateway; the console must not flash its shell before
  // we know whether this visitor is allowed to see it.
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
    window.location.hash = '#/' + id; // updates the URL; hashchange syncs the view.
  };

  if (session === null) return <div className="app-boot">…</div>;
  if (!session.authenticated) {
    return <SignIn needsSetup={session.needs_setup} onAuthenticated={refresh} />;
  }

  const render = VIEWS[view] || VIEWS.dashboard;
  return (
    <div className="app">
      <Sidebar current={view} onNavigate={navigate} />
      {render()}
    </div>
  );
}
