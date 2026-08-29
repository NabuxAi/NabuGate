export const navGroups = [
  {
    title: 'مرور کلی',
    items: [
      { id: 'dashboard', label: 'داشبورد', icon: '⊞' },
      { id: 'account', label: 'حساب و مصرف', icon: '◴' },
      { id: 'plans', label: 'خرید و شارژ', icon: '✨' },
      { id: 'payments', label: 'پرداخت‌ها', icon: '💳' },
    ]
  },
  {
    title: 'توسعه‌دهنده',
    items: [
      { id: 'tokens', label: 'کلیدهای API', icon: '🔑' },
      { id: 'models', label: 'مدل‌ها', icon: '🧠' },
      { id: 'requests', label: 'درخواست‌ها', icon: '➤' },
      { id: 'integration', label: 'اتصال به دروازه', icon: '🔗' },
      { id: 'docs', label: 'مستندات', icon: '📘' },
    ]
  },
  {
    title: 'حساب کاربری',
    items: [
      { id: 'profile', label: 'پروفایل', icon: '👤' },
      { id: 'security', label: 'امنیت', icon: '🛡️' },
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

// Money is USD everywhere, because that is the unit the gateway prices models
// in and stores as cost_usd. Parts of the panel used to render the same
// `balance` field labelled "تومان" while others labelled it "$" — one of the
// two was wrong by a factor of tens of thousands, and neither said which.
export const usd = (n) => faDigits('$' + Number(n || 0).toFixed(2));
