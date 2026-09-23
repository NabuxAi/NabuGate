/*
 * The documentation's table of contents, one entry per section id. Titles are
 * given in both languages here so the sidebar, the search and the prev/next
 * links agree with each other.
 */
export const GROUPS = [
  { title: { fa: 'شروع', en: 'Getting started' }, icon: 'sparkles', items: [
    { id: 'intro', fa: 'شروع سریع', en: 'Quickstart' },
    { id: 'billing', fa: 'خرید و شارژ حساب', en: 'Billing & top-ups' },
    { id: 'payment-issues', fa: 'مشکلات پرداخت', en: 'Payment issues' },
    { id: 'models', fa: 'مدل‌ها و alias‌ها', en: 'Models & aliases' },
  ] },
  { title: { fa: 'اتصال ابزارها', en: 'Connect your tools' }, icon: 'plug', items: [
    { id: 'env', fa: 'متغیرهای محیطی', en: 'Environment variables' },
    { id: 'cursor', fa: 'Cursor', en: 'Cursor' },
    { id: 'cline', fa: 'Cline / Roo Code / Continue', en: 'Cline / Roo Code / Continue' },
    { id: 'claude-code', fa: 'Claude Code', en: 'Claude Code' },
    { id: 'codex', fa: 'Codex CLI', en: 'Codex CLI' },
    { id: 'vscode', fa: 'VS Code', en: 'VS Code' },
    { id: 'sdk', fa: 'SDK پایتون و Node', en: 'Python & Node SDKs' },
    { id: 'curl', fa: 'cURL', en: 'cURL' },
  ] },
  { title: { fa: 'مرجع', en: 'Reference' }, icon: 'book', items: [
    { id: 'api-reference', fa: 'مرجع API', en: 'API reference' },
    { id: 'errors', fa: 'خطاها و عیب‌یابی', en: 'Errors & troubleshooting' },
    { id: 'keys', fa: 'کلیدها و امنیت', en: 'Keys & security' },
    { id: 'gateway-setup', fa: 'راه‌اندازی درگاه پرداخت (مدیر)', en: 'Payment gateway setup (admins)' },
  ] },
];

export const ALL = GROUPS.flatMap((g) => g.items);
export const ALL_IDS = ALL.map((i) => i.id);
