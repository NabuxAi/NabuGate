/*
 * Representative console data. Mirrors the shape of NabuGate's real config
 * (providers, alias routes, per-key policy) and its /v1/usage output, so these
 * structures can later be swapped for live calls to GET /v1/models and
 * GET /v1/usage with no change to the views. Values here are illustrative.
 */

// ---- Persian-digit formatting (UI is fa-IR; the design uses ٫ / ٬ separators) ----
const FA = ['۰', '۱', '۲', '۳', '۴', '۵', '۶', '۷', '۸', '۹'];
export const faDigits = (s) =>
  String(s)
    .replace(/[0-9]/g, (d) => FA[+d])
    .replace(/\./g, '٫')
    .replace(/,/g, '٬');
export const faInt = (n) => faDigits(Number(n).toLocaleString('en-US'));

export const stats = [
  { label: 'درخواست امروز', value: faInt(12480) },
  { label: 'توکن مصرفی', value: faDigits('8.6M') },
  { label: 'هزینه امروز', value: faDigits('$14.2'), tone: 'ok', ltr: true },
  { label: 'پرووایدر فعال', value: faDigits('10 / 12') },
  { label: 'نرخ فالبک', value: faDigits('3.2%'), tone: 'warn' },
];

export const providers = [
  { name: 'dahl', tag: 'پیش‌فرض', tagKind: 'default', url: 'inference.dahl.global/v1', on: true },
  { name: 'openai', tag: 'openai', tagKind: 'plain', url: 'api.openai.com/v1', on: true },
  { name: 'groq', tag: 'openai', tagKind: 'plain', url: 'api.groq.com/openai/v1', on: true },
  { name: 'anthropic', tag: 'anthropic', tagKind: 'plain', url: 'api.anthropic.com/v1', on: true },
  { name: 'gemini', tag: 'gemini', tagKind: 'plain', url: 'generativelanguage.googleapis…', on: true },
  { name: 'openrouter', tag: 'openai', tagKind: 'plain', url: 'openrouter.ai/api/v1', on: true },
  { name: 'parspack', tag: 'passthrough', tagKind: 'pass', url: 'my.parspack.com/…', on: true },
  { name: 'avalai', tag: 'passthrough', tagKind: 'pass', url: 'api.avalai.ir/v1', on: true },
  { name: 'gapgpt', tag: 'passthrough', tagKind: 'pass', url: 'api.gapgpt.app/v1', on: true },
  { name: 'pexels', tag: 'pexels', tagKind: 'plain', url: 'api.pexels.com/v1', on: true },
  { name: 'arvan', tag: 'apikey', tagKind: 'plain', url: 'اندپوینت تنظیم‌نشده', on: false },
  { name: 'ollama', tag: 'local', tagKind: 'plain', url: 'OLLAMA_BASE_URL خالی', on: false, urlLtr: true },
];

export const usageByProject = [
  { project: 'nabugen', requests: 9320, cost: '$9.84' },
  { project: 'crm', requests: 2640, cost: '$3.12' },
  { project: 'cinema', requests: 520, cost: '$1.24' },
];

export const health = [
  { k: 'آپ‌تایم', v: '99.98%' },
  { k: 'میانگین تأخیر', v: '940ms', ltr: true },
  { k: 'p95 تأخیر', v: '2.1s', ltr: true },
  { k: 'نرخ خطای ۲۴ساعت', v: '0.6%', warn: true },
];

