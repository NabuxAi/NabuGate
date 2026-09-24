import { useEffect, useRef, useState } from 'react';
import * as api from '../api.js';
import Icon from '../components/Icon.jsx';
import CodeBlock from '../components/CodeBlock.jsx';
import { LangSwitch, Logo, ThemeToggle } from '../components/Brand.jsx';
import { fmtDigits, fmtInt, useI18n, useT } from '../i18n/index.jsx';
import '../styles/landing.css';

/*
 * The public home page. Everything on it is either live (the model list comes
 * from /api/public/models) or a fact the gateway enforces — no invented
 * customer counts or uptime numbers.
 */

const T = {
  fa: {
    brand: 'نبوگیت',
    navFeatures: 'امکانات',
    navHow: 'نحوهٔ کار',
    navPricing: 'قیمت‌ها',
    navDocs: 'مستندات',
    signIn: 'ورود',
    getStarted: 'شروع کنید',
    openConsole: 'رفتن به پنل',
    menu: 'منو',
    eyebrow: 'درگاه هوش مصنوعیِ سازگار با OpenAI',
    heroA: 'یک API،',
    heroB: 'همهٔ مدل‌های هوش مصنوعی',
    heroSub: 'نبوگیت GPT، Claude، Gemini، DeepSeek و ده‌ها مدل دیگر را پشت یک آدرس و یک کلید می‌آورد؛ با مسیریابی هوشمند، fallback خودکار بین ارائه‌دهنده‌ها و پرداخت به‌ازای مصرف.',
    readDocs: 'مطالعهٔ مستندات',
    worksWith: 'سازگار با ابزارهایی که هر روز استفاده می‌کنید',
    online: 'آنلاین',
    route: 'مسیر درخواست',
    rateLimited: 'محدودیت نرخ ← بعدی',
    served: 'پاسخ داده شد',
    balance: 'موجودی',
    modelsFrom: 'دسترسی به مدل‌های',
    statModels: 'مدل و alias در دسترس همین حالا',
    statEndpoint: 'آدرس برای چت، embedding، تصویر و گفتار',
    statCode: 'خط تغییر در کد؛ فقط آدرس پایه عوض می‌شود',
    statMin: 'حداقل شارژ؛ موجودی هرگز منقضی نمی‌شود',
    featEyebrow: 'امکانات',
    featTitle: 'هر چیزی که برای ساختن با هوش مصنوعی لازم دارید',
    featSub: 'یک لایهٔ نازک و قابل‌اعتماد بین برنامهٔ شما و ارائه‌دهنده‌ها — بدون قفل‌شدن روی یک vendor.',
    f1t: 'سازگاری کامل با OpenAI',
    f1b: 'هر SDK و ابزاری که با OpenAI کار می‌کند فقط با تغییر آدرس پایه وصل می‌شود. بدنهٔ درخواست دست‌نخورده عبور می‌کند: tools، response_format، استریم و بینایی.',
    f2t: 'fallback خودکار',
    f2b: 'یک alias مثل nabu-fast زنجیره‌ای از چند ارائه‌دهنده است. اگر یکی قطع شود یا پاسخ خالی بدهد، بعدی جواب می‌دهد و کاربر شما چیزی نمی‌فهمد.',
    f3t: 'یک کلید برای هر برنامه',
    f3b: 'هر کلید را به مدل‌ها، مبدأها و سقف درخواست محدود کنید. مصرف هر کلید جدا دیده می‌شود و ابطال یکی به بقیه دست نمی‌زند.',
    f4t: 'پرداخت به‌ازای مصرف',
    f4b: 'کیف پول دلاری که با قیمت واقعی هر مدل کم می‌شود. هر پاسخ موجودی باقی‌مانده را در هدر برمی‌گرداند.',
    f5t: 'کلید خودتان (BYOK)',
    f5b: 'کلید ارائه‌دهندهٔ خودتان را با هدر X-Nabu-Key-<provider> بفرستید؛ مصرف ثبت می‌شود ولی هزینه‌ای از موجودی کم نمی‌شود و کلید هرگز ذخیره نمی‌شود.',
    f6t: 'ایجنت‌ها و فلوها',
    f6b: 'یک دستیار با system prompt آماده یا یک زنجیرهٔ چندمرحله‌ای را دقیقاً مثل یک مدل صدا بزنید — با یک درخواست از هر کلاینت OpenAI.',
    f7t: 'همهٔ مدالیته‌ها',
    f7b: 'چت، embedding، تولید تصویر، تبدیل متن به گفتار، گفتار به متن و صدای زنده — همه از همان کلید.',
    mChat: 'چت', mEmbed: 'Embedding', mImage: 'تصویر', mSpeech: 'گفتار', mStt: 'گفتار به متن', mLive: 'صدای زنده',
    howEyebrow: 'نحوهٔ کار',
    howTitle: 'در سه گام به اولین پاسخ برسید',
    howSub: 'از ثبت‌نام تا اولین درخواست API، مسیری روشن و بدون پیچیدگی.',
    s1t: 'ثبت‌نام',
    s1b: 'در کنسول نبوگیت با ایمیل، گوگل یا حساب نابو حساب بسازید.',
    s2t: 'شارژ کیف پول',
    s2b: 'هر مبلغی از ۱ دلار شارژ کنید؛ پرداخت از درگاه بانکی و به تومان.',
    s3t: 'کلید بسازید و وصل شوید',
    s3b: 'کلید API بسازید، آدرس پایه را تنظیم کنید و ابزارتان آماده است.',
    devEyebrow: 'برای توسعه‌دهنده‌ها',
    devTitle: 'با ابزارهایی که همین حالا دارید کار می‌کند',
    devSub: 'تنظیمات آماده برای هر ابزار. کلید را جایگزین کنید و تمام.',
    devMore: 'راهنمای کامل هر ابزار',
    priceEyebrow: 'قیمت‌گذاری',
    priceTitle: 'پرداخت به‌ازای مصرف، بدون اشتراک اجباری',
    priceSub: 'کیف پول دلاری شارژ می‌کنید؛ هر درخواست به قیمت واقعی مدل کم می‌شود. موجودی منقضی نمی‌شود.',
    pStarter: 'شروع', pStarterB: 'تست مدل‌ها و پروژه‌های کوچک.',
    pPro: 'حرفه‌ای', pProB: 'توسعهٔ روزمره با Cursor، Cline و Claude Code.',
    pTeam: 'تیمی', pTeamB: 'یک کلید برای هر برنامه، گزارش مصرف جدا.',
    pCredit: 'اعتبار',
    pF1: 'دسترسی به همهٔ مدل‌ها و aliasها',
    pF2: 'fallback خودکار بین ارائه‌دهنده‌ها',
    pF3: 'کلیدهای نامحدود با گزارش جدا',
    pF4: 'موجودی بدون تاریخ انقضا',
    recommended: 'پیشنهاد ما',
    choose: 'انتخاب در پنل',
    priceNote: 'هر مبلغی از ۱ تا ۵٬۰۰۰ دلار. پرداخت از درگاه بانکی به تومان با نرخ روز؛ اطلاعات کارت هرگز به ما نمی‌رسد.',
    faqEyebrow: 'سؤالات متداول',
    faqTitle: 'پاسخ پرسش‌های رایج',
    faqSub: 'جوابتان اینجا نیست؟ مستندات همه‌چیز را با جزئیات توضیح می‌دهد.',
    fullDocs: 'مستندات کامل',
    ctaTitle: 'همین امروز با همهٔ مدل‌ها بسازید',
    ctaSub: 'حساب بسازید، کلید بگیرید و اولین درخواست را در کمتر از یک دقیقه بفرستید.',
    footTag: 'دروازهٔ یکپارچهٔ هوش مصنوعی: یک API سازگار با OpenAI برای همهٔ مدل‌ها.',
    footProduct: 'محصول',
    footDev: 'توسعه‌دهنده',
    footAccount: 'حساب',
    footModels: 'مدل‌ها',
    footApi: 'مرجع API',
    footStatus: 'وضعیت سرویس',
    footSignup: 'ثبت‌نام',
    rights: 'تمامی حقوق محفوظ است.',
  },
  en: {
    brand: 'NabuGate',
    navFeatures: 'Features',
    navHow: 'How it works',
    navPricing: 'Pricing',
    navDocs: 'Docs',
    signIn: 'Sign in',
    getStarted: 'Get started',
    openConsole: 'Open console',
    menu: 'Menu',
    eyebrow: 'OpenAI-compatible AI gateway',
    heroA: 'One API.',
    heroB: 'Every AI model.',
    heroSub: 'NabuGate puts GPT, Claude, Gemini, DeepSeek and dozens more behind one URL and one key — with smart routing, automatic cross-provider fallback and pay-as-you-go billing.',
    readDocs: 'Read the docs',
    worksWith: 'Works with the tools you already use',
    online: 'Online',
    route: 'Request route',
    rateLimited: 'rate limited → next',
    served: 'served',
    balance: 'Balance',
    modelsFrom: 'Models from',
    statModels: 'models & aliases available right now',
    statEndpoint: 'endpoint for chat, embeddings, images and speech',
    statCode: 'code changes — only the base URL moves',
    statMin: 'minimum top-up; credit never expires',
    featEyebrow: 'Features',
    featTitle: 'Everything you need to build with AI',
    featSub: 'A thin, dependable layer between your app and the providers — without locking you to one vendor.',
    f1t: 'Drop-in OpenAI compatible',
    f1b: 'Any SDK or tool that speaks OpenAI connects by changing the base URL. Requests pass through untouched: tools, response_format, streaming and vision.',
    f2t: 'Automatic fallback',
    f2b: 'An alias like nabu-fast is a chain across vendors. If one is down or answers empty, the next one replies — your users never notice.',
    f3t: 'One key per app',
    f3b: 'Scope each key to models, origins and a rate limit. See spend per key, and revoke one without touching the rest.',
    f4t: 'Pay only for usage',
    f4b: 'A USD wallet charged at each model’s real price. Every response reports your remaining balance in a header.',
    f5t: 'Bring your own key',
    f5b: 'Send your own provider key with X-Nabu-Key-<provider>: usage is metered but nothing is charged here, and the key is never stored.',
    f6t: 'Agents & flows',
    f6b: 'Call a ready-made assistant or a multi-step chain exactly like a model — one request from any OpenAI client.',
    f7t: 'Every modality',
    f7b: 'Chat, embeddings, image generation, text-to-speech, transcription and realtime voice — all from the same key.',
    mChat: 'Chat', mEmbed: 'Embeddings', mImage: 'Images', mSpeech: 'Speech', mStt: 'Transcription', mLive: 'Realtime voice',
    howEyebrow: 'How it works',
    howTitle: 'Your first response in three steps',
    howSub: 'From sign-up to your first API call, a clear path with no detours.',
    s1t: 'Sign up',
    s1b: 'Create an account in the NabuGate console with email, Google or Nabu.',
    s2t: 'Top up your wallet',
    s2b: 'Add any amount from $1; paid through the bank gateway.',
    s3t: 'Create a key & connect',
    s3b: 'Create an API key, set the base URL, and your tool is ready.',
    devEyebrow: 'For developers',
    devTitle: 'Works with the tools you already have',
    devSub: 'Ready-made settings for each tool. Paste your key and you’re done.',
    devMore: 'Full guide for every tool',
    priceEyebrow: 'Pricing',
    priceTitle: 'Pay as you go, no forced subscription',
    priceSub: 'Top up a USD wallet; each request deducts the model’s real price. Credit never expires.',
    pStarter: 'Starter', pStarterB: 'Try the models, small projects.',
    pPro: 'Pro', pProB: 'Daily development with Cursor, Cline and Claude Code.',
    pTeam: 'Team', pTeamB: 'One key per app, separate usage reports.',
    pCredit: 'credit',
    pF1: 'Every model and alias',
    pF2: 'Automatic cross-provider fallback',
    pF3: 'Unlimited keys with separate reports',
    pF4: 'Credit that never expires',
    recommended: 'Recommended',
    choose: 'Choose in console',
    priceNote: 'Any amount from $1 to $5,000. Paid on the bank’s page in Toman at the day’s rate; card details never reach us.',
    faqEyebrow: 'FAQ',
    faqTitle: 'Common questions',
    faqSub: 'Not here? The docs cover everything in detail.',
    fullDocs: 'Full documentation',
    ctaTitle: 'Start building with every model today',
    ctaSub: 'Create an account, get a key and send your first request in under a minute.',
    footTag: 'The unified AI gateway: one OpenAI-compatible API for every model.',
    footProduct: 'Product',
    footDev: 'Developers',
    footAccount: 'Account',
    footModels: 'Models',
    footApi: 'API reference',
    footStatus: 'Service status',
    footSignup: 'Sign up',
    rights: 'All rights reserved.',
  },
};

