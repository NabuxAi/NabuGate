import { useEffect, useState } from 'react';
import * as api from '../api.js';
import { fmtInt, fmtNum, useT } from '../i18n/index.jsx';
import '../styles/claude-advisor.css';

// Workload sizes are editorial assumptions, NOT the user's measured requests.
// Model rates and subscription fees are fetched from the deployed backend.
const WORKLOADS = [
  { id: 'light', input: 1500, output: 300 },
  { id: 'coding', input: 8500, output: 1800 },
  { id: 'heavy', input: 26000, output: 4000 },
];
const TOPUPS = [5, 25, 100];

const T = {
  fa: {
    kicker: 'ایران بزرگ · پیش‌بینی شفاف مصرف',
    title: 'هزینهٔ واقعی را قبل از شارژ بهتر بشناسید',
    desc: 'قیمت مدل‌ها و پلن‌ها از تنظیمات همین سرور گرفته می‌شود؛ بعد از انتخاب نوع کار، مصرف عادی و سناریوی سنگین را جدا ببینید. این تخمین، تضمین تعداد ساعت یا قیمت قطعی نیست.',
    load: 'در حال دریافت قیمت‌های نصب فعلی…',
    loadFail: 'تعرفهٔ قابل تأیید از سرور دریافت نشد؛ برای جلوگیری از عددسازی، تخمین مالی نمایش داده نمی‌شود.',
    model: 'مدل واقعی پشت ابزار', work: 'نوع کار', hours: 'ساعت کار روزانه', speed: 'درخواست در ساعت', days: 'روز کاری در ماه',
    light: 'گفت‌وگوی کوتاه', coding: 'کدنویسی و تحلیل', heavy: 'پروژهٔ سنگین و کانتکست طولانی',
    input: 'ورودی', output: 'خروجی', perCall: 'فرضِ هر درخواست',
    noRates: 'تعرفهٔ این مدل در تنظیمات سرور ناقص است؛ برای جلوگیری از محاسبهٔ اشتباه نمی‌توان هزینه‌اش را اعلام کرد.',
    payMode: 'روش پرداخت', gateway: 'با کلید NabuGate', byok: 'با کلید شخصی ارائه‌دهنده (BYOK)',
    choosePlan: 'اشتراک دسترسی به کلیدهای سرور', withoutPlan: 'بدون اشتراک (فقط مسیرهای مجاز)',
    perTerm: 'برای هر دوره', modelCost: 'هزینهٔ پایهٔ مدل', gatewayService: 'افزایش قیمت بابت خدمات Gateway', access: 'هزینهٔ اشتراک دسترسی',
    included: 'اعتبار همراه اشتراک', separateProxy: 'کارمزد پراکسیِ جداگانه',
    typical: 'سناریوی عادی', stress: 'سناریوی سنگین (نه سقف تضمینی)',
    total: 'هزینهٔ تخمینی دوره', budget: 'اعتبار پیشنهادی با سناریوی سنگین',
    perDay: 'توکن روزانه', byokNote: 'در حالت BYOK، صورتحساب مدل را خود ارائه‌دهنده صادر می‌کند؛ هزینهٔ واقعی نزد آن ارائه‌دهنده از این کاتالوگ NabuGate قابل تضمین نیست. در حال حاضر NabuGate برای این حالت کارمزد جداگانه نمی‌گیرد.',
    assumptions: 'سناریوی سنگین: ۲٫۵ برابر ورودی، ۱٫۳ برابر خروجی و ۱۵٪ درخواست اضافی نسبت به فرض عادی. این یک آزمون فشار است، نه صدک ۹۰ آماری؛ پس از ثبت مصرف واقعی می‌توان P50 و P90 را از تاریخچه محاسبه کرد.',
    caveat: 'حافظهٔ مکالمه، کشِ خواندن/نوشتن، مصرف ابزار، تغییر مدل در fallback، مدل‌های با نرخ پلکانی، تغییر تعرفهٔ تأمین‌کننده و تبدیل ارز در مرحلهٔ پرداخت می‌توانند مبلغ واقعی را تغییر دهند. اگر یکی از مسیرها قیمت نداشته باشد، عمداً نتیجه مخفی می‌شود.',
    planNote: 'اشتراک ممکن است برای بعضی ارائه‌دهنده‌ها لازم باشد. هزینهٔ آن جدا از مصرف توکن است و اعتبار هدیه از هزینهٔ مصرف کسر می‌شود؛ ماندهٔ قبلی کاربر در این تخمین فرض نشده است.',
    pricingNote: 'قیمت مرجع در همین استقرار؛ دلار به‌ازای هر میلیون توکن ورودی / خروجی',
    signup: 'ساخت کلید و مدیریت اعتبار', guide: 'راهنمای اتصال ابزارها',
    day: 'روز', hour: 'ساعت', req: 'درخواست', tokens: 'توکن', unknown: 'نامشخص',
    info: 'برای جلوگیری از غافلگیری، مبلغ پیشنهادی با سناریوی سنگین محاسبه شده است. برداشت از کیف پول همچنان بر اساس مصرف واقعی و قیمت مدلِ پاسخ‌داده‌شده انجام می‌شود.',
  },
  en: {
    kicker: 'Greater Iran · transparent usage planner',
    title: 'Understand the cost before topping up',
    desc: 'Model prices and subscription terms come from this deployment. Compare your expected use with a stress scenario. No guaranteed hours or fixed model price.',
    load: 'Loading live deployment pricing…',
    loadFail: 'Unable to load verified deployment rates. We will not invent a price estimate.',
    model: 'Actual model behind the tool', work: 'Workload', hours: 'Hours per workday', speed: 'Requests per hour', days: 'Workdays per month',
    light: 'Short chat', coding: 'Coding and analysis', heavy: 'Large projects / long context',
    input: 'Input', output: 'Output', perCall: 'Per request assumption',
    noRates: 'At least one fallback route lacks configured pricing. We cannot give a trustworthy estimate for this model.',
    payMode: 'Payment method', gateway: 'NabuGate-managed provider key', byok: 'Bring your own provider key (BYOK)',
    choosePlan: 'Gateway key access plan', withoutPlan: 'No subscription (eligible routes only)',
    perTerm: 'per term', modelCost: 'Base model cost', gatewayService: 'Gateway service markup', access: 'Access subscription fee',
    included: 'Included plan credit', separateProxy: 'Additional proxy fee',
    typical: 'Typical scenario', stress: 'High-usage stress scenario (not a cap)',
    total: 'Estimated term cost', budget: 'Suggested top-up (stress scenario)',
    perDay: 'Daily tokens', byokNote: 'With BYOK, the upstream bills you directly. Its actual rate cannot be guaranteed by the NabuGate rate catalog. NabuGate currently adds no separate proxy fee for this mode.',
    assumptions: 'Stress case: 2.5× input, 1.3× output, and 15% more requests than your usual estimate. This is NOT an empirical P90; once actual usage is collected, P50/P90 can be calculated.',
    caveat: 'Context history, prompt caching, tools, fallback routing, tiered prices, vendor rate changes and checkout exchange rates may change the actual bill. Incomplete route pricing deliberately hides numeric quotes.',
    planNote: 'Some managed providers require an access subscription. Its fee is separate from metered tokens; included credit offsets usage. Existing wallet balances are not assumed.',
    pricingNote: 'Deployment reference price per million input / output tokens in USD',
    signup: 'Manage keys and wallet', guide: 'Tool connection guide',
    day: 'days', hour: 'hours', req: 'requests', tokens: 'tokens', unknown: 'Unknown',
    info: 'The suggested top-up uses the stress scenario to reduce surprises. Actual wallet deductions still depend on the model and usage of each completed request.',
  },
  ar: {
    kicker: 'إيران الكبرى · توقع الاستهلاك بشفافية',
    title: 'افهم التكلفة قبل شحن الرصيد',
    desc: 'تأتي أسعار النماذج والخطط من إعدادات البوابة الفعلية. قارن بين الاستخدام العادي والمرتفع دون ضمان ساعات ثابتة.',
    load: 'تحميل تعرفة البوابة…',
    loadFail: 'تعذر تحميل الأسعار المتحققة؛ لن نعرض أرقاماً غير موثوقة.',
    model: 'النموذج الفعلي', work: 'نوع العمل', hours: 'ساعات العمل يومياً', speed: 'طلبات في الساعة', days: 'أيام العمل في الشهر',
    light: 'محادثة قصيرة', coding: 'برمجة وتحليل', heavy: 'مشاريع كبيرة وسياق طويل',
    input: 'إدخال', output: 'إخراج', perCall: 'الافتراض لكل طلب',
    noRates: 'بعض المسارات الاحتياطية دون أسعار معلنة؛ لا يمكن إصدار تقدير موثوق.',
    payMode: 'طريقة الدفع', gateway: 'مفتاح NabuGate', byok: 'المفتاح الشخصي (BYOK)',
    choosePlan: 'خطة الوصول', withoutPlan: 'دون اشتراك (المسارات المسموحة)',
    perTerm: 'لكل فترة', modelCost: 'تكلفة النموذج الأساسية', gatewayService: 'هامش خدمة البوابة', access: 'رسوم الاشتراك',
    included: 'رصيد ضمن الخطة', separateProxy: 'رسوم بروكسي إضافية',
    typical: 'الاستخدام الاعتيادي', stress: 'سيناريو مرتفع (غير مضمون)',
    total: 'التكلفة المقدرة للفترة', budget: 'الرصيد المقترح للسيناريو المرتفع',
    perDay: 'رموز يومية', byokNote: 'عند استعمال مفتاحك الشخصي يرسل المزود فاتورته مباشرة، ولا يضمن NabuGate تعرفته. رسوم البروكسي الإضافية حالياً صفر.',
    assumptions: 'السيناريو المرتفع: ٢٫٥× إدخال و١٫٣× إخراج و١٥٪ طلبات إضافية. هذه ليست قيمة P90 إحصائية.',
    caveat: 'قد تتغير التكلفة بسبب تاريخ المحادثة والتخزين المؤقت والأدوات والتوجيه والأسعار المتدرجة والصرف. لا نعرض سعراً عند نقص البيانات.',
    planNote: 'الاشتراك منفصل عن الاستهلاك وقد يكون ضرورياً لبعض المزودين. يحسم الرصيد المرفق من رسوم الاستخدام.',
    pricingNote: 'تعرفة البوابة بالدولار لكل مليون رمز إدخال وإخراج',
    signup: 'إدارة المفاتيح والرصيد', guide: 'دليل اتصال الأدوات',
    day: 'أيام', hour: 'ساعات', req: 'طلبات', tokens: 'رموز', unknown: 'غير معروف',
    info: 'الشحن المقترح مبني على السيناريو المرتفع. تُحسب الفاتورة الفعلية حسب الاستخدام والنموذج الذي أجاب الطلب.',
  },
};

