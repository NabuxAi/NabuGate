import { useEffect, useState } from 'react';
import { navigate } from '../nav.js';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { fmtInt, usd, useT } from '../i18n/index.jsx';
import { usePayment } from '../components/usePayment.js';
import { Skeleton, SkeletonStats, SkeletonTable } from '../components/Skeleton.jsx';
import EmptyState from '../components/EmptyState.jsx';
import Icon from '../components/Icon.jsx';

const QUICK = [5, 10, 25, 50];

const T = {
  fa: {
    title: 'حساب و مصرف',
    subtitle: 'موجودی، مصرف کلیدهای شما و شارژ حساب',
    notYet: 'هنوز تأییدی از درگاه نرسیده. چند دقیقه بعد دوباره بررسی کنید.',
    credited: 'پرداخت تأیید شد و موجودی اضافه شد.',
    pending: (v) => <><strong>{v.n} پرداخت در انتظار تأیید.</strong> اگر مبلغ از حسابتان کم شده، دکمهٔ کنار را بزنید؛ وضعیت از خودِ درگاه پرسیده می‌شود و به‌محض تأیید، موجودی اضافه می‌شود.</>,
    recheck: 'بررسی دوباره',
    zero: () => <><strong>موجودی صفر است.</strong> همهٔ کلیدهای شما با خطای ۴۰۲ رد می‌شوند تا شارژ کنید. با هر مبلغی، همان لحظه دوباره کار می‌کنند.</>,
    low: () => <><strong>موجودی زیر ۱ دلار.</strong> پاسخ‌ها هدر <code dir="ltr">X-Nabu-Balance-Warning: low</code> دارند؛ پیش از توقف کلیدها شارژ کنید.</>,
    email: 'ایمیل',
    balance: 'موجودی',
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
    notYet: 'No confirmation from the gateway yet. Check again in a few minutes.',
    credited: 'Payment confirmed and your balance has been credited.',
    pending: (v) => <><strong>{v.n} payment(s) awaiting confirmation.</strong> If the amount has left your account, press the button alongside; the status is fetched from the gateway itself and your balance is credited as soon as it confirms.</>,
    recheck: 'Check again',
    zero: () => <><strong>Your balance is zero.</strong> All your keys are refused with 402 until you top up. Any amount brings them back instantly.</>,
    low: () => <><strong>Balance under $1.</strong> Responses carry the <code dir="ltr">X-Nabu-Balance-Warning: low</code> header; top up before your keys stop.</>,
    email: 'Email',
    balance: 'Balance',
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

export default function Account() {
  const t = useT(T);
  const [user, setUser] = useState(null);
  const [usage, setUsage] = useState(null);
  const [amount, setAmount] = useState(10);
  const [error, setError] = useState(null);
  const [recheck, setRecheck] = useState(false);

  // The balance and the spend come from two endpoints, and the recharge form
  // only changes the first — so reloading both after a top-up would refetch a
  // usage report that cannot have moved.
  const loadUser = () => api.getMe().then(setUser).catch((e) => setError(e.message));

  // Handles both halves: sending the payer to the gateway, and crediting the
  // wallet when they come back. loadUser refreshes the balance on the way in.
  const payment = usePayment(loadUser);

  useEffect(() => {
    loadUser();
    api.myUsage().then(setUsage).catch((e) => setError(e.message));
  }, []);

  async function checkAgain() {
    setRecheck(true);
    try {
      const res = await api.settleMyPayments();
      if (res?.credited) await loadUser();
      payment.setError(res?.credited ? null : t('notYet'));
    } catch (e) {
      payment.setError(e.message);
    } finally {
      setRecheck(false);
    }
  }

  const projects = Object.entries(usage?.projects || {}).sort((a, b) => (b[1].cost_usd || 0) - (a[1].cost_usd || 0));
  const pending = (user?.payments || []).filter((p) => p.status === 'pending');
  const low = user && user.balance > 0 && user.balance < 1;

  return (
    <Layout title={t('title')} subtitle={t('subtitle')}>
      {(error || payment.error) && <div className="card banner-error"><span>⚠️</span>{error || payment.error}</div>}
      {payment.settled?.credited && <div className="card banner-ok"><span>✓</span>{t('credited')}</div>}
      {pending.length > 0 && !payment.settled?.credited && (
        <div className="callout warn">
          <span className="ci">⏳</span>
          <div style={{ flex: 1 }}>
            {t('pending', { n: fmtInt(pending.length) })}
          </div>
          <button className="btn btn-outline btn-sm" onClick={checkAgain} disabled={recheck}>{recheck ? '…' : t('recheck')}</button>
        </div>
      )}
      {user && user.balance <= 0 && (
        <div className="callout danger">
          <span className="ci">⛔</span>
          <div>{t('zero')}</div>
        </div>
      )}
      {low && (
        <div className="callout warn">
          <span className="ci">⚠️</span>
          <div>{t('low')}</div>
        </div>
      )}

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
          <div className="chips" style={{ marginBottom: 12 }}>
            {QUICK.map((a) => (
              <button key={a} type="button" className={'chip' + (Number(amount) === a ? ' active' : '')} onClick={() => setAmount(a)} dir="ltr">{usd(a)}</button>
            ))}
          </div>
          <form className="pay-form" onSubmit={(e) => { e.preventDefault(); payment.pay(amount); }}>
            <input type="number" className="input" value={amount} onChange={(e) => setAmount(e.target.value)} min="1" max="5000" step="1" dir="ltr" />
            <span className="muted" style={{ fontSize: 13 }}>{t('dollars')}</span>
            <button className="btn btn-primary" disabled={payment.busy || Number(amount) < 1}>
              {payment.busy ? t('redirecting') : t('pay')}
            </button>
          </form>
          <p className="muted" style={{ marginTop: 14, fontSize: 12, lineHeight: 1.8 }}>
            {t('plansLink', { link: <button type="button" className="linklike" onClick={() => navigate('plans')}>{t('plansLinkText')}</button> })}
          </p>
        </div>
      </div>

      <p className="muted" style={{ fontSize: 12 }}>
        {t('usdNote')}
      </p>
    </Layout>
  );
}