const FAQ = {
  fa: [
    ['NabuGate چیست؟', 'یک درگاه سازگار با OpenAI که پشت یک آدرس و یک کلید، مدل‌های OpenAI، Anthropic، Google و بقیه را با fallback خودکار ارائه می‌دهد.'],
    ['با Cursor و Claude Code کار می‌کند؟', 'بله. Cursor، Cline، Roo Code، Codex و SDK‌های OpenAI با تغییر آدرس پایه وصل می‌شوند؛ Claude Code با متغیرهای ANTHROPIC_*. راهنمای هر کدام در مستندات است.'],
    ['هزینه چطور حساب می‌شود؟', 'کیف پول دلاری. هر درخواست به قیمت واقعی مدل (به‌ازای میلیون توکن) کم می‌شود و هر پاسخ موجودی باقی‌مانده را در هدر برمی‌گرداند.'],
    ['اگر پرداختم مشکل داشت؟', 'فاکتور پیش از رفتن به بانک به نام حساب شما ثبت می‌شود و پنل هر بار از خودِ درگاه می‌پرسد. پول کم‌شده‌ای که تأیید نشود ظرف ۷۲ ساعت برمی‌گردد. بخش «مشکلات پرداخت» در مستندات همه را پوشش می‌دهد.'],
    ['کلید API چطور کار می‌کند؟', 'برای هر برنامه یک کلید می‌سازید، با محدودیت مدل، مبدأ و سقف درخواست. متن کامل کلید فقط یک‌بار نمایش داده می‌شود.'],
  ],
  en: [
    ['What is NabuGate?', 'An OpenAI-compatible gateway: one URL and one key for models from OpenAI, Anthropic, Google and more, with automatic fallback.'],
    ['Does it work with Cursor and Claude Code?', 'Yes. Cursor, Cline, Roo Code, Codex and the OpenAI SDKs connect by changing the base URL; Claude Code through the ANTHROPIC_* variables. The docs have a guide for each.'],
    ['How is it billed?', 'A USD wallet. Each request deducts the model’s real per-million-token price, and every response reports the remaining balance in a header.'],
    ['What if my payment fails?', 'The invoice is bound to your account before you leave for the bank, and the console re-asks the gateway on every visit. An unconfirmed charge is reversed within 72 hours. “Payment issues” in the docs covers every case.'],
    ['How do API keys work?', 'One key per app, limited by model, origin and rate. The full key is shown only once.'],
  ],
};

