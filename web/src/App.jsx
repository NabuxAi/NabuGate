import { useState } from 'react';

import Sidebar from './components/Sidebar.jsx';
import Dashboard from './views/Dashboard.jsx';
import Providers from './views/Providers.jsx';
import Models from './views/Models.jsx';
import Keys from './views/Keys.jsx';
import Usage from './views/Usage.jsx';
import Placeholder from './views/Placeholder.jsx';

const VIEWS = {
  dashboard: () => <Dashboard />,
  providers: () => <Providers />,
  models: () => <Models />,
  keys: () => <Keys />,
  usage: () => <Usage />,
  agents: () => (
    <Placeholder
      title="ساب‌اجنت‌ها"
      subtitle="دستیارهای نام‌دار: system prompt + پارامترهای پیش‌فرض روی یک آلیاس"
      icon="◈"
      body="مدیریت ساب‌اجنت‌ها به‌زودی در این نما اضافه می‌شود."
    />
  ),
  logs: () => (
    <Placeholder
      title="لاگ‌ها"
      subtitle="لاگ‌های ساخت‌یافتهٔ JSON: تأخیر، توکن، هزینه، وضعیت"
      icon="➤"
      body="نمایش لاگ‌های زندهٔ دروازه به‌زودی در این نما اضافه می‌شود."
    />
  ),
};

export default function App() {
  const [view, setView] = useState('dashboard');
  const render = VIEWS[view] || VIEWS.dashboard;
  return (
    <div className="app">
      <Sidebar current={view} onNavigate={setView} />
      {render()}
    </div>
  );
}
