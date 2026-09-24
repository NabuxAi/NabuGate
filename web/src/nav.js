export function navigate(id) {
  const isPanel = window.location.pathname.startsWith('/panel');
  const isAdminPath = window.location.pathname.startsWith('/admin');
  const basePath = isAdminPath ? '/admin' : (isPanel ? '/panel' : '');
  const newUrl = `${basePath}/${id}`;
  window.history.pushState(null, '', newUrl);
  window.dispatchEvent(new PopStateEvent('popstate'));
}

// Sidebar structure. Labels live in NAV_T so each language names the same
// entries; `icon` is a name from components/Icon.jsx.
export const navGroups = [
  {
    key: 'overview',
    items: [
      { id: 'dashboard', icon: 'grid' },
      { id: 'account', icon: 'wallet' },
      { id: 'plans', icon: 'sparkles' },
      { id: 'payments', icon: 'card' },
    ],
  },
  {
    key: 'developer',
    items: [
      { id: 'tokens', icon: 'key' },
      { id: 'providers', icon: 'plug' },
      { id: 'models', icon: 'cpu' },
      { id: 'requests', icon: 'activity' },
      { id: 'integration', icon: 'link' },
      { id: 'docs', icon: 'book' },
    ],
  },
  {
    key: 'account',
    items: [
      { id: 'profile', icon: 'user' },
      { id: 'security', icon: 'shield' },
    ],
  },
  {
    key: 'admin',
    adminOnly: true,
    items: [
      { id: 'usage', icon: 'chart' },
      { id: 'users', icon: 'users' },
      { id: 'provider-requests', icon: 'inbox' },
      { id: 'agents', icon: 'bot' },
      { id: 'keys', icon: 'lock' },
      { id: 'opsless', icon: 'zap' },
    ],
  },
];

export const NAV_T = {
  fa: {
    'group.overview': 'مرور کلی',
    'group.developer': 'توسعه‌دهنده',
    'group.account': 'حساب کاربری',
    'group.admin': 'مدیریت کل',
    dashboard: 'داشبورد',
    account: 'حساب و مصرف',
    plans: 'خرید و شارژ',
    payments: 'پرداخت‌ها',
    tokens: 'کلیدهای API',
    providers: 'پرووایدرها',
    models: 'مدل‌ها',
    requests: 'درخواست‌ها',
    integration: 'اتصال به دروازه',
    docs: 'مستندات',
    profile: 'پروفایل',
    security: 'امنیت',
    usage: 'مصرف کل',
    users: 'کاربران',
    'provider-requests': 'درخواست دسترسی',
    agents: 'عامل‌ها',
    keys: 'کلیدهای سیستم',
    opsless: 'اپس‌لس (Zero-UI)',
  },
  en: {
    'group.overview': 'Overview',
    'group.developer': 'Developers',
    'group.account': 'Account',
    'group.admin': 'Administration',
    dashboard: 'Dashboard',
    account: 'Balance & usage',
    plans: 'Plans & top-up',
    payments: 'Payments',
    tokens: 'API keys',
    providers: 'Providers',
    models: 'Models',
    requests: 'Requests',
    integration: 'Connect',
    docs: 'Docs',
    profile: 'Profile',
    security: 'Security',
    usage: 'Global usage',
    users: 'Users',
    'provider-requests': 'Access requests',
    agents: 'Agents',
    keys: 'System keys',
    opsless: 'Opsless (Zero-UI)',
  },
};