// Brand dots for the provider strip. Names only — no logos to keep in sync.
const PROVIDERS = [
  ['OpenAI', '#10a37f'], ['Anthropic', '#d97757'], ['Google Gemini', '#4f8cff'], ['DeepSeek', '#4d6bfe'],
  ['Groq', '#f55036'], ['OpenRouter', '#6467f2'], ['Mistral', '#fa520f'], ['xAI', '#94a3b8'],
  ['Cloudflare', '#f38020'], ['TokenRouter', '#22c55e'], ['AvalAI', '#0ea5e9'], ['GapGPT', '#a855f7'],
  ['ArvanCloud', '#00b4b0'], ['ElevenLabs', '#cbd5e1'], ['Pexels', '#05a081'],
];

const TOOLS = ['Claude Code', 'Cursor', 'Codex', 'Cline', 'Roo Code', 'Continue', 'VS Code', 'OpenAI SDK'];

function snippets(ORIGIN, BASE, KEY, hello) {
  return [
    { id: 'curl', label: 'cURL', icon: 'terminal', file: 'bash', code: `curl ${BASE}/chat/completions \\\n  -H "Authorization: Bearer ${KEY}" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"nabu-fast","messages":[{"role":"user","content":"${hello}"}]}'` },
    { id: 'python', label: 'Python', icon: 'code', file: 'python', code: `from openai import OpenAI\n\nclient = OpenAI(base_url="${BASE}", api_key="${KEY}")\n\nr = client.chat.completions.create(\n    model="nabu-smart",\n    messages=[{"role": "user", "content": "${hello}"}],\n)\nprint(r.choices[0].message.content)` },
    { id: 'node', label: 'Node.js', icon: 'code', file: 'node', code: `import OpenAI from "openai";\n\nconst client = new OpenAI({ baseURL: "${BASE}", apiKey: "${KEY}" });\n\nconst r = await client.chat.completions.create({\n  model: "nabu-smart",\n  messages: [{ role: "user", content: "${hello}" }],\n});` },
    { id: 'claude', label: 'Claude Code', icon: 'terminal', file: '~/.claude/settings.json', code: `{\n  "env": {\n    "ANTHROPIC_BASE_URL": "${ORIGIN}",\n    "ANTHROPIC_API_KEY": "${KEY}",\n    "ANTHROPIC_AUTH_TOKEN": "${KEY}",\n    "ANTHROPIC_MODEL": "nabu-smart",\n    "ANTHROPIC_SMALL_FAST_MODEL": "nabu-fast"\n  }\n}` },
    { id: 'cursor', label: 'Cursor', icon: 'wand', file: 'Settings → Models', code: `OpenAI API Key          ${KEY}\nOverride OpenAI Base URL ${BASE}\nAdd model               nabu-smart` },
    { id: 'codex', label: 'Codex CLI', icon: 'terminal', file: '~/.codex/config.toml', code: `model = "nabu-smart"\nmodel_provider = "nabugate"\n\n[model_providers.nabugate]\nname = "NabuGate"\nbase_url = "${BASE}"\nenv_key = "OPENAI_API_KEY"\nwire_api = "responses"` },
    { id: 'cline', label: 'Cline / Roo', icon: 'bot', file: 'settings', code: `API Provider : OpenAI Compatible\nBase URL     : ${BASE}\nAPI Key      : ${KEY}\nModel ID     : nabu-smart` },
  ];
}