function dollars(value) {
  if (value == null || !Number.isFinite(value)) return '—';
  return '$' + fmtNum(value, { minimumFractionDigits: 2, maximumFractionDigits: 4 });
}

function quote(rate, calls, inTokens, outTokens, markup, minCharge) {
  const model = calls * (rate.input * inTokens + rate.output * outTokens) / 1e6;
  const billed = calls * Math.max(((rate.input * inTokens + rate.output * outTokens) / 1e6) * markup, minCharge);
  return { model, gateway: Math.max(0, billed - model), billed };
}

function workloadResult(model, calls, inTokens, outTokens, markup, minCharge, stress = false) {
  if (!model || !model.complete || !Array.isArray(model.rates) || !model.rates.length) return null;
  const count = calls * (stress ? 1.15 : 1);
  const input = inTokens * (stress ? 2.5 : 1);
  const output = outTokens * (stress ? 1.3 : 1);
  const quotes = model.rates.map((rate) => quote(rate, count, input, output, markup, minCharge));
  return stress
    ? quotes.reduce((a, b) => b.billed > a.billed ? b : a)
    : quotes[0]; // The configured first, preferred routing choice.
}

function topUpSuggestion(cost) {
  if (!Number.isFinite(cost)) return null;
  const ceil = Math.ceil(Math.max(1, cost) * 100) / 100;
  return TOPUPS.find((n) => n >= ceil) || Math.ceil(ceil);
}

