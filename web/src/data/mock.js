export const nav = [
  { id: 'dashboard', label: 'داشبورد', icon: '⊞' },
  { id: 'plans', label: 'پلن‌ها', icon: '✨' },
  { id: 'usage', label: 'مصرف', icon: '◴' },
  { id: 'tokens', label: 'کلیدهای API', icon: '🔑' },
  { id: 'integration', label: 'راهنمای توسعه', icon: '📖' },
  { id: 'teams', label: 'تیم‌های من', icon: '👥' },
  { id: 'account', label: 'حساب کاربری', icon: '👤' },
  { id: 'models', label: 'مدل‌ها', icon: '🧠' },
  { id: 'users', label: 'مدیریت کاربران (ادمین)', icon: '👑' },
  { id: 'providers', label: 'پروایدرها (ادمین)', icon: '🔌' },
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
