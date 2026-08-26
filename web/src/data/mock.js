export const navGroups = [
  {
    title: 'مرور کلی',
    items: [
      { id: 'dashboard', label: 'داشبورد', icon: '⊞' },
      { id: 'plans', label: 'خرید و شارژ', icon: '✨' },
    ]
  },
  {
    title: 'توسعه‌دهنده',
    items: [
      { id: 'tokens', label: 'کلیدهای API', icon: '🔑' },
      { id: 'models', label: 'مدل‌ها', icon: '🧠' },
    ]
  },
  {
    title: 'حساب کاربری',
    items: [
      { id: 'profile', label: 'پروفایل', icon: '👤' },
    ]
  },
  {
    title: 'مدیریت کل (ادمین)',
    adminOnly: true,
    items: [
      { id: 'usage', label: 'مصرف کل', icon: '◴' },
      { id: 'users', label: 'کاربران', icon: '👑' },
      { id: 'providers', label: 'پروایدرها', icon: '🔌' },
      { id: 'agents', label: 'عامل‌ها', icon: '🤖' },
      { id: 'keys', label: 'کلیدهای سیستم', icon: '🔐' },
    ]
  }
];

export const faInt = (n) => {
  if (n == null) return '۰';
  return Number(n).toLocaleString('fa-IR');
};

export const faDigits = (s) => {
  if (s == null) return '';
  const map = ['۰','۱','۲','۳','۴','۵','۶','۷','۸','۹'];
  return String(s).replace(/[0-9]/g, d => map[d]);
};
