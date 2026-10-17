import { useEffect, useState } from 'react';
import { navigate } from '../nav.js';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { fmtInt, usd, fmtDate, useT } from '../i18n/index.jsx';
import { usePayment, MIN_USD, MAX_USD } from '../components/usePayment.js';
import { PayError, SettleBanners, PaymentsDisabled, GatewayPicker, BankRateNote } from '../components/PaymentNotices.jsx';
import { Skeleton, SkeletonStats, SkeletonTable } from '../components/Skeleton.jsx';
import EmptyState from '../components/EmptyState.jsx';
import Icon from '../components/Icon.jsx';

const QUICK = [5, 10, 25, 50];

const T = {
  fa: {
    title: 'حساب و مصرف',
    subtitle: 'موجودی، مصرف کلیدهای شما و شارژ حساب',
    notYet: 'هنوز تأییدی از درگاه نرسیده. این چند دقیقه طبیعی است؛ کمی بعد دوباره بررسی کنید.',
    checking: 'در حال پرسیدن از درگاه…',
    loadErr: 'اطلاعات حساب بارگیری نشد. صفحه را دوباره باز کنید.',
    remaining: 'موجودی باقی‌مانده',
    byModel: 'مصرف به تفکیک مدل',
    byModelSub: 'مرتب‌شده بر اساس هزینه.',
    colModel: 'مدل',
    daily: 'مصرف روزانه',
    dailySub: '{n} روز اخیر',
    noDaily: 'در {n} روز اخیر مصرفی ثبت نشده.',
    dayReq: '{n} درخواست',
    pending: (v) => <><strong>{v.n} پرداخت در انتظار تأیید.</strong> اگر مبلغ از حسابتان کم شده، دکمهٔ کنار را بزنید؛ وضعیت از خودِ درگاه پرسیده می‌شود و به‌محض تأیید، موجودی اضافه می‌شود.</>,
    recheck: 'بررسی دوباره',
    zero: () => <><strong>موجودی صفر است.</strong> همهٔ کلیدهای شما با خطای ۴۰۲ رد می‌شوند تا شارژ کنید. با هر مبلغی، همان لحظه دوباره کار می‌کنند.</>,
    low: () => <><strong>موجودی زیر ۱ دلار.</strong> پاسخ‌ها هدر <code dir="ltr">X-Nabu-Balance-Warning: low</code> دارند؛ پیش از توقف کلیدها شارژ کنید.</>,
    email: 'ایمیل',
    balance: 'موجودی باقی‌مانده',
    spent: 'هزینهٔ مصرف‌شده',
    requests: 'درخواست‌ها',
    byKey: 'مصرف کلیدهای شما',
    byKeySub: 'تفکیک به‌ازای هر کلید. فقط کلیدهای خودتان.',
    recent: 'درخواست‌های اخیر',
    emptyTitle: 'هنوز درخواستی ثبت نشده',
    emptyHint: 'بعد از اولین تماس با کلیدتان، مصرف هر کلید اینجا با توکن و هزینه دیده می‌شود.',
    colKey: 'کلید',
    colRequests: 'درخواست',
    colIn: 'توکن ورودی',
    colOut: 'توکن خروجی',
    colDenied: 'رد‌شده',
    colCost: 'هزینه',
    topUp: 'افزایش موجودی',
    topUpNote: 'به درگاه بانکی منتقل می‌شوید. اطلاعات کارت را همان‌جا وارد می‌کنید و هرگز در این پنل ذخیره نمی‌شود؛ موجودی بعد از تأیید خودِ درگاه اضافه می‌شود.',
    dollars: 'دلار',
    redirecting: 'در حال انتقال به درگاه…',
    pay: 'پرداخت',
    plansLink: (v) => <>بسته‌های آماده و پاسخ به مشکلات پرداخت در {v.link}.</>,
    plansLinkText: 'خرید و شارژ',
    usdNote: 'همهٔ مبالغ به دلار (USD) است، همان واحدی که دروازه هزینهٔ مدل‌ها را با آن حساب می‌کند.',
  },
  en: {
    title: 'Account & usage',
    subtitle: 'Your balance, per-key usage and top-ups',
    notYet: 'No confirmation from the gateway yet. A few minutes is normal; check again shortly.',
    checking: 'Asking the gateway…',
    loadErr: 'Could not load your account. Reopen the page to try again.',
    remaining: 'Remaining balance',
    byModel: 'Usage by model',
    byModelSub: 'Sorted by cost.',
    colModel: 'Model',
    daily: 'Daily usage',
    dailySub: 'Last {n} days',
    noDaily: 'No usage in the last {n} days.',
    dayReq: '{n} requests',
    pending: (v) => <><strong>{v.n} payment(s) awaiting confirmation.</strong> If the amount has left your account, press the button alongside; the status is fetched from the gateway itself and your balance is credited as soon as it confirms.</>,
    recheck: 'Check again',
    zero: () => <><strong>Your balance is zero.</strong> All your keys are refused with 402 until you top up. Any amount brings them back instantly.</>,
    low: () => <><strong>Balance under $1.</strong> Responses carry the <code dir="ltr">X-Nabu-Balance-Warning: low</code> header; top up before your keys stop.</>,
    email: 'Email',
    balance: 'Remaining balance',
    spent: 'Total spend',
    requests: 'Requests',
    byKey: 'Usage by key',
    byKeySub: 'Broken down per key. Only your own keys.',
    recent: 'Recent requests',
    emptyTitle: 'No requests yet',
    emptyHint: 'After the first call with your key, each key’s tokens and cost show up here.',
    colKey: 'Key',
    colRequests: 'Requests',
    colIn: 'Input tokens',
    colOut: 'Output tokens',
    colDenied: 'Denied',
    colCost: 'Cost',
    topUp: 'Add funds',
    topUpNote: 'You will be sent to the bank gateway. Card details are entered there and never stored in this console; your balance is credited once the gateway confirms.',
    dollars: 'USD',
    redirecting: 'Redirecting to the gateway…',
    pay: 'Pay',
    plansLink: (v) => <>Ready-made packs and help with payment issues under {v.link}.</>,
    plansLinkText: 'Plans & top-up',
    usdNote: 'All amounts are in US dollars (USD), the same unit the gateway prices models in.',
  },
};

