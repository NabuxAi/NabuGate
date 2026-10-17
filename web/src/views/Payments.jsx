import { useEffect, useState } from 'react';
import { navigate } from '../nav.js';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { usd, fmtInt, fmtDate, useT } from '../i18n/index.jsx';
import { SkeletonTable } from '../components/Skeleton.jsx';
import { usePayment } from '../components/usePayment.js';
import { PayError, SettleBanners } from '../components/PaymentNotices.jsx';
import EmptyState from '../components/EmptyState.jsx';
import Icon from '../components/Icon.jsx';

// label is a key into T below.
const STATUS = {
  success: { label: 'stSuccess', cls: 'badge-ok' },
  pending: { label: 'stPending', cls: 'badge-warn' },
  failed: { label: 'stFailed', cls: 'badge-fail' },
  expired: { label: 'stExpired', cls: 'badge-muted' },
  underpaid: { label: 'stUnderpaid', cls: 'badge-fail' },
  'admin-recharge': { label: 'stAdmin', cls: 'badge-ok' },
  'plan-credit': { label: 'stPlanCredit', cls: 'badge-ok' },
};

const KIND = {
  topup: 'kindTopup',
  plan: 'kindPlan',
  'plan-credit': 'kindPlanCredit',
  admin: 'kindAdmin',
};

// Older rows carry no kind; their status still says what they were.
const kindOf = (p) => p.kind || (p.status === 'admin-recharge' ? 'admin' : p.status === 'plan-credit' ? 'plan-credit' : 'topup');

const T = {
  fa: {
    stSuccess: 'موفق',
    stPending: 'در انتظار تأیید',
    stFailed: 'ناموفق',
    stExpired: 'منقضی / لغوشده',
    stUnderpaid: 'کم‌پرداخت',
    stAdmin: 'شارژ دستیِ مدیر',
    stPlanCredit: 'اعتبار هدیهٔ اشتراک',
    kindTopup: 'شارژ',
    kindPlan: 'خرید اشتراک',
    kindPlanCredit: 'اعتبار هدیهٔ اشتراک',
    kindAdmin: 'شارژ دستیِ مدیر',
    colKind: 'نوع',
    stillAfterCheck: 'درگاه هنوز این پرداخت‌ها را تأیید نکرده است. این چند دقیقه طبیعی است؛ کمی بعد دوباره «بررسی وضعیت» را بزنید.',
    underpaidRow: 'کمتر از مبلغ فاکتور پرداخت شده؛ با این شناسه به پشتیبانی پیام دهید.',
    title: 'پرداخت‌ها',
    subtitle: 'تاریخچهٔ شارژها و وضعیت هر تراکنش',
    asking: 'در حال پرسیدن از درگاه…',
    checkStatus: 'بررسی وضعیت',
    topUp: 'شارژ',
    nonePending: 'پرداختِ در انتظاری وجود ندارد.',
    pending: (v) => <><strong>{v.n} پرداخت در انتظار تأیید دارید.</strong> اگر مبلغ از حسابتان کم شده، «بررسی وضعیت» را بزنید؛ پنل از خودِ درگاه می‌پرسد و به‌محض تأیید، موجودی یک‌بار اضافه می‌شود. پرداختی که تأیید نشود، ظرف ۷۲ ساعت از طرف بانک برگشت می‌خورد.</>,
    balance: 'موجودی فعلی',
    totalPaid: 'مجموع شارژهای موفق',
    count: 'تعداد تراکنش‌ها',
    emptyTitle: 'هنوز پرداختی ثبت نشده',
    emptyHint: 'اولین شارژ را از بخش «خرید و شارژ» انجام دهید. هر تراکنش، حتی ناموفق، اینجا با شناسه‌اش می‌ماند تا برای پیگیری دست‌تان باشد.',
    topUpAccount: 'شارژ حساب',
    colInvoice: 'شناسهٔ فاکتور',
    colAmount: 'مبلغ',
    colStatus: 'وضعیت',
    colDate: 'تاریخ',
    footer: 'برای پیگیری با پشتیبانی، شناسهٔ فاکتور را بفرستید. مبلغِ مثبت اعتبارِ دلاریِ اضافه‌شده و مبلغِ منفی خریدِ اشتراک از موجودی است؛ رقم تومانیِ پرداختی روی رسید بانک است.',
  },
  en: {
    stSuccess: 'Succeeded',
    stPending: 'Pending',
    stFailed: 'Failed',
    stExpired: 'Expired / cancelled',
    stUnderpaid: 'Underpaid',
    stAdmin: 'Manual top-up by admin',
    stPlanCredit: 'Plan bonus credit',
    kindTopup: 'Top-up',
    kindPlan: 'Plan purchase',
    kindPlanCredit: 'Plan bonus credit',
    kindAdmin: 'Manual (admin)',
    colKind: 'Type',
    stillAfterCheck: 'The gateway has not confirmed these payments yet. A few minutes is normal; press “Check status” again shortly.',
    underpaidRow: 'Less than the invoice was paid; contact support with this ID.',
    title: 'Payments',
    subtitle: 'Top-up history and the status of each transaction',
    asking: 'Asking the gateway…',
    checkStatus: 'Check status',
    topUp: 'Top up',
    nonePending: 'There are no pending payments.',
    pending: (v) => <><strong>You have {v.n} payment(s) awaiting confirmation.</strong> If the amount has left your account, press “Check status”; the console asks the gateway itself and credits your balance exactly once as soon as it confirms. A payment that is never confirmed is refunded by the bank within 72 hours.</>,
    balance: 'Current balance',
    totalPaid: 'Total successful top-ups',
    count: 'Transactions',
    emptyTitle: 'No payments yet',
    emptyHint: 'Make your first top-up under “Plans & top-up”. Every transaction, even a failed one, stays here with its ID so you have it for follow-up.',
    topUpAccount: 'Top up',
    colInvoice: 'Invoice ID',
    colAmount: 'Amount',
    colStatus: 'Status',
    colDate: 'Date',
    footer: 'To follow up with support, send the invoice ID. Positive amounts are USD credit added; negative ones are plan purchases paid from your balance. The toman amount you paid is on the bank receipt.',
  },
};