export default function ClaudeAdvisor({ signedIn = false }) {
  const t = useT(T);
  const [catalog, setCatalog] = useState(null);
  const [loadError, setLoadError] = useState(false);
  const [modelID, setModelID] = useState('nabu-smart');
  const [work, setWork] = useState('coding');
  const [hours, setHours] = useState(2);
  const [requestsPerHour, setRequestsPerHour] = useState(8);
  const [days, setDays] = useState(22);
  const [paymentMode, setPaymentMode] = useState('gateway');
  const [planID, setPlanID] = useState('');

  useEffect(() => {
    let active = true;
    api.publicModelCosts().then((data) => {
      if (!active) return;
      if (!Array.isArray(data?.models) || !Array.isArray(data?.plans)) {
        setLoadError(true);
        return;
      }
      setCatalog(data);
      const preferred = data.models.find((m) => m.alias === 'nabu-smart' && m.complete)
        || data.models.find((m) => m.complete);
      if (preferred) setModelID(preferred.alias);
      if (data.plans.length) setPlanID(data.plans[0].id);
    }).catch(() => { if (active) setLoadError(true); });
    return () => { active = false; };
  }, []);

  const models = catalog?.models || [];
  const plans = catalog?.plans || [];
  const model = models.find((m) => m.alias === modelID) || null;
  const selectedPlan = plans.find((p) => p.id === planID) || null;
  const workload = WORKLOADS.find((w) => w.id === work) || WORKLOADS[1];
  const managed = paymentMode === 'gateway';
  const markup = managed ? (selectedPlan?.markup || 1) : 1;
  const floor = managed ? (selectedPlan?.min_charge_usd || 0) : 0;
  const planFee = managed ? (selectedPlan?.price_usd || 0) : 0;
  const includedCredit = managed ? (selectedPlan?.includes_credit_usd || 0) : 0;
  const calls = hours * requestsPerHour * days;
  const typical = managed ? workloadResult(model, calls, workload.input, workload.output, markup, floor) : null;
  const high = managed ? workloadResult(model, calls, workload.input, workload.output, markup, floor, true) : null;
  const cashEstimate = typical ? planFee + Math.max(0, typical.billed - includedCredit) : null;
  const highCash = high ? planFee + Math.max(0, high.billed - includedCredit) : null;
  const recommended = highCash == null ? null : topUpSuggestion(highCash);
  const displayedRate = model?.complete && model.rates?.length ? model.rates[0] : null;

  return (
    <section id="claude-advisor" className="lp-advisor-section" aria-labelledby="lp-advisor-heading">
      <div className="lp-container">
        <div className="lp-advisor-heading">
          <span className="lp-eyebrow">{t('kicker')}</span>
          <h2 id="lp-advisor-heading">{t('title')}</h2>
          <p>{t('desc')}</p>
        </div>
        {!catalog && (
          <p className="lp-advisor-load" role={loadError ? 'alert' : 'status'}>{t(loadError ? 'loadFail' : 'load')}</p>
        )}
        {catalog && (
          <>
            <div className="lp-advisor-layout">
              <div className="lp-advisor-controls">
                <label className="lp-advisor-field">
                  <span>{t('model')}</span>
                  <select value={modelID} onChange={(e) => setModelID(e.target.value)}>
                    {models.map((m) => <option value={m.alias} key={m.alias}>{m.alias}{m.complete ? '' : ' · ?'}</option>)}
                  </select>
                  <small>{displayedRate ? t('pricingNote') + ': $' + displayedRate.input + ' / $' + displayedRate.output : t('noRates')}</small>
                </label>
                <label className="lp-advisor-field">
                  <span>{t('work')}</span>
                  <select value={work} onChange={(e) => setWork(e.target.value)}>
                    {WORKLOADS.map((w) => <option key={w.id} value={w.id}>{t(w.id)}</option>)}
                  </select>
                  <small>{t('perCall')}: {fmtInt(workload.input)} {t('input')} + {fmtInt(workload.output)} {t('output')}</small>
                </label>
                <div className="lp-advisor-sliders">
                  <label className="lp-advisor-field">
                    <span>{t('hours')} <b>{fmtInt(hours)} {t('hour')}</b></span>
                    <input type="range" min="1" max="12" value={hours} onChange={(e) => setHours(Number(e.target.value))} />
                  </label>
                  <label className="lp-advisor-field">
                    <span>{t('speed')} <b>{fmtInt(requestsPerHour)} {t('req')}</b></span>
                    <input type="range" min="2" max="30" step="2" value={requestsPerHour} onChange={(e) => setRequestsPerHour(Number(e.target.value))} />
                  </label>
                  <label className="lp-advisor-field">
                    <span>{t('days')} <b>{fmtInt(days)} {t('day')}</b></span>
                    <input type="range" min="5" max="30" value={days} onChange={(e) => setDays(Number(e.target.value))} />
                  </label>
                </div>
                <label className="lp-advisor-field">
                  <span>{t('payMode')}</span>
                  <select value={paymentMode} onChange={(e) => setPaymentMode(e.target.value)}>
                    <option value="gateway">{t('gateway')}</option>
                    <option value="byok">{t('byok')}</option>
                  </select>
                </label>
                {managed && (
                  <label className="lp-advisor-field">
                    <span>{t('choosePlan')}</span>
                    <select value={planID} onChange={(e) => setPlanID(e.target.value)}>
                      <option value="">{t('withoutPlan')}</option>
                      {plans.map((p) => <option key={p.id} value={p.id}>{p.name} · {dollars(p.price_usd)} / {fmtInt(p.days)} {t('day')}</option>)}
                    </select>
                  </label>
                )}
              </div>
              <div className="lp-advisor-result" aria-live="polite">
                <span className="lp-advisor-result-caption">{t('typical')}</span>
                <strong className="lp-advisor-price" dir="ltr">{dollars(cashEstimate)}</strong>
                {!managed ? (
                  <p className="lp-advisor-wallet">{t('byokNote')}</p>
                ) : !typical ? (
                  <p className="lp-advisor-wallet" role="status">{t('noRates')}</p>
                ) : (
                  <>
                    <dl className="lp-advisor-metrics">
                      <div><dt>{t('modelCost')}</dt><dd dir="ltr">{dollars(typical.model)}</dd></div>
                      <div><dt>{t('gatewayService')}</dt><dd dir="ltr">{dollars(typical.gateway)}</dd></div>
                      <div><dt>{t('access')}</dt><dd dir="ltr">{dollars(planFee)}</dd></div>
                      <div><dt>{t('included')}</dt><dd dir="ltr">−{dollars(includedCredit)}</dd></div>
                      <div><dt>{t('separateProxy')}</dt><dd dir="ltr">{dollars(catalog.proxy_fee_usd || 0)}</dd></div>
                    </dl>
                    <div className="lp-advisor-wallet">
                      <span>{t('stress')}</span>
                      <strong dir="ltr">{dollars(highCash)}</strong>
                      <span>{t('budget')}</span>
                      <strong dir="ltr">{dollars(recommended)}</strong>
                    </div>
                  </>
                )}
                <div className="lp-advisor-metrics">
                  <div><span>{t('perDay')}</span><b>{fmtInt(requestsPerHour * hours * (workload.input + workload.output))} {t('tokens')}</b></div>
                </div>
                <div className="lp-advisor-actions">
                  <a href={signedIn ? '/panel/plans' : '/panel/login'} className="lp-btn lp-btn-primary">{t('signup')}</a>
                  <a href="/docs#claude-desktop" className="lp-btn lp-btn-outline">{t('guide')}</a>
                </div>
              </div>
            </div>
            <div className="lp-advisor-disclaimer">
              <p>{t('assumptions')}</p>
              <p>{t('caveat')}</p>
              <p>{t('planNote')}</p>
              <strong>{t('info')}</strong>
            </div>
          </>
        )}
      </div>
    </section>
  );
}
