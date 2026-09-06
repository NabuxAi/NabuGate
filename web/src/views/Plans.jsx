import { navigate } from '../nav.js';
import { useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { usd, faInt } from '../data/mock.js';
import { usePayment } from '../components/usePayment.js';

/*
 * Buying credit.
 *
 * The wallet is in USD because that is the unit the gateway prices models in.
 * The bank page itself charges in toman at the rate the payment bridge is
 * configured with, so the toman figure is only ever shown by the bank.
 */
const PLANS = [
  { id: 'starter', name: 'شروع', amount: 5, blurb: 'برای تست مدل‌ها و پروژه‌های کوچک.', perks: ['حدود ۲ تا ۱۰ میلیون توکن مدل‌های سبک', 'دسترسی به همهٔ مدل‌های مجازِ کلید', 'بدون تاریخ انقضا'] },
  { id: 'pro', name: 'حرفه‌ای', amount: 25, popular: true, blurb: 'برای توسعهٔ روزمره و اتصال به نرم‌افزار.', perks: ['مناسب Cursor، Cline و Claude Code', 'چند کلید با سهمیهٔ جداگانه', 'هشدار موجودی کم روی هر پاسخ (هدر X-Nabu-Balance-Warning)'] },
  { id: 'team', name: 'تیمی', amount: 100, blurb: 'برای تیم و اتوماسیون پیوسته.', perks: ['یک کلید به‌ازای هر برنامه', 'محدودسازی مبدأ و پروایدر برای هر کلید', 'گزارش مصرف به تفکیک کلید و مدل'] },
];

const QUICK = [5, 10, 25, 50, 100];

export default function Plans() {
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
        `اشتراک فعال شد تا ${new Date(r.subscription.expires_at).toLocaleDateString('fa-IR')}.`,
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
      setSubNotice('اشتراک همین حالا پایان یافت. کلیدهای خودت بدون تغییر کار می‌کنند.');
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
    <Layout title="خرید و شارژ حساب" subtitle="پرداخت به‌ازای مصرف؛ موجودی منقضی نمی‌شود">
      {payment.error && <div className="card banner-error"><span>⚠️</span>{payment.error}</div>}
      {payment.settled?.credited && <div className="card banner-ok"><span>✓</span>پرداخت تأیید شد و موجودی اضافه شد.</div>}

      {status && !enabled && (
        <div className="callout warn">
          <span className="ci">⏸</span>
          <div>
            <strong>درگاه پرداخت در این استقرار فعال نیست.</strong> دکمه‌های پرداخت کار نمی‌کنند تا مدیر متغیرهای <code dir="ltr">NABUPAY_URL</code> و <code dir="ltr">NABUPAY_SECRET</code> را تنظیم کند. برای شارژ دستی با پشتیبانی تماس بگیرید.
          </div>
        </div>
      )}

      <div className="plan-grid stagger" style={{ paddingTop: 14 }}>
        {PLANS.map((plan) => (
          <div key={plan.id} className={'plan' + (plan.popular ? ' featured' : '')}>
            {plan.popular && <div className="ribbon">پیشنهاد ما</div>}
            <h3>{plan.name}</h3>
            <div className="price" dir="ltr">{usd(plan.amount)}<small>اعتبار</small></div>
            <p className="muted" style={{ fontSize: 13, lineHeight: 1.7, margin: 0 }}>{plan.blurb}</p>
            <ul>{plan.perks.map((p) => <li key={p}>{p}</li>)}</ul>
            <div className="spacer" />
            <button
              className={'btn btn-lg ' + (plan.popular ? 'btn-primary' : 'btn-outline')}
              onClick={() => buy(plan.id, plan.amount)}
              disabled={payment.busy || !enabled}
            >
              {payment.busy && selected === plan.id ? 'در حال انتقال به درگاه…' : `پرداخت ${usd(plan.amount)}`}
            </button>
          </div>
        ))}
      </div>

      {subs?.plans?.length > 0 && (
        <div className="card">
          <div className="card-head">
            <h3>اشتراک کلیدهای ما</h3>
            <span className="badge badge-muted">جدا از شارژ اعتبار</span>
          </div>
          <p className="muted" style={{ fontSize: 13, lineHeight: 1.9, margin: '0 0 12px' }}>
            اشتراک، اجازهٔ استفاده از کلیدهای خودِ سرور ما را می‌خرد؛ خودِ مصرف جداگانه از همین اعتبار
            کم می‌شود. اگر با کلید خودت کار می‌کنی، نه اشتراک لازم داری و نه چیزی از اعتبارت کم می‌شود —
            صورت‌حسابت را همان سرویس‌دهنده می‌فرستد.
          </p>

          {subError && <div className="banner-error" style={{ marginBottom: 10 }}>{subError}</div>}
          {subNotice && <div className="banner-ok" style={{ marginBottom: 10 }}>{subNotice}</div>}

          {subs.subscription && (
            <div className={subs.active ? 'callout ok' : 'callout warn'} style={{ marginBottom: 12 }}>
              <span className="ci">{subs.active ? '✓' : '⏸'}</span>
              <div className="sub-live">
                <div>
                  <strong>{subs.subscription.name || subs.subscription.plan_id}</strong>{' '}
                  {subs.active ? 'فعال تا' : 'منقضی شده در'}{' '}
                  <span className="ltr">
                    {new Date(subs.subscription.expires_at).toLocaleDateString('fa-IR')}
                  </span>
                </div>
                {subs.active && (
                  <button className="btn btn-sm btn-ghost" disabled={subBusy === 'cancel'} onClick={endPlan}>
                    پایان اشتراک
                  </button>
                )}
              </div>
            </div>
          )}

          <div className="plan-grid sub-plans stagger">
            {subs.plans.map((pl) => (
              <div key={pl.id} className={'plan sub-card' + (pl.popular ? ' featured' : '')}>
                {pl.popular && <div className="ribbon">پیشنهاد ما</div>}
                <h3>{pl.name}</h3>
                <div className="price" dir="ltr">{usd(pl.price_usd)}<small>/ {faInt(pl.days)} روز</small></div>
                {pl.description && (
                  <p className="muted" style={{ fontSize: 13, lineHeight: 1.7, margin: 0 }}>{pl.description}</p>
                )}
                <ul className="sub-terms">
                  <li>
                    <b>{pl.unlocks_all ? 'همهٔ سرویس‌ها' : `${faInt(pl.unlocks)} سرویس`}</b>
                    <span>با کلید ما</span>
                  </li>
                  <li>
                    <b dir="ltr">×{pl.markup}</b>
                    <span>نرخ مصرف روی کلید ما</span>
                  </li>
                  {pl.includes_credit_usd > 0 && (
                    <li><b dir="ltr">{usd(pl.includes_credit_usd)}</b><span>اعتبار هدیه</span></li>
                  )}
                </ul>
                <div className="spacer" />
                <button
                  className={'btn btn-lg ' + (pl.popular ? 'btn-primary' : 'btn-outline')}
                  disabled={subBusy === pl.id}
                  onClick={() => buyPlan(pl.id)}
                >
                  {subBusy === pl.id
                    ? 'در حال فعال‌سازی…'
                    : subs.active && subs.subscription?.plan_id === pl.id
                      ? `تمدید ${faInt(pl.days)} روز`
                      : `فعال‌سازی — ${usd(pl.price_usd)}`}
                </button>
                <p className="muted" style={{ fontSize: 11.5, lineHeight: 1.9, margin: 0 }}>
                  از موجودی حسابت کم می‌شود، نه از کارت. اگر موجودی کافی نداری، اول از همین صفحه شارژ کن.
                </p>
              </div>
            ))}
          </div>
        </div>
      )}

      <div className="card" style={{ marginTop: 6 }}>
        <div className="card-head"><h3>مبلغ دلخواه</h3><span className="badge badge-muted">حداقل ۱ و حداکثر ۵٬۰۰۰ دلار</span></div>
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
          <span className="muted" style={{ fontSize: 13 }}>دلار</span>
          <button className="btn btn-primary" disabled={payment.busy || !enabled || !customOk}>
            {payment.busy && selected === 'custom' ? 'در حال انتقال به درگاه…' : 'پرداخت'}
          </button>
        </form>
      </div>

      <div className="card">
        <div className="card-head"><h3>پرداخت چطور انجام می‌شود؟</h3></div>
        <div className="steps stagger">
          <div className="step"><span className="num">۱</span><div><strong>انتخاب مبلغ</strong><span>یکی از بسته‌ها یا مبلغ دلخواه. فاکتور به نام حساب شما ثبت می‌شود.</span></div></div>
          <div className="step"><span className="num">۲</span><div><strong>انتقال به درگاه بانکی</strong><span>مبلغ به تومان و با نرخ روز نمایش داده می‌شود. کارت را همان‌جا وارد می‌کنید؛ این پنل هیچ اطلاعات کارتی نمی‌بیند.</span></div></div>
          <div className="step"><span className="num">۳</span><div><strong>بازگشت و تأیید</strong><span>به «حساب و مصرف» برمی‌گردید. پنل از خودِ درگاه می‌پرسد پول رسیده یا نه و بعد موجودی را اضافه می‌کند.</span></div></div>
          <div className="step"><span className="num">۴</span><div><strong>مصرف</strong><span>هر درخواست به قیمت واقعی مدل از موجودی کم می‌شود. هدر <code dir="ltr">X-Nabu-Balance-USD</code> روی هر پاسخ باقی‌مانده را می‌گوید.</span></div></div>
        </div>
      </div>

      <div className="card">
        <div className="card-head"><h3>مشکل در پرداخت؟</h3><button className="btn btn-ghost" onClick={() => navigate('docs')}>راهنمای کامل</button></div>
        <details className="faq"><summary>پول از حسابم کم شد ولی موجودی اضافه نشد</summary>
          <div className="faq-body">صفحهٔ «حساب و مصرف» یا «پرداخت‌ها» را باز کنید و «بررسی وضعیت» را بزنید. هر بار که این صفحه‌ها باز می‌شوند، وضعیت فاکتورهای در انتظار از خودِ درگاه پرسیده می‌شود و به‌محض تأیید، موجودی یک‌بار اضافه می‌شود. اگر تا ۷۲ ساعت تأیید نشد، بانک مبلغ را خودکار برمی‌گرداند.</div>
        </details>
        <details className="faq"><summary>بعد از پرداخت به صفحهٔ خطا برگشتم</summary>
          <div className="faq-body">مهم نیست آدرس بازگشت چه می‌گوید؛ پنل به چیزی که در آدرس است اعتماد نمی‌کند و مستقیم از درگاه می‌پرسد. وارد پنل شوید و «پرداخت‌ها» را باز کنید. اگر وضعیت «در انتظار» ماند، شناسهٔ فاکتور را برای پشتیبانی بفرستید.</div>
        </details>
        <details className="faq"><summary>خطای «درگاه پرداخت آدرسی برای ادامه نداد»</summary>
          <div className="faq-body">پل پرداخت نتوانست فاکتور بسازد؛ معمولاً درگاه بانکی موقتاً در دسترس نیست. چند دقیقه بعد دوباره تلاش کنید. اگر تکرار شد، مدیر باید لاگ دروازه را برای پیام <code>payment bridge refused</code> ببیند.</div>
        </details>
        <details className="faq"><summary>چرا مبلغ به دلار است؟</summary>
          <div className="faq-body">دروازه قیمت مدل‌ها را به دلار حساب می‌کند و موجودی هم با همان واحد نگه‌داری می‌شود تا یک عدد در دو جا دو معنی نداشته باشد. معادل تومانی را درگاه بانکی لحظهٔ پرداخت نشان می‌دهد.</div>
        </details>
        <details className="faq"><summary>کلیدم خطای ۴۰۲ می‌دهد</summary>
          <div className="faq-body">یعنی موجودی صفر شده. با هر مبلغی شارژ کنید، همان لحظه کلید دوباره کار می‌کند؛ نیازی به ساخت کلید جدید نیست. برای این که غافلگیر نشوید، پاسخ‌ها وقتی موجودی زیر ۱ دلار برود هدر <code>X-Nabu-Balance-Warning: low</code> دارند.</div>
        </details>
      </div>

      <p className="muted" style={{ fontSize: 12, lineHeight: 1.8 }}>
        همهٔ مبالغ به دلار (USD) است. موجودی قابل برداشت نقدی نیست و بین حساب‌ها منتقل نمی‌شود. {faInt(0)} ریال کارمزد اضافه از طرف NabuGate؛ کارمزد درگاه در صورت وجود، روی صفحهٔ بانک نمایش داده می‌شود.
      </p>
    </Layout>
  );
}