// "YYYY-MM-DD" read as a local calendar day, not UTC midnight, so the label
// never slips to the previous day west of Greenwich.
const day = (d) => {
  const [y, m, dd] = String(d).split('-').map(Number);
  return y && m && dd ? new Date(y, m - 1, dd) : d;
};

export default function Account() {
  const t = useT(T);
  const [user, setUser] = useState(null);
  const [usage, setUsage] = useState(null);
  const [status, setStatus] = useState(null);
  const [amount, setAmount] = useState(10);
  const [gateway, setGateway] = useState('');
  const [loadFailed, setLoadFailed] = useState(false);
  const [notYet, setNotYet] = useState(false);

  // The balance and the spend come from two endpoints, and the recharge form
  // only changes the first — so reloading both after a top-up would refetch a
  // usage report that cannot have moved.
  const loadUser = () => api.getMe().then(setUser).catch((e) => { if (!e.unauthenticated) setLoadFailed(true); });

  // Handles both halves: sending the payer to the gateway, and crediting the
  // wallet when they come back. loadUser refreshes the balance on the way in.
  const payment = usePayment(() => { loadUser(); });

  useEffect(() => {
    loadUser();
    api.myUsage().then(setUsage).catch((e) => { if (!e.unauthenticated) setLoadFailed(true); });
    api.status().then(setStatus).catch(() => setStatus({}));
  }, []);

  async function checkAgain() {
    setNotYet(false);
    const res = await payment.recheck();
    // Not confirmed yet is information, not a failure: the money may well be
    // on its way.
    if (res && !res.credited && !res.subscribed) setNotYet(true);
  }

  // Unknown counts as off, like on the plans screen.
  const enabled = status?.payments_enabled === true;
  const amountOk = Number(amount) >= MIN_USD && Number(amount) <= MAX_USD;

  const projects = Object.entries(usage?.projects || {}).sort((a, b) => (b[1].cost_usd || 0) - (a[1].cost_usd || 0));
  const models = Array.isArray(usage?.by_model) ? usage.by_model : [];
  const daily = Array.isArray(usage?.daily) ? usage.daily : [];
  const maxDay = daily.reduce((m, d) => Math.max(m, d.cost_usd || 0), 0);
  const pending = (user?.payments || []).filter((p) => p.status === 'pending');
  const low = user && user.balance > 0 && user.balance < 1;
  const settled = payment.settled;

  return (
    <Layout title={t('title')} subtitle={t('subtitle')}>
      {loadFailed && <div className="card banner-error"><Icon name="alert" size={18} /><div>{t('loadErr')}</div></div>}
      <PayError error={payment.error} />
      <SettleBanners result={settled} hidePending={pending.length > 0} />
      {notYet && (
        <div className="callout" role="status">
          <span className="ci"><Icon name="clock" size={18} /></span>
          <div>{t('notYet')}</div>
        </div>
      )}
      {pending.length > 0 && !settled?.credited && (
        <div className="callout">
          <span className="ci"><Icon name="clock" size={18} /></span>
          <div style={{ flex: 1 }}>
            {t('pending', { n: fmtInt(pending.length) })}
          </div>
          <button className="btn btn-outline btn-sm" onClick={checkAgain} disabled={payment.checking}>{payment.checking ? t('checking') : t('recheck')}</button>
        </div>
      )}
      {user && user.balance <= 0 && (
        <div className="callout danger">
          <span className="ci"><Icon name="ban" size={18} /></span>
          <div>{t('zero')}</div>
        </div>
      )}
      {low && (
        <div className="callout warn">
          <span className="ci"><Icon name="alert" size={18} /></span>
          <div>{t('low')}</div>
        </div>
      )}
      {status && !enabled && <PaymentsDisabled />}

      {user === null ? (
        <SkeletonStats n={4} />
      ) : (
        <div className="grid-auto stagger">
          <div className="card kpi"><div className="kpi-label">{t('email')}</div><div className="kpi-value" style={{ fontSize: 15 }} dir="ltr">{user.email}</div></div>
          <div className="card kpi"><div className="kpi-label">{t('balance')}</div><div className="kpi-value ltr">{usd(user.balance)}</div></div>
          <div className="card kpi"><div className="kpi-label">{t('spent')}</div><div className="kpi-value ltr">{usage ? usd(usage.cost_usd) : <Skeleton w={80} h={26} />}</div></div>
          <div className="card kpi"><div className="kpi-label">{t('requests')}</div><div className="kpi-value">{usage ? fmtInt(usage.requests) : <Skeleton w={60} h={26} />}</div></div>
        </div>
      )}

      <div className="grid grid-2-wide">
        <div className="card">
          <div className="card-head">
            <div>
              <h3><Icon name="chart" size={17} />{t('byKey')}</h3>
              <p className="card-sub" style={{ marginBottom: 0 }}>{t('byKeySub')}</p>
            </div>
            <button className="btn btn-ghost" onClick={() => navigate('requests')}>{t('recent')}</button>
          </div>
          {usage === null ? (
            <SkeletonTable rows={4} cols={6} />
          ) : projects.length === 0 ? (
            <EmptyState icon={<Icon name="activity" size={22} />} title={t('emptyTitle')} hint={t('emptyHint')} />
          ) : (
            <table className="tbl" style={{ margin: 0 }}>
              <thead>
                <tr><th>{t('colKey')}</th><th>{t('colRequests')}</th><th>{t('colIn')}</th><th>{t('colOut')}</th><th>{t('colDenied')}</th><th>{t('colCost')}</th></tr>
              </thead>
              <tbody className="stagger">
                {projects.map(([name, c]) => (
                  <tr key={name}>
                    <td className="mono" dir="ltr">{name}</td>
                    <td>{fmtInt(c.requests)}</td>
                    <td>{fmtInt(c.prompt_tokens)}</td>
                    <td>{fmtInt(c.completion_tokens)}</td>
                    <td>{c.denied ? <span className="badge badge-fail">{fmtInt(c.denied)}</span> : <span className="muted">{fmtInt(0)}</span>}</td>
                    <td className="ltr" style={{ fontWeight: 700 }}>{usd(c.cost_usd)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>

        <div className="card">
          <div className="card-head"><h3><Icon name="wallet" size={17} />{t('topUp')}</h3></div>
          <p className="muted" style={{ marginBottom: 14, fontSize: 13, lineHeight: 1.8 }}>
            {t('topUpNote')}
          </p>
          {enabled && <GatewayPicker gateways={status?.payment_gateways} value={gateway} onChange={setGateway} />}
          <div className="chips" style={{ marginBottom: 12 }}>
            {QUICK.map((a) => (
              <button key={a} type="button" className={'chip' + (Number(amount) === a ? ' active' : '')} onClick={() => setAmount(a)} dir="ltr">{usd(a)}</button>
            ))}
          </div>
          <form className="pay-form" onSubmit={(e) => { e.preventDefault(); if (amountOk && enabled) payment.pay({ amount: Number(amount), gateway }); }}>
            <input type="number" className="input" value={amount} onChange={(e) => setAmount(e.target.value)} min={MIN_USD} max={MAX_USD} step="1" dir="ltr" />
            <span className="muted" style={{ fontSize: 13 }}>{t('dollars')}</span>
            <button className="btn btn-primary" disabled={payment.busy || !enabled || !amountOk}>
              {payment.busy ? t('redirecting') : t('pay')}
            </button>
          </form>
          <BankRateNote />
          <p className="muted" style={{ marginTop: 14, fontSize: 12, lineHeight: 1.8 }}>
            {t('plansLink', { link: <button type="button" className="linklike" onClick={() => navigate('plans')}>{t('plansLinkText')}</button> })}
          </p>
        </div>
      </div>

      <div className="grid grid-2-wide">
        <div className="card">
          <div className="card-head">
            <div>
              <h3><Icon name="cpu" size={17} />{t('byModel')}</h3>
              <p className="card-sub" style={{ marginBottom: 0 }}>{t('byModelSub')}</p>
            </div>
          </div>
          {usage === null ? (
            <SkeletonTable rows={4} cols={5} />
          ) : models.length === 0 ? (
            <EmptyState icon={<Icon name="cpu" size={22} />} title={t('emptyTitle')} hint={t('emptyHint')} />
          ) : (
            <table className="tbl" style={{ margin: 0 }}>
              <thead>
                <tr><th>{t('colModel')}</th><th>{t('colRequests')}</th><th>{t('colIn')}</th><th>{t('colOut')}</th><th>{t('colCost')}</th></tr>
              </thead>
              <tbody>
                {models.map((m) => (
                  <tr key={m.model}>
                    <td className="mono cell-wrap" dir="ltr">{m.model}</td>
                    <td>{fmtInt(m.requests)}</td>
                    <td>{fmtInt(m.prompt_tokens)}</td>
                    <td>{fmtInt(m.completion_tokens)}</td>
                    <td className="ltr" style={{ fontWeight: 700 }}>{usd(m.cost_usd)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>

        <div className="card">
          <div className="card-head">
            <div>
              <h3><Icon name="activity" size={17} />{t('daily')}</h3>
              <p className="card-sub" style={{ marginBottom: 0 }}>{t('dailySub', { n: fmtInt(30) })}</p>
            </div>
          </div>
          {usage === null ? (
            <SkeletonTable rows={6} cols={2} />
          ) : daily.length === 0 ? (
            <p className="muted" style={{ fontSize: 13, margin: 0 }}>{t('noDaily', { n: fmtInt(30) })}</p>
          ) : (
            <ul className="day-bars">
              {[...daily].reverse().map((d) => (
                <li key={d.date} title={t('dayReq', { n: fmtInt(d.requests) })}>
                  <span className="day-date">{fmtDate(day(d.date))}</span>
                  <span className="day-track">
                    <span className="day-fill" style={{ inlineSize: maxDay > 0 ? `${Math.max(2, ((d.cost_usd || 0) / maxDay) * 100)}%` : '0%' }} />
                  </span>
                  <span className="day-cost ltr">{usd(d.cost_usd)}</span>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>

      <p className="muted" style={{ fontSize: 12 }}>
        {t('usdNote')}
      </p>
    </Layout>
  );
}
