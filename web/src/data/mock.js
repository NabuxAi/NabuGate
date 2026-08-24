export const navGroups = [
  {
    title: 'مرور کلی',
    items: [
      { id: 'dashboard', label: 'داشبورد', icon: '⊞' },
      { id: 'plans', label: 'پلن‌ها', icon: '✨' },
      { id: 'subscriptions', label: 'اشتراک‌ها', icon: '💼' },
      { id: 'usage', label: 'مصرف', icon: '◴' },
    ]
  },
  {
    title: 'توسعه‌دهنده',
    items: [
      { id: 'tokens', label: 'کلیدهای API', icon: '🔑' },
      { id: 'requests', label: 'درخواست‌ها', icon: '📄' },
      { id: 'integration', label: 'راهنمای توسعه', icon: '📖' },
    ]
  },
  {
    title: 'تیم‌ها',
    items: [
      { id: 'teams', label: 'تیم‌های من', icon: '👥' },
      { id: 'invitations', label: 'دعوت‌نامه‌ها', icon: '✉️' },
    ]
  },
  {
    title: 'حساب کاربری',
    items: [
      { id: 'payments', label: 'پرداخت‌ها', icon: '💳' },
      { id: 'referrals', label: 'دعوت دوستان', icon: '🎁' },
      { id: 'profile', label: 'پروفایل', icon: '👤' },
      { id: 'security', label: 'امنیت', icon: '🛡️' },
      { id: 'support', label: 'پشتیبانی', icon: '⚙️' },
    ]
  },
  {
    title: 'آموزش',
    items: [
      { id: 'help', label: 'راهنما', icon: '❓' },
    ]
  },
  {
    title: 'مدیریت (ادمین)',
    adminOnly: true,
    items: [
      { id: 'models', label: 'مدل‌ها', icon: '🧠' },
      { id: 'users', label: 'کاربران', icon: '👑' },
      { id: 'providers', label: 'پروایدرها', icon: '🔌' },
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