// Sections fade up as they scroll into view. Everything is visible without
// JS or under reduced motion; the class only adds the entrance.
function useReveal() {
  useEffect(() => {
    const els = [...document.querySelectorAll('.lp-reveal')];
    if (typeof IntersectionObserver === 'undefined' || window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      els.forEach((el) => el.classList.add('in'));
      return undefined;
    }
    const io = new IntersectionObserver(
      (entries) => entries.forEach((e) => {
        if (e.isIntersecting) { e.target.classList.add('in'); io.unobserve(e.target); }
      }),
      { rootMargin: '0px 0px -8% 0px', threshold: 0.08 },
    );
    els.forEach((el) => io.observe(el));
    return () => io.disconnect();
  }, []);
}

function Section({ id, eyebrow, title, sub, children, className = '' }) {
  return (
    <section id={id} className={'lp-section ' + className}>
      <div className="lp-container">
        <header className="lp-section-head lp-reveal">
          <span className="lp-eyebrow">{eyebrow}</span>
          <h2>{title}</h2>
          {sub && <p>{sub}</p>}
        </header>
        {children}
      </div>
    </section>
  );
}

export default function Landing({ signedIn = false }) {
  const t = useT(T);
  const { lang } = useI18n();
  const [models, setModels] = useState([]);
  const [menu, setMenu] = useState(false);
  const [tab, setTab] = useState('curl');
  const [scrolled, setScrolled] = useState(false);
  const heroRef = useRef(null);
  useReveal();

  useEffect(() => {
    api.publicModels().then((names) => setModels(Array.isArray(names) ? names : [])).catch(() => {});
    const onScroll = () => setScrolled(window.scrollY > 8);
    onScroll();
    window.addEventListener('scroll', onScroll, { passive: true });
    return () => window.removeEventListener('scroll', onScroll);
  }, []);

  // The hero card leans a few degrees toward the pointer. Transform only, and
  // skipped entirely for touch and reduced motion.
  useEffect(() => {
    const el = heroRef.current;
    if (!el || window.matchMedia('(pointer: coarse), (prefers-reduced-motion: reduce)').matches) return undefined;
    const onMove = (e) => {
      const r = el.getBoundingClientRect();
      const x = (e.clientX - r.left) / r.width - 0.5;
      const y = (e.clientY - r.top) / r.height - 0.5;
      el.style.setProperty('--rx', `${(-y * 5).toFixed(2)}deg`);
      el.style.setProperty('--ry', `${(x * 7).toFixed(2)}deg`);
    };
    const onLeave = () => { el.style.setProperty('--rx', '0deg'); el.style.setProperty('--ry', '0deg'); };
    el.addEventListener('pointermove', onMove);
    el.addEventListener('pointerleave', onLeave);
    return () => { el.removeEventListener('pointermove', onMove); el.removeEventListener('pointerleave', onLeave); };
  }, []);

  const ORIGIN = window.location.origin.startsWith('http') ? window.location.origin : 'https://gate.nabuxai.com';
  const BASE = `${ORIGIN}/v1`;
  const KEY = 'ng_xxxxxxxxxxxxxxxx';
  const hello = lang === 'fa' ? 'سلام!' : 'Hello!';
  const snips = snippets(ORIGIN, BASE, KEY, hello);
  const snip = snips.find((s) => s.id === tab) || snips[0];

  const cta = signedIn ? '/panel/' : '/panel/login';
  const ctaLabel = signedIn ? t('openConsole') : t('getStarted');

  // Live model names for the marquee; a fixed handful until the list lands.
  const modelNames = models.length ? models.slice(0, 24) : ['nabu-fast', 'nabu-smart', 'nabu-embed', 'gpt-5', 'claude-sonnet', 'gemini-2.5-pro', 'deepseek-chat', 'llama-4'];

  const plans = [
    { name: t('pStarter'), amount: 5, blurb: t('pStarterB') },
    { name: t('pPro'), amount: 25, hot: true, blurb: t('pProB') },
    { name: t('pTeam'), amount: 100, blurb: t('pTeamB') },
  ];

  const nav = [
    ['#features', t('navFeatures')],
    ['#how', t('navHow')],
    ['#pricing', t('navPricing')],
    ['/docs', t('navDocs')],
  ];

  return (
    <div className="lp">
      <div className="lp-bg" aria-hidden="true">
        <span className="lp-orb a" /><span className="lp-orb b" /><span className="lp-orb c" />
        <span className="lp-grid" />
      </div>

      <header className={'lp-header' + (scrolled ? ' scrolled' : '') + (menu ? ' open' : '')}>
        <div className="lp-container lp-header-in">
          <a href="/" className="lp-logo" aria-label="NabuGate">
            <Logo size={34} />
            <span>{t('brand')}</span>
          </a>
          <nav className="lp-nav">
            {nav.map(([href, label]) => <a key={href} href={href} onClick={() => setMenu(false)}>{label}</a>)}
          </nav>
          <div className="lp-actions">
            <LangSwitch size="sm" />
            <ThemeToggle />
            {!signedIn && <a href="/panel/login" className="lp-btn lp-btn-ghost lp-hide-sm">{t('signIn')}</a>}
            <a href={cta} className="lp-btn lp-btn-primary lp-hide-sm">{ctaLabel}</a>
            <button type="button" className="icon-btn lp-menu-btn" onClick={() => setMenu((m) => !m)} aria-expanded={menu} aria-label={t('menu')}>
              <Icon name={menu ? 'x' : 'menu'} size={18} />
            </button>
          </div>
        </div>
        {menu && (
          <div className="lp-mobile-menu">
            {nav.map(([href, label]) => <a key={href} href={href} onClick={() => setMenu(false)}>{label}</a>)}
            <div className="lp-mobile-cta">
              {!signedIn && <a href="/panel/login" className="lp-btn lp-btn-outline">{t('signIn')}</a>}
              <a href={cta} className="lp-btn lp-btn-primary">{ctaLabel}</a>
            </div>
          </div>
        )}
      </header>

      <main>
        {/* ── Hero ─────────────────────────────────────────────────────────── */}
        <section className="lp-hero">
          <div className="lp-container lp-hero-in">
            <div className="lp-hero-copy">
              <span className="lp-pill lp-reveal">
                <span className="lp-live" aria-hidden="true" />
                {t('eyebrow')}
              </span>
              <h1 className="lp-reveal">
                {t('heroA')}<br />
                <span className="lp-grad">{t('heroB')}</span>
              </h1>
              <p className="lp-lead lp-reveal">{t('heroSub')}</p>
              <div className="lp-hero-cta lp-reveal">
                <a href={cta} className="lp-btn lp-btn-primary lp-btn-lg">
                  {ctaLabel}<Icon name="arrow" size={18} className="flip-rtl" />
                </a>
                <a href="/docs" className="lp-btn lp-btn-outline lp-btn-lg"><Icon name="book" size={18} />{t('readDocs')}</a>
              </div>
              <div className="lp-tools lp-reveal">
                <span>{t('worksWith')}</span>
                <div>{TOOLS.map((tool) => <span key={tool} className="lp-chip" dir="ltr">{tool}</span>)}</div>
              </div>
            </div>

            <div className="lp-hero-visual lp-reveal" ref={heroRef}>
              <div className="lp-window">
                <div className="lp-window-bar">
                  <span className="lp-dots"><i /><i /><i /></span>
                  <span className="lp-endpoint" dir="ltr">POST {BASE.replace(/^https?:\/\//, '')}/chat/completions</span>
                  <span className="lp-online"><span className="lp-live" />{t('online')}</span>
                </div>
                <pre className="lp-code" dir="ltr"><code>
                  <span className="k">const</span> r = <span className="k">await</span> client.chat.completions.<span className="f">create</span>({'{\n'}
                  {'  '}model: <span className="s">"nabu-smart"</span>,{'\n'}
                  {'  '}messages: [{'{'} role: <span className="s">"user"</span>, content: <span className="s">"{hello}"</span> {'}'}],{'\n'}
                  {'}'});
                </code></pre>
                <div className="lp-trace">
                  <div className="lp-trace-head">
                    <Icon name="route" size={14} />{t('route')}
                    <code dir="ltr">nabu-smart</code>
                  </div>
                  <div className="lp-trace-row fail">
                    <span className="n">{fmtDigits(1)}</span>
                    <code dir="ltr">openai/gpt-…</code>
                    <span className="st"><b dir="ltr">429</b> {t('rateLimited')}</span>
                  </div>
                  <div className="lp-trace-row ok">
                    <span className="n">{fmtDigits(2)}</span>
                    <code dir="ltr">anthropic/claude-…</code>
                    <span className="st"><b dir="ltr">200 · 812ms</b> <Icon name="check" size={13} stroke={2.6} />{t('served')}</span>
                  </div>
                </div>
                <div className="lp-window-foot" dir="ltr">
                  <span>X-Nabu-Balance-USD: <b>24.18</b></span>
                  <span className="lp-bearer">Bearer ng_••••</span>
                </div>
              </div>
              <div className="lp-float lp-float-a"><Icon name="zap" size={15} />fallback</div>
              <div className="lp-float lp-float-b"><Icon name="shield" size={15} />allow-list</div>
            </div>
          </div>

          {/* ── Provider marquee ── */}
          <div className="lp-marquee-wrap lp-reveal">
            <p className="lp-marquee-label">{t('modelsFrom')}</p>
            <div className="lp-marquee" dir="ltr">
              <div className="lp-marquee-track">
                {[...PROVIDERS, ...PROVIDERS].map(([name, color], i) => (
                  <span key={i} className="lp-prov" aria-hidden={i >= PROVIDERS.length ? 'true' : undefined}>
                    <i style={{ background: color }} />{name}
                  </span>
                ))}
              </div>
            </div>
            <div className="lp-marquee lp-marquee-models" dir="ltr">
              <div className="lp-marquee-track reverse">
                {[...modelNames, ...modelNames].map((m, i) => (
                  <code key={i} className="lp-model" aria-hidden={i >= modelNames.length ? 'true' : undefined}>{m}</code>
                ))}
              </div>
            </div>
          </div>
        </section>

        {/* ── Stats ─────────────────────────────────────────────────────────── */}
        <section className="lp-stats-band">
          <div className="lp-container">
            <div className="lp-stats lp-reveal">
              <div><strong>{models.length ? fmtInt(models.length) : '—'}</strong><span>{t('statModels')}</span></div>
              <div><strong>{fmtInt(1)}</strong><span>{t('statEndpoint')}</span></div>
              <div><strong>{fmtInt(0)}</strong><span>{t('statCode')}</span></div>
              <div><strong dir="ltr">{fmtDigits('$1')}</strong><span>{t('statMin')}</span></div>
            </div>
          </div>
        </section>

        {/* ── Features ──────────────────────────────────────────────────────── */}
        <Section id="features" eyebrow={t('featEyebrow')} title={t('featTitle')} sub={t('featSub')}>
          <div className="lp-bento">
            <article className="lp-card lp-span-2 lp-reveal">
              <div className="lp-card-icon"><Icon name="plug" size={20} /></div>
              <h3>{t('f1t')}</h3>
              <p>{t('f1b')}</p>
              <pre className="lp-diff" dir="ltr"><code>
                <span className="del">- base_url = "https://api.openai.com/v1"</span>{'\n'}
                <span className="add">+ base_url = "{BASE}"</span>
              </code></pre>
            </article>
            <article className="lp-card lp-reveal">
              <div className="lp-card-icon"><Icon name="route" size={20} /></div>
              <h3>{t('f2t')}</h3>
              <p>{t('f2b')}</p>
              <div className="lp-chain" dir="ltr">
                <code className="alias">nabu-fast</code>
                <Icon name="arrow" size={14} />
                <code>groq</code><code>gemini</code><code>openai</code>
              </div>
            </article>
            <article className="lp-card lp-reveal">
              <div className="lp-card-icon"><Icon name="key" size={20} /></div>
              <h3>{t('f3t')}</h3>
              <p>{t('f3b')}</p>
              <div className="lp-kv" dir="ltr">
                <span>allow</span><code>nabu-* · cine-*</code>
                <span>origin</span><code>app.example.com</code>
                <span>rate</span><code>120/min</code>
              </div>
            </article>
            <article className="lp-card lp-reveal">
              <div className="lp-card-icon"><Icon name="wallet" size={20} /></div>
              <h3>{t('f4t')}</h3>
              <p>{t('f4b')}</p>
              <pre className="lp-diff" dir="ltr"><code><span className="hdr">X-Nabu-Balance-USD: 4.1837</span></code></pre>
            </article>
            <article className="lp-card lp-reveal">
              <div className="lp-card-icon"><Icon name="lock" size={20} /></div>
              <h3>{t('f5t')}</h3>
              <p>{t('f5b')}</p>
            </article>
            <article className="lp-card lp-span-md-2 lp-reveal">
              <div className="lp-card-icon"><Icon name="flow" size={20} /></div>
              <h3>{t('f6t')}</h3>
              <p>{t('f6b')}</p>
            </article>
            <article className="lp-card lp-span-2 lp-reveal">
              <div>
                <div className="lp-card-icon"><Icon name="layers" size={20} /></div>
                <h3>{t('f7t')}</h3>
                <p>{t('f7b')}</p>
              </div>
              <div className="lp-modalities">
                {[['activity', 'mChat', '/v1/chat/completions'], ['box', 'mEmbed', '/v1/embeddings'], ['image', 'mImage', '/v1/images/generations'], ['mic', 'mSpeech', '/v1/audio/speech'], ['receipt', 'mStt', '/v1/audio/transcriptions'], ['zap', 'mLive', '/v1/live/sessions']].map(([icon, key, path]) => (
                  <div key={key} className="lp-modality">
                    <Icon name={icon} size={18} />
                    <strong>{t(key)}</strong>
                    <code dir="ltr">{path}</code>
                  </div>
                ))}
              </div>
            </article>
          </div>
        </Section>

        {/* ── How it works ──────────────────────────────────────────────────── */}
        <Section id="how" eyebrow={t('howEyebrow')} title={t('howTitle')} sub={t('howSub')} className="lp-alt">
          <ol className="lp-steps">
            {[['user', 's1t', 's1b'], ['card', 's2t', 's2b'], ['key', 's3t', 's3b']].map(([icon, tt, bb], i) => (
              <li key={tt} className="lp-step lp-reveal" style={{ transitionDelay: `${i * 90}ms` }}>
                <span className="lp-step-num">{fmtInt(i + 1)}</span>
                <div className="lp-step-icon"><Icon name={icon} size={22} /></div>
                <h3>{t(tt)}</h3>
                <p>{t(bb)}</p>
              </li>
            ))}
          </ol>
        </Section>

        {/* ── Developers ────────────────────────────────────────────────────── */}
        <Section id="developers" eyebrow={t('devEyebrow')} title={t('devTitle')} sub={t('devSub')}>
          <div className="lp-dev lp-reveal">
            <div className="lp-dev-tabs" role="tablist">
              {snips.map((s) => (
                <button key={s.id} type="button" role="tab" aria-selected={tab === s.id} className={'lp-dev-tab' + (tab === s.id ? ' active' : '')} onClick={() => setTab(s.id)}>
                  <Icon name={s.icon} size={16} />
                  <span dir="ltr">{s.label}</span>
                </button>
              ))}
              <a href="/docs#env" className="lp-dev-more">{t('devMore')}<Icon name="arrow" size={14} className="flip-rtl" /></a>
            </div>
            <div className="lp-dev-code">
              <CodeBlock key={snip.id} code={snip.code} label={snip.file} />
            </div>
          </div>
        </Section>

        {/* ── Pricing ───────────────────────────────────────────────────────── */}
        <Section id="pricing" eyebrow={t('priceEyebrow')} title={t('priceTitle')} sub={t('priceSub')} className="lp-alt">
          <div className="lp-plans">
            {plans.map((p, i) => (
              <a href={signedIn ? '/panel/plans' : '/panel/login'} key={p.amount} className={'lp-plan lp-reveal' + (p.hot ? ' hot' : '')} style={{ transitionDelay: `${i * 80}ms` }}>
                {p.hot && <span className="lp-plan-ribbon">{t('recommended')}</span>}
                <strong className="lp-plan-name">{p.name}</strong>
                <span className="lp-plan-price"><span dir="ltr">{fmtDigits('$' + p.amount)}</span><small>{t('pCredit')}</small></span>
                <p>{p.blurb}</p>
                <ul>
                  {['pF1', 'pF2', 'pF3', 'pF4'].map((k) => <li key={k}><Icon name="check" size={15} stroke={2.4} />{t(k)}</li>)}
                </ul>
                <span className={'lp-btn ' + (p.hot ? 'lp-btn-primary' : 'lp-btn-outline')}>{t('choose')}</span>
              </a>
            ))}
          </div>
          <p className="lp-price-note">{t('priceNote')}</p>
        </Section>

        {/* ── FAQ ───────────────────────────────────────────────────────────── */}
        <section id="faq" className="lp-section">
          <div className="lp-container lp-faq">
            <header className="lp-faq-head lp-reveal">
              <span className="lp-eyebrow">{t('faqEyebrow')}</span>
              <h2>{t('faqTitle')}</h2>
              <p>{t('faqSub')}</p>
              <a href="/docs" className="lp-btn lp-btn-outline"><Icon name="book" size={16} />{t('fullDocs')}</a>
            </header>
            <div className="lp-faq-list lp-reveal">
              {(FAQ[lang] || FAQ.fa).map(([q, a]) => (
                <details key={q} className="lp-faq-item">
                  <summary>{q}<Icon name="plus" size={18} /></summary>
                  <p>{a}</p>
                </details>
              ))}
            </div>
          </div>
        </section>

        {/* ── Final call to action ─────────────────────────────────────────── */}
        <section className="lp-section lp-cta-wrap">
          <div className="lp-container">
            <div className="lp-cta lp-reveal">
              <div>
                <h2>{t('ctaTitle')}</h2>
                <p>{t('ctaSub')}</p>
              </div>
              <div className="lp-cta-btns">
                <a href={cta} className="lp-btn lp-btn-light lp-btn-lg">{ctaLabel}<Icon name="arrow" size={18} className="flip-rtl" /></a>
                <a href="/docs" className="lp-btn lp-btn-glass lp-btn-lg">{t('readDocs')}</a>
              </div>
            </div>
          </div>
        </section>
      </main>

      <footer className="lp-footer">
        <div className="lp-container">
          <div className="lp-footer-top">
            <div className="lp-footer-brand">
              <a href="/" className="lp-logo"><Logo size={32} /><span>{t('brand')}</span></a>
              <p>{t('footTag')}</p>
            </div>
            <div className="lp-footer-cols">
              <div>
                <h4>{t('footProduct')}</h4>
                <a href="#features">{t('navFeatures')}</a>
                <a href="#pricing">{t('navPricing')}</a>
                <a href="/docs#models">{t('footModels')}</a>
              </div>
              <div>
                <h4>{t('footDev')}</h4>
                <a href="/docs">{t('navDocs')}</a>
                <a href="/docs#api-reference">{t('footApi')}</a>
                <a href="/healthz">{t('footStatus')}</a>
              </div>
              <div>
                <h4>{t('footAccount')}</h4>
                <a href="/panel/login">{t('signIn')}</a>
                <a href="/panel/login">{t('footSignup')}</a>
              </div>
            </div>
          </div>
          <div className="lp-footer-bottom">
            <span>© {fmtDigits(new Date().getFullYear())} NabuGate. {t('rights')}</span>
            <div className="lp-footer-controls"><LangSwitch size="sm" /><ThemeToggle /></div>
          </div>
        </div>
      </footer>
    </div>
  );
}