// alias → primary target + ordered fallback chain (from models: in config.yaml)
export const chatAliases = [
  { alias: 'nabu-fast', primary: 'dahl/MiniMax-M2.7', fallbacks: ['groq/llama-3.1-70b', 'openai/gpt-4o-mini', 'anthropic/claude-3-5-haiku'], on: true },
  { alias: 'nabu-smart', primary: 'dahl/Kimi-K2.6', fallbacks: ['openai/gpt-4o', 'anthropic/claude-3-5-sonnet', 'gemini/1.5-pro'], on: true },
  { alias: 'nabu-cheap', primary: 'openrouter/llama-3.1-8b', fallbacks: ['groq/llama-3.1-8b-instant', 'dahl/MiniMax-M2.7'], on: true },
  { alias: 'nabu-vision', primary: 'openai/gpt-4o', fallbacks: ['gemini/1.5-pro'], on: true },
  { alias: 'nabu-parspack', primary: 'parspack/openai/gpt-5.5', fallbacks: ['parspack/claude-sonnet-4.6', 'parspack/gemini-2.5-flash'], on: true },
  { alias: 'nabu-avalai', primary: 'avalai/gpt-5.5', fallbacks: ['avalai/gemini-2.5-flash'], on: true },
  { alias: 'nabu-gap', primary: 'gapgpt/gpt-5.6-luna', fallbacks: ['gapgpt/claude-sonnet-5'], on: true },
  { alias: 'nabu-local', primary: 'ollama/llama3.1', fallbacks: [], note: 'بدون فالبک', on: false },
  { alias: 'nabu-arvan', primary: 'arvan/DeepSeek-R1', fallbacks: [], note: 'apikey · بدون فالبک', on: false },
];

export const mediaAliases = {
  image: [
    { alias: 'nabu-image', chain: ['openai/gpt-image-1', 'gemini/2.5-flash-image', 'pexels/search'] },
    { alias: 'nabu-photo', chain: ['pexels/search'] },
  ],
  audio: [{ alias: 'nabu-voice', chain: ['openai/gpt-4o-mini-tts', 'gemini/2.5-flash-tts'] }],
  embed: [{ alias: 'nabu-embed', chain: ['openai/text-embedding-3-small', 'gemini/text-embedding-004'] }],
};

export const keys = [
  { key: 'admin_key', full: true, project: '—', allow: ['*'], rate: 'نامحدود' },
  { key: 'nabugen_prod_•••8f2', project: 'nabugen', allow: ['nabu-smart', 'nabu-image', 'nabu-voice', 'nabu-photo', 'nabu-embed'], rate: faInt(240) },
  { key: 'crm_prod_•••a19', project: 'crm', allow: ['nabu-fast', 'nabu-embed'], rate: faInt(120) },
  { key: 'cinema_•••c07', project: 'cinema', allow: ['cine-*', 'nabu-smart'], rate: faInt(60) },
];

export const usageByModel = [
  { model: 'openai/gpt-4o', requests: 3120, tokens: '4.2M', cost: '$7.80' },
  { model: 'anthropic/claude-3-5-sonnet', requests: 1480, tokens: '2.1M', cost: '$3.90' },
  { model: 'gemini/gemini-1.5-pro', requests: 960, tokens: '1.3M', cost: '$1.62' },
  { model: 'groq/llama-3.1-70b', requests: 2840, tokens: '3.6M', cost: '$0.90' },
  { model: 'dahl/MiniMax-M2.7', requests: 4080, tokens: '5.5M', cost: null },
];

export const pricing = [
  { model: 'openai/gpt-4o', price: '2.5 / 10' },
  { model: 'openai/gpt-4o-mini', price: '0.15 / 0.6' },
  { model: 'claude-3-5-sonnet', price: '3 / 15' },
  { model: 'gemini-1.5-pro', price: '1.25 / 5' },
  { model: 'groq/llama-3.1-70b', price: '0.59 / 0.79' },
];
export const totalToday = '$14.22';

export const nav = [
  { id: 'dashboard', label: 'داشبورد', icon: '▦' },
  { id: 'providers', label: 'پرووایدرها', icon: '◉' },
  { id: 'models', label: 'مدل‌ها و آلیاس‌ها', icon: '✦' },
  { id: 'keys', label: 'کلیدهای پروژه', icon: '▣' },
  { id: 'usage', label: 'مصرف و هزینه', icon: '▤' },
  { id: 'agents', label: 'ساب‌اجنت‌ها', icon: '◈' },
  { id: 'logs', label: 'لاگ‌ها', icon: '➤' },
];
