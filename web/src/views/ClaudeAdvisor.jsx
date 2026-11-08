import { useState } from 'react';
import { fmtInt, fmtNum, useT } from '../i18n/index.jsx';
import '../styles/claude-advisor.css';

// Public Anthropic API reference rates in USD per million tokens, Nov 2026.
// Not a quote from NabuGate: the deployment may use different providers,
// markups, caching rules and model prices. Revalidate before publication.
const RATES = [
  { id: 'haiku', label: 'Haiku 5.5 (prompt <=100k)', input: 0.10, output: 0.50 },
  { id: 'sonnet', label: 'Sonnet 5.5', input: 2, output: 10 },
  { id: 'opus', label: 'Opus 5.5', input: 4, output: 20 },
];
const WORKLOADS = [
  { id: 'light', input: 1500, output: 300 },
  { id: 'coding', input: 8500, output: 1800 },
  { id: 'heavy', input: 26000, output: 4000 },
];
const CREDIT_OPTIONS = [5, 25, 100];

const T = {
  fa: {
    kicker: 'ایران بزرگ · محاسبه‌گر مصرف',
    title: 'روزانه چند ساعت با هوش مصنوعی کار می‌کنید؟',
    desc: 'با پنج انتخاب، تعداد توکن، هزینهٔ تقریبی روزانه و شارژ پیشنهادی را حساب کنید. نرخ‌های زیر صرفاً مرجع API هستند، نه قیمت نهایی NabuGate.',
    model: 'مدل مرجع', work: 'نوع کار', hours: 'ساعت کار روزانه', speed: 'درخواست در ساعت', days: 'روز کاری در ماه',
    light: 'گفت‌وگوی کوتاه', coding: 'کدنویسی و تحلیل', heavy: 'پروژهٔ سنگین و متن طولانی',
    input: 'ورودی', output: 'خروجی', perCall: 'توکن در هر درخواست', perDay: 'مصرف توکن روزانه', perMonth: 'هزینهٔ تخمینی ماه',
    costDay: 'هزینهٔ روزانه', pack: 'شارژ پیشنهادی با ۳۰٪ حاشیه', wallet: 'با این میزان اعتبار',
    workDays: 'روز کاری', workHours: 'ساعت کار', priceNote: 'دلار به‌ازای هر یک‌میلیون توکن ورودی / خروجی:',
    disclaimer: 'اعداد بر اساس متوسط حجم درخواست، تعداد درخواست‌ها و روزهای انتخاب‌شده هستند؛ کش، تاریخچهٔ چت، ابزارها، نرخ ارز و قیمت واقعی NabuGate می‌توانند نتیجه را تغییر دهند. ساعت یا تعداد درخواست تضمین نمی‌شود.',
    legal: 'این شارژ، اشتراک Claude.ai نیست. مصرف واقعی Claude Code باید با کلید شخصی یا توافق مجاز انجام شود.',
    signup: 'ساخت حساب و شارژ', guide: 'راهنمای اتصال', day: 'روز', hour: 'ساعت', req: 'درخواست', tokens: 'توکن',
  },
  en: {
    kicker: 'Greater Iran · usage calculator',
    title: 'How many hours a day will you use AI?',
    desc: 'Five inputs estimate daily tokens, daily cost, and a suggested top-up. Public API reference rates are not a NabuGate price quote.',
    model: 'Reference model', work: 'Workload', hours: 'Hours per day', speed: 'Requests per hour', days: 'Workdays per month',
    light: 'Short chat', coding: 'Coding and analysis', heavy: 'Large projects / long context',
    input: 'Input', output: 'Output', perCall: 'Tokens per request', perDay: 'Daily token usage', perMonth: 'Estimated monthly cost',
    costDay: 'Daily cost', pack: 'Suggested top-up (+30% buffer)', wallet: 'With this credit',
    workDays: 'workdays', workHours: 'hours of work', priceNote: 'USD per million input / output tokens:',
    disclaimer: 'Based on average message size, request frequency and workdays. Caching, history, tools, exchange rates and actual NabuGate prices change the outcome; hours and request counts are not guaranteed.',
    legal: 'Not a Claude.ai subscription. Claude Code usage requires personal credentials or a permitted arrangement.',
    signup: 'Create account and top up', guide: 'Connection guide', day: 'days', hour: 'hours', req: 'requests', tokens: 'tokens',
  },
  ar: {
    kicker: 'إيران الكبرى · حاسبة الاستهلاك',
    title: 'كم ساعة تعمل بالذكاء الاصطناعي يومياً؟',
    desc: 'خمسة اختيارات لتقدير الرموز والكلفة والرصيد المقترح. الأسعار مرجعية وليست عرض سعر نهائياً من NabuGate.',
    model: 'النموذج', work: 'نوع العمل', hours: 'ساعات يومية', speed: 'طلبات في الساعة', days: 'أيام العمل شهرياً',
    light: 'دردشة قصيرة', coding: 'برمجة وتحليل', heavy: 'مشاريع وسياق طويل',
    input: 'الإدخال', output: 'الإخراج', perCall: 'رموز لكل طلب', perDay: 'الرموز اليومية', perMonth: 'التكلفة الشهرية التقديرية',
    costDay: 'التكلفة اليومية', pack: 'الرصيد المقترح مع هامش ٣٠٪', wallet: 'بهذا الرصيد',
    workDays: 'أيام عمل', workHours: 'ساعات عمل', priceNote: 'دولار لكل مليون رمز إدخال / إخراج:',
    disclaimer: 'تقدير يعتمد على متوسط الطلب وعدده وأيام العمل. التخزين المؤقت والأدوات وأسعار الصرف والتعرفة الفعلية قد تغير النتيجة ولا نضمن الساعات أو الطلبات.',
    legal: 'هذا ليس اشتراك Claude.ai. استخدام Claude Code يتطلب بيانات اعتماد شخصية أو اتفاقية مناسبة.',
    signup: 'إنشاء حساب وشحن', guide: 'دليل الاتصال', day: 'أيام', hour: 'ساعات', req: 'طلبات', tokens: 'رموز',
  },
};
const dollar = (value) => '$' + fmtNum(value, { minimumFractionDigits: 2, maximumFractionDigits: 2 });

