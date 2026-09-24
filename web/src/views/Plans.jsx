import { navigate } from '../nav.js';
import { useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { usd, fmtInt, fmtDate, useT } from '../i18n/index.jsx';
import { usePayment } from '../components/usePayment.js';
import Icon from '../components/Icon.jsx';

/*
 * Buying credit.
 *
 * The wallet is in USD because that is the unit the gateway prices models in.
 * The bank page itself charges in toman at the rate the payment bridge is
 * configured with, so the toman figure is only ever shown by the bank.
 */
// name, blurb and perks are keys into T below.
const PLANS = [
  { id: 'starter', name: 'starterName', amount: 5, blurb: 'starterBlurb', perks: ['starterP1', 'starterP2', 'starterP3'] },
  { id: 'pro', name: 'proName', amount: 25, popular: true, blurb: 'proBlurb', perks: ['proP1', 'proP2', 'proP3'] },
  { id: 'team', name: 'teamName', amount: 100, blurb: 'teamBlurb', perks: ['teamP1', 'teamP2', 'teamP3'] },
];

const T = {
  fa: {
    starterName: 'شروع',
    starterBlurb: 'برای تست مدل‌ها و پروژه‌های کوچک.',
    starterP1: 'حدود ۲ تا ۱۰ میلیون توکن مدل‌های سبک',
    starterP2: 'دسترسی به همهٔ مدل‌های مجازِ کلید',
    starterP3: 'بدون تاریخ انقضا',
    proName: 'حرفه‌ای',
    proBlurb: 'برای توسعهٔ روزمره و اتصال به نرم‌افزار.',
    proP1: 'مناسب Cursor، Cline و Claude Code',
    proP2: 'چند کلید با سهمیهٔ جداگانه',
    proP3: 'هشدار موجودی کم روی هر پاسخ (هدر X-Nabu-Balance-Warning)',
    teamName: 'تیمی',
    teamBlurb: 'برای تیم و اتوماسیون پیوسته.',
    teamP1: 'یک کلید به‌ازای هر برنامه',
    teamP2: 'محدودسازی مبدأ و پروایدر برای هر کلید',
    teamP3: 'گزارش مصرف به تفکیک کلید و مدل',

    title: 'خرید و شارژ حساب',
    subtitle: 'پرداخت به‌ازای مصرف؛ موجودی منقضی نمی‌شود',
    subActivated: 'اشتراک فعال شد تا {date}.',
    subEnded: 'اشتراک همین حالا پایان یافت. کلیدهای خودت بدون تغییر کار می‌کنند.',
    credited: 'پرداخت تأیید شد و موجودی اضافه شد.',
    disabled: () => <><strong>درگاه پرداخت در این استقرار فعال نیست.</strong> دکمه‌های پرداخت کار نمی‌کنند تا مدیر متغیرهای <code dir="ltr">NABUPAY_URL</code> و <code dir="ltr">NABUPAY_SECRET</code> را تنظیم کند. برای شارژ دستی با پشتیبانی تماس بگیرید.</>,
    recommended: 'پیشنهاد ما',
    credit: 'اعتبار',
    redirecting: 'در حال انتقال به درگاه…',
    payAmount: 'پرداخت {amount}',
    subsTitle: 'اشتراک کلیدهای ما',
    subsBadge: 'جدا از شارژ اعتبار',
    subsIntro: 'اشتراک، اجازهٔ استفاده از کلیدهای خودِ سرور ما را می‌خرد؛ خودِ مصرف جداگانه از همین اعتبار کم می‌شود. اگر با کلید خودت کار می‌کنی، نه اشتراک لازم داری و نه چیزی از اعتبارت کم می‌شود — صورت‌حسابت را همان سرویس‌دهنده می‌فرستد.',
    activeUntil: 'فعال تا',
    expiredOn: 'منقضی شده در',
    endSub: 'پایان اشتراک',
    perDays: '/ {n} روز',
    allServices: 'همهٔ سرویس‌ها',
    nServices: '{n} سرویس',
    withOurKey: 'با کلید ما',
    markup: 'نرخ مصرف روی کلید ما',
    giftCredit: 'اعتبار هدیه',
    activating: 'در حال فعال‌سازی…',
    renew: 'تمدید {n} روز',
    activate: 'فعال‌سازی — {price}',
    fromBalance: 'از موجودی حسابت کم می‌شود، نه از کارت. اگر موجودی کافی نداری، اول از همین صفحه شارژ کن.',
    customTitle: 'مبلغ دلخواه',
    customRange: 'حداقل ۱ و حداکثر ۵٬۰۰۰ دلار',
    dollars: 'دلار',
    pay: 'پرداخت',
    howTitle: 'پرداخت چطور انجام می‌شود؟',
    step1t: 'انتخاب مبلغ',
    step1b: 'یکی از بسته‌ها یا مبلغ دلخواه. فاکتور به نام حساب شما ثبت می‌شود.',
    step2t: 'انتقال به درگاه بانکی',
    step2b: 'مبلغ به تومان و با نرخ روز نمایش داده می‌شود. کارت را همان‌جا وارد می‌کنید؛ این پنل هیچ اطلاعات کارتی نمی‌بیند.',
    step3t: 'بازگشت و تأیید',
    step3b: 'به «حساب و مصرف» برمی‌گردید. پنل از خودِ درگاه می‌پرسد پول رسیده یا نه و بعد موجودی را اضافه می‌کند.',
    step4t: 'مصرف',
    step4b: () => <>هر درخواست به قیمت واقعی مدل از موجودی کم می‌شود. هدر <code dir="ltr">X-Nabu-Balance-USD</code> روی هر پاسخ باقی‌مانده را می‌گوید.</>,
    troubleTitle: 'مشکل در پرداخت؟',
    fullGuide: 'راهنمای کامل',
    faq1q: 'پول از حسابم کم شد ولی موجودی اضافه نشد',
    faq1a: 'صفحهٔ «حساب و مصرف» یا «پرداخت‌ها» را باز کنید و «بررسی وضعیت» را بزنید. هر بار که این صفحه‌ها باز می‌شوند، وضعیت فاکتورهای در انتظار از خودِ درگاه پرسیده می‌شود و به‌محض تأیید، موجودی یک‌بار اضافه می‌شود. اگر تا ۷۲ ساعت تأیید نشد، بانک مبلغ را خودکار برمی‌گرداند.',
    faq2q: 'بعد از پرداخت به صفحهٔ خطا برگشتم',
    faq2a: 'مهم نیست آدرس بازگشت چه می‌گوید؛ پنل به چیزی که در آدرس است اعتماد نمی‌کند و مستقیم از درگاه می‌پرسد. وارد پنل شوید و «پرداخت‌ها» را باز کنید. اگر وضعیت «در انتظار» ماند، شناسهٔ فاکتور را برای پشتیبانی بفرستید.',
    faq3q: 'خطای «درگاه پرداخت آدرسی برای ادامه نداد»',
    faq3a: () => <>پل پرداخت نتوانست فاکتور بسازد؛ معمولاً درگاه بانکی موقتاً در دسترس نیست. چند دقیقه بعد دوباره تلاش کنید. اگر تکرار شد، مدیر باید لاگ دروازه را برای پیام <code>payment bridge refused</code> ببیند.</>,
    faq4q: 'چرا مبلغ به دلار است؟',
    faq4a: 'دروازه قیمت مدل‌ها را به دلار حساب می‌کند و موجودی هم با همان واحد نگه‌داری می‌شود تا یک عدد در دو جا دو معنی نداشته باشد. معادل تومانی را درگاه بانکی لحظهٔ پرداخت نشان می‌دهد.',
    faq5q: 'کلیدم خطای ۴۰۲ می‌دهد',
    faq5a: () => <>یعنی موجودی صفر شده. با هر مبلغی شارژ کنید، همان لحظه کلید دوباره کار می‌کند؛ نیازی به ساخت کلید جدید نیست. برای این که غافلگیر نشوید، پاسخ‌ها وقتی موجودی زیر ۱ دلار برود هدر <code>X-Nabu-Balance-Warning: low</code> دارند.</>,
    footer: 'همهٔ مبالغ به دلار (USD) است. موجودی قابل برداشت نقدی نیست و بین حساب‌ها منتقل نمی‌شود. {zero} ریال کارمزد اضافه از طرف NabuGate؛ کارمزد درگاه در صورت وجود، روی صفحهٔ بانک نمایش داده می‌شود.',
  },
  en: {
    starterName: 'Starter',
    starterBlurb: 'For trying out models and small projects.',
    starterP1: 'Roughly 2–10 million tokens on lightweight models',
    starterP2: 'Access to every model your key allows',
    starterP3: 'Never expires',
    proName: 'Pro',
    proBlurb: 'For everyday development and hooking up your tools.',
    proP1: 'Works with Cursor, Cline and Claude Code',
    proP2: 'Multiple keys with separate quotas',
    proP3: 'Low-balance warning on every response (X-Nabu-Balance-Warning header)',
    teamName: 'Team',
    teamBlurb: 'For teams and always-on automation.',
    teamP1: 'One key per app',
    teamP2: 'Origin and provider restrictions per key',
    teamP3: 'Usage reports by key and model',

    title: 'Plans & top-up',
    subtitle: 'Pay as you go; credit never expires',
    subActivated: 'Subscription active until {date}.',
    subEnded: 'Subscription ended just now. Your own keys keep working unchanged.',
    credited: 'Payment confirmed and your balance has been credited.',
    disabled: () => <><strong>The payment gateway is not enabled on this deployment.</strong> Payment buttons won’t work until an admin sets <code dir="ltr">NABUPAY_URL</code> and <code dir="ltr">NABUPAY_SECRET</code>. Contact support for a manual top-up.</>,
    recommended: 'Recommended',
    credit: 'credit',
    redirecting: 'Redirecting to the gateway…',
    payAmount: 'Pay {amount}',
    subsTitle: 'Subscriptions to our keys',
    subsBadge: 'Separate from credit top-ups',
    subsIntro: 'A subscription buys the right to use our server’s own keys; the usage itself is still deducted from your credit. If you bring your own key, you need no subscription and nothing comes out of your credit — that provider bills you directly.',
    activeUntil: 'active until',
    expiredOn: 'expired on',
    endSub: 'End subscription',
    perDays: '/ {n} days',
    allServices: 'All services',
    nServices: '{n} services',
    withOurKey: 'with our key',
    markup: 'usage rate on our key',
    giftCredit: 'bonus credit',
    activating: 'Activating…',
    renew: 'Renew {n} days',
    activate: 'Activate — {price}',
    fromBalance: 'Paid from your account balance, not your card. If your balance is too low, top up on this page first.',
    customTitle: 'Custom amount',
    customRange: 'Min $1, max $5,000',
    dollars: 'USD',
    pay: 'Pay',
    howTitle: 'How does payment work?',
    step1t: 'Pick an amount',
    step1b: 'One of the packs or a custom amount. The invoice is issued in your account’s name.',
    step2t: 'Go to the bank gateway',
    step2b: 'The amount is shown in toman at the day’s rate. You enter your card there; this console never sees any card details.',
    step3t: 'Return and confirmation',
    step3b: 'You land back on “Account & usage”. The console asks the gateway itself whether the money arrived, then credits your balance.',
    step4t: 'Usage',
    step4b: () => <>Each request deducts the model’s actual price from your balance. The <code dir="ltr">X-Nabu-Balance-USD</code> header on every response shows what is left.</>,
    troubleTitle: 'Trouble paying?',
    fullGuide: 'Full guide',
    faq1q: 'Money left my account but my balance didn’t go up',
    faq1a: 'Open “Account & usage” or “Payments” and press “Check status”. Every time these pages open, pending invoices are checked with the gateway itself, and once confirmed your balance is credited exactly once. If it isn’t confirmed within 72 hours, the bank refunds the amount automatically.',
    faq2q: 'I was sent to an error page after paying',
    faq2a: 'It doesn’t matter what the return URL says; the console doesn’t trust anything in the URL and asks the gateway directly. Sign in and open “Payments”. If the status stays “Pending”, send the invoice ID to support.',
    faq3q: 'Error “The payment gateway did not return a checkout URL.”',
    faq3a: () => <>The payment bridge couldn’t create an invoice; usually the bank gateway is temporarily unavailable. Try again in a few minutes. If it keeps happening, an admin should check the gateway logs for <code>payment bridge refused</code>.</>,
    faq4q: 'Why are amounts in dollars?',
    faq4a: 'The gateway prices models in dollars, and your balance is kept in the same unit so one number never means two things in two places. The bank gateway shows the toman equivalent at the moment of payment.',
    faq5q: 'My key returns a 402 error',
    faq5a: () => <>It means your balance has hit zero. Top up any amount and the key works again instantly; no need to create a new one. So it never catches you off guard, responses carry the <code>X-Nabu-Balance-Warning: low</code> header once your balance drops below $1.</>,
    footer: 'All amounts are in US dollars (USD). Credit cannot be withdrawn as cash or transferred between accounts. NabuGate adds no fees of its own; any gateway fee is shown on the bank’s page.',
  },
};

const QUICK = [5, 10, 25, 50, 100];

export default function Plans() {
  const t = useT(T);
  const [selected, setSelected] = useState(null);
  const [custom, setCustom] = useState(25);
  const [status, setStatus] = useState(null);
  const [subs, setSubs] = useState(null); // { plans, subscription, active, balance }
  const [subBusy, setSubBusy] = useState(null);
  const [subError, setSubError] = useState(null);
  const [subNotice, setSubNotice] = useState(null);
  const payment = usePayment(() => navigate('account'));

  const loadSubs = () => api.listPlans().then(setSubs).catch(() => setSubs({ plans: [] }));

  useEffect(() => {
    api.status().then(setStatus).catch(() => setStatus({}));
    loadSubs();
  }, []);

  const buyPlan = async (id) => {
    setSubBusy(id);
    setSubError(null);
    setSubNotice(null);
    try {
      const r = await api.subscribe(id);
      setSubNotice(
        t('subActivated', { date: fmtDate(r.subscription.expires_at) }),
      );
      await loadSubs();
    } catch (e) {
      setSubError(e.message);
    } finally {
      setSubBusy(null);
    }
  };

  const endPlan = async () => {
    setSubBusy('cancel');
    setSubError(null);
    try {
      await api.cancelSubscription();
      setSubNotice(t('subEnded'));
      await loadSubs();
    } catch (e) {
      setSubError(e.message);
    } finally {
      setSubBusy(null);
    }
  };

  const enabled = status?.payments_enabled !== false;

  const buy = (id, amount) => {
    setSelected(id);
    payment.pay(amount);
  };

  const customOk = Number(custom) >= 1 && Number(custom) <= 5000;

  return (
    <Layout title={t('title')} subtitle={t('subtitle')}>
      {payment.error && <div className="card banner-error"><span>⚠️</span>{payment.error}</div>}
      {payment.settled?.credited && <div className="card banner-ok"><span>✓</span>{t('credited')}</div>}

      {status && !enabled && (
        <div className="callout warn">
          <span className="ci">⏸</span>
          <div>
            {t('disabled')}
          </div>
        </div>
      )}

      <div className="plan-grid stagger" style={{ paddingTop: 14 }}>
        {PLANS.map((plan) => (
          <div key={plan.id} className={'plan' + (plan.popular ? ' featured' : '')}>
            {plan.popular && <div className="ribbon">{t('recommended')}</div>}
            <h3>{t(plan.name)}</h3>
            <div className="price" dir="ltr">{usd(plan.amount)}<small>{t('credit')}</small></div>
            <p className="muted" style={{ fontSize: 13, lineHeight: 1.7, margin: 0 }}>{t(plan.blurb)}</p>
            <ul>{plan.perks.map((p) => <li key={p}>{t(p)}</li>)}</ul>
            <div className="spacer" />
            <button
              className={'btn btn-lg ' + (plan.popular ? 'btn-primary' : 'btn-outline')}
              onClick={() => buy(plan.id, plan.amount)}
              disabled={payment.busy || !enabled}
            >
              {payment.busy && selected === plan.id ? t('redirecting') : t('payAmount', { amount: usd(plan.amount) })}
            </button>
          </div>
        ))}
      </div>

      {subs?.plans?.length > 0 && (
        <div className="card">
          <div className="card-head">
            <h3><Icon name="key" size={17} />{t('subsTitle')}</h3>
            <span className="badge badge-muted">{t('subsBadge')}</span>
          </div>
          <p className="muted" style={{ fontSize: 13, lineHeight: 1.9, margin: '0 0 12px' }}>
            {t('subsIntro')}
          </p>

          {subError && <div className="banner-error" style={{ marginBottom: 10 }}>{subError}</div>}
          {subNotice && <div className="banner-ok" style={{ marginBottom: 10 }}>{subNotice}</div>}

          {subs.subscription && (
            <div className={subs.active ? 'callout ok' : 'callout warn'} style={{ marginBottom: 12 }}>
              <span className="ci">{subs.active ? '✓' : '⏸'}</span>
              <div className="sub-live">
                <div>
                  <strong>{subs.subscription.name || subs.subscription.plan_id}</strong>{' '}
                  {subs.active ? t('activeUntil') : t('expiredOn')}{' '}
                  <span className="ltr">
                    {fmtDate(subs.subscription.expires_at)}
                  </span>
                </div>
                {subs.active && (
                  <button className="btn btn-sm btn-ghost" disabled={subBusy === 'cancel'} onClick={endPlan}>
                    {t('endSub')}
                  </button>
                )}
              </div>
            </div>
          )}

          <div className="plan-grid sub-plans stagger">
            {subs.plans.map((pl) => (
              <div key={pl.id} className={'plan sub-card' + (pl.popular ? ' featured' : '')}>
                {pl.popular && <div className="ribbon">{t('recommended')}</div>}
                <h3>{pl.name}</h3>
                <div className="price" dir="ltr">{usd(pl.price_usd)}<small>{t('perDays', { n: fmtInt(pl.days) })}</small></div>
                {pl.description && (
                  <p className="muted" style={{ fontSize: 13, lineHeight: 1.7, margin: 0 }}>{pl.description}</p>
                )}
                <ul className="sub-terms">
                  <li>
                    <b>{pl.unlocks_all ? t('allServices') : t('nServices', { n: fmtInt(pl.unlocks) })}</b>
                    <span>{t('withOurKey')}</span>
                  </li>
                  <li>
                    <b dir="ltr">×{pl.markup}</b>
                    <span>{t('markup')}</span>
                  </li>
                  {pl.includes_credit_usd > 0 && (
                    <li><b dir="ltr">{usd(pl.includes_credit_usd)}</b><span>{t('giftCredit')}</span></li>
                  )}
                </ul>
                <div className="spacer" />
                <button
                  className={'btn btn-lg ' + (pl.popular ? 'btn-primary' : 'btn-outline')}
                  disabled={subBusy === pl.id}
                  onClick={() => buyPlan(pl.id)}
                >
                  {subBusy === pl.id
                    ? t('activating')
                    : subs.active && subs.subscription?.plan_id === pl.id
                      ? t('renew', { n: fmtInt(pl.days) })
                      : t('activate', { price: usd(pl.price_usd) })}
                </button>
                <p className="muted" style={{ fontSize: 11.5, lineHeight: 1.9, margin: 0 }}>
                  {t('fromBalance')}
                </p>
              </div>
            ))}
          </div>
        </div>
      )}

      <div className="card" style={{ marginTop: 6 }}>
        <div className="card-head"><h3><Icon name="wallet" size={17} />{t('customTitle')}</h3><span className="badge badge-muted">{t('customRange')}</span></div>
        <div className="chips" style={{ marginBottom: 14 }}>
          {QUICK.map((a) => (
            <button key={a} type="button" className={'chip' + (Number(custom) === a ? ' active' : '')} onClick={() => setCustom(a)} dir="ltr">
              {usd(a)}
            </button>
          ))}
        </div>
        <form
          onSubmit={(e) => { e.preventDefault(); if (customOk) buy('custom', Number(custom)); }}
          style={{ display: 'flex', gap: 10, alignItems: 'center', flexWrap: 'wrap' }}
        >
          <input type="number" className="input" value={custom} onChange={(e) => setCustom(e.target.value)} min="1" max="5000" step="1" style={{ width: 140 }} dir="ltr" />
          <span className="muted" style={{ fontSize: 13 }}>{t('dollars')}</span>
          <button className="btn btn-primary" disabled={payment.busy || !enabled || !customOk}>
            {payment.busy && selected === 'custom' ? t('redirecting') : t('pay')}
          </button>
        </form>
      </div>

      <div className="card">
        <div className="card-head"><h3><Icon name="info" size={17} />{t('howTitle')}</h3></div>
        <div className="steps stagger">
          {[1, 2, 3, 4].map((n) => (
            <div key={n} className="step"><span className="num">{fmtInt(n)}</span><div><strong>{t('step' + n + 't')}</strong><span>{t('step' + n + 'b')}</span></div></div>
          ))}
        </div>
      </div>

      <div className="card">
        <div className="card-head"><h3><Icon name="alert" size={17} />{t('troubleTitle')}</h3><button className="btn btn-ghost" onClick={() => navigate('docs')}>{t('fullGuide')}</button></div>
        {[1, 2, 3, 4, 5].map((n) => (
          <details key={n} className="faq"><summary>{t('faq' + n + 'q')}</summary>
            <div className="faq-body">{t('faq' + n + 'a')}</div>
          </details>
        ))}
      </div>

      <p className="muted" style={{ fontSize: 12, lineHeight: 1.8 }}>
        {t('footer', { zero: fmtInt(0) })}
      </p>
    </Layout>
  );
}