export default function Payments() {
  const t = useT(T);
  const [user, setUser] = useState(null);
  const [error, setError] = useState(null);
  const [checked, setChecked] = useState(false);

  const load = () => api.getMe().then(setUser).catch((e) => { if (!e.unauthenticated) setError(e); });
  useEffect(() => { load(); }, []);

  // Settles on mount like every payment screen — a payer who lands here from
  // the bank must not have to press anything to be credited — and reloads the
  // history whenever an invoice moved.
  const payment = usePayment(() => { load(); });

  // Asks the gateway about every invoice this account left pending. Safe to
  // repeat: each invoice is credited once, however many times it is asked.
  async function check() {
    setError(null);
    const res = await payment.recheck();
    if (res) {
      setChecked(true);
      await load();
    }
  }

  const payments = [...(user?.payments || [])].sort((a, b) => new Date(b.created_at) - new Date(a.created_at));
  const pending = payments.filter((p) => p.status === 'pending');
  const totalPaid = payments
    .filter((p) => p.status === 'success' && kindOf(p) === 'topup' && (p.amount || 0) > 0)
    .reduce((a, p) => a + (p.amount || 0), 0);
  const settled = payment.settled;

  // A debit reads as one: "−$9.00", not "$-9.00".
  const amountCell = (n) => {
    const v = Number(n || 0);
    return v < 0
      ? <span className="debit">−{usd(Math.abs(v))}</span>
      : usd(v);
  };

  return (
    <Layout
      title={t('title')}
      subtitle={t('subtitle')}
      actions={
        <>
          <button className="btn btn-outline" onClick={check} disabled={payment.checking || user === null}>
            {payment.checking ? t('asking') : <><Icon name="refresh" size={16} />{t('checkStatus')}</>}
          </button>
          <button className="btn btn-primary" onClick={() => navigate('plans')}><Icon name="plus" size={16} />{t('topUp')}</button>
        </>
      }
    >
      <PayError error={error || payment.error} />
      <SettleBanners result={settled} hidePending={pending.length > 0} />
      {checked && settled && !settled.credited && !settled.subscribed && !(settled.payments?.length) && pending.length === 0 && (
        <div className="card banner-ok"><Icon name="check" size={18} /><div>{t('nonePending')}</div></div>
      )}

      {pending.length > 0 && (
        <div className="callout" role="status">
          <span className="ci"><Icon name="clock" size={18} /></span>
          <div>
            {checked ? t('stillAfterCheck') : t('pending', { n: fmtInt(pending.length) })}
          </div>
        </div>
      )}

      <div className="grid-auto">
        <div className="card kpi"><div className="kpi-label">{t('balance')}</div><div className="kpi-value ltr">{user ? usd(user.balance) : '—'}</div></div>
        <div className="card kpi"><div className="kpi-label">{t('totalPaid')}</div><div className="kpi-value ltr">{usd(totalPaid)}</div></div>
        <div className="card kpi"><div className="kpi-label">{t('count')}</div><div className="kpi-value">{fmtInt(payments.length)}</div></div>
      </div>

      <div className="card">
        {user === null ? (
          <SkeletonTable rows={5} cols={5} />
        ) : payments.length === 0 ? (
          <EmptyState
            icon={<Icon name="card" size={22} />}
            title={t('emptyTitle')}
            hint={t('emptyHint')}
            action={<button className="btn btn-primary btn-sm" onClick={() => navigate('plans')}>{t('topUpAccount')}</button>}
          />
        ) : (
          <table className="tbl" style={{ margin: 0 }}>
            <thead>
              <tr><th>{t('colInvoice')}</th><th>{t('colKind')}</th><th>{t('colAmount')}</th><th>{t('colStatus')}</th><th>{t('colDate')}</th></tr>
            </thead>
            <tbody className="stagger">
              {payments.map((p) => {
                const known = STATUS[p.status];
                const st = known ? { label: t(known.label), cls: known.cls } : { label: p.status, cls: 'badge-muted' };
                const kind = KIND[kindOf(p)];
                return (
                  <tr key={p.id}>
                    <td className="mono" dir="ltr" style={{ fontSize: 12 }}>{p.id}</td>
                    <td>{kind ? t(kind) : <span dir="ltr">{p.kind}</span>}</td>
                    <td className="ltr" style={{ fontWeight: 700 }}>{amountCell(p.amount)}</td>
                    <td>
                      <span className={'badge ' + st.cls}>{st.label}</span>
                      {p.status === 'underpaid' && <div className="muted" style={{ fontSize: 11.5, marginTop: 4 }}>{t('underpaidRow')}</div>}
                    </td>
                    <td style={{ fontSize: 12, color: 'var(--ng-muted)' }}>{fmtDate(p.created_at, 'datetime')}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>

      <p className="muted" style={{ fontSize: 12, lineHeight: 1.8 }}>
        {t('footer')}
      </p>
    </Layout>
  );
}