export default function ClaudeAdvisor({ signedIn = false }) {
  const t = useT(T);
  const [model, setModel] = useState('sonnet');
  const [work, setWork] = useState('coding');
  const [hours, setHours] = useState(2);
  const [speed, setSpeed] = useState(8);
  const [days, setDays] = useState(22);
  const [credit, setCredit] = useState(25);
  const rate = RATES.find((m) => m.id === model) || RATES[1];
  const workload = WORKLOADS.find((w) => w.id === work) || WORKLOADS[1];
  const dailyCalls = hours * speed;
  const dailyInput = dailyCalls * workload.input;
  const dailyOutput = dailyCalls * workload.output;
  const dailyCost = (dailyInput * rate.input + dailyOutput * rate.output) / 1_000_000;
  const monthlyCost = dailyCost * days;
  const recommendedCredit = Math.max(1, Math.ceil(monthlyCost * 1.3));
  const pack = CREDIT_OPTIONS.find((amount) => amount >= recommendedCredit);
  const actualCredit = pack === undefined ? recommendedCredit : pack;
  const availableDays = dailyCost > 0 ? credit / dailyCost : 0;

  return (
    <section id="claude-advisor" className="lp-advisor-section" aria-labelledby="lp-advisor-heading">
      <div className="lp-container">
        <div className="lp-advisor-heading">
          <span className="lp-eyebrow">{t('kicker')}</span>
          <h2 id="lp-advisor-heading">{t('title')}</h2>
          <p>{t('desc')}</p>
        </div>
        <div className="lp-advisor-layout">
          <div className="lp-advisor-controls">
            <label className="lp-advisor-field">
              <span>{t('model')}</span>
              <select value={model} onChange={(e) => setModel(e.target.value)}>
                {RATES.map((r) => <option value={r.id} key={r.id}>{r.label}</option>)}
              </select>
              <small>{t('priceNote')} <b dir="ltr">{'$' + rate.input + ' / $' + rate.output}</b></small>
            </label>
            <label className="lp-advisor-field">
              <span>{t('work')}</span>
              <select value={work} onChange={(e) => setWork(e.target.value)}>
                {WORKLOADS.map((w) => <option value={w.id} key={w.id}>{t(w.id)}</option>)}
              </select>
              <small>{t('perCall')}: <b>{fmtInt(workload.input)} {t('input')} + {fmtInt(workload.output)} {t('output')}</b></small>
            </label>
            <div className="lp-advisor-sliders">
              <label className="lp-advisor-field">
                <span>{t('hours')} <b>{fmtInt(hours)} {t('hour')}</b></span>
                <input type="range" min="1" max="12" step="1" value={hours} onChange={(e) => setHours(Number(e.target.value))} />
              </label>
              <label className="lp-advisor-field">
                <span>{t('speed')} <b>{fmtInt(speed)} {t('req')}</b></span>
                <input type="range" min="2" max="30" step="2" value={speed} onChange={(e) => setSpeed(Number(e.target.value))} />
              </label>
              <label className="lp-advisor-field">
                <span>{t('days')} <b>{fmtInt(days)} {t('day')}</b></span>
                <input type="range" min="5" max="30" step="1" value={days} onChange={(e) => setDays(Number(e.target.value))} />
              </label>
            </div>
          </div>
          <div className="lp-advisor-result" aria-live="polite">
            <span className="lp-advisor-result-caption">{t('perMonth')}</span>
            <strong className="lp-advisor-price" dir="ltr">{dollar(monthlyCost)}</strong>
            <dl className="lp-advisor-metrics">
              <div><dt>{t('costDay')}</dt><dd dir="ltr">{dollar(dailyCost)}</dd></div>
              <div><dt>{t('perDay')}</dt><dd>{fmtInt(dailyInput + dailyOutput)} {t('tokens')}</dd></div>
              <div><dt>{t('pack')}</dt><dd dir="ltr">{dollar(actualCredit)}</dd></div>
            </dl>
            <div className="lp-advisor-wallet">
              <label htmlFor="lp-advisor-credit">{t('wallet')}</label>
              <select id="lp-advisor-credit" value={credit} onChange={(e) => setCredit(Number(e.target.value))}>
                {CREDIT_OPTIONS.map((amount) => <option key={amount} value={amount}>{'$' + amount}</option>)}
              </select>
              <strong>{fmtNum(availableDays, { maximumFractionDigits: 1 })} {t('workDays')}</strong>
              <span>≈ {fmtNum(availableDays * hours, { maximumFractionDigits: 0 })} {t('workHours')}</span>
            </div>
            <div className="lp-advisor-actions">
              <a href={signedIn ? '/panel/plans' : '/panel/login'} className="lp-btn lp-btn-primary">{t('signup')}</a>
              <a href="/docs#claude-desktop" className="lp-btn lp-btn-outline">{t('guide')}</a>
            </div>
          </div>
        </div>
        <p className="lp-advisor-disclaimer">{t('disclaimer')} <strong>{t('legal')}</strong></p>
      </div>
    </section>
  );
}
