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
import Profile from "./views/Profile.jsx";
import Payments from "./views/Payments.jsx";
import Placeholder from './views/Placeholder.jsx';
import Integration from './views/Integration.jsx';
import Account from './views/Account.jsx';
import Plans from './views/Plans.jsx';
import Teams from './views/Teams.jsx';
import Landing from './views/Landing.jsx';

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
  teams: () => <Teams />,
  subscriptions: () => <Placeholder title="اشتراک‌ها" subtitle="مدیریت اشتراک‌های فعال شما" icon="💼" />,
  requests: () => <Placeholder title="درخواست‌ها" subtitle="گزارش درخواست‌های ارسالی به API" icon="📄" />,
  invitations: () => <Placeholder title="دعوت‌نامه‌ها" subtitle="دعوت‌نامه‌های ارسالی و دریافتی" icon="✉️" />,
  payments: () => <Payments />,
  referrals: () => <Placeholder title="دعوت دوستان" subtitle="لینک دعوت و پاداش‌ها" icon="🎁" />,
  profile: () => <Profile />,
  security: () => <Placeholder title="امنیت" subtitle="تنظیمات امنیتی و رمز عبور" icon="🛡️" />,
  support: () => <Placeholder title="پشتیبانی" subtitle="تیکت‌ها و ارتباط با پشتیبانی" icon="⚙️" />,
  help: () => <Placeholder title="راهنما" subtitle="مستندات و آموزش‌ها" icon="❓" />,
  logs: () => (
    <Placeholder
      title="لاگ‌ها"
      subtitle="لاگ‌های ساخت‌یافتهٔ JSON: تأخیر، توکن، هزینه، وضعیت"
      icon="➤"
      body="نمایش لاگ‌های زندهٔ دروازه به‌زودی در این نما اضافه می‌شود."
    />
  ),
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
  let allowed = [
    'dashboard', 'integration', 'tokens', 'account', 'plans', 'teams',
    'subscriptions', 'requests', 'invitations', 'payments', 'referrals', 'profile', 'security', 'support', 'help'
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
