import { useEffect, useState } from 'react';
import { navigate } from '../nav.js';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { usd, fmtInt, fmtDate, useT } from '../i18n/index.jsx';
import { SkeletonTable } from '../components/Skeleton.jsx';
import EmptyState from '../components/EmptyState.jsx';
import Icon from '../components/Icon.jsx';

// label is a key into T below.
const STATUS = {
  success: { label: 'stSuccess', cls: 'badge-ok' },
  pending: { label: 'stPending', cls: 'badge-warn' },
  failed: { label: 'stFailed', cls: 'badge-fail' },
};

const T = {
  fa: {
    stSuccess: 'موفق',
    stPending: 'در انتظار تأیید',
    stFailed: 'ناموفق',
    title: 'پرداخت‌ها',
    subtitle: 'تاریخچهٔ شارژها و وضعیت هر تراکنش',
    asking: 'در حال پرسیدن از درگاه…',
    checkStatus: 'بررسی وضعیت',
    topUp: 'شارژ',
    credited: (v) => <>پرداخت تأیید شد و موجودی اضافه شد. موجودی فعلی: {v.balance}</>,
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
    footer: 'برای پیگیری با پشتیبانی، شناسهٔ فاکتور را بفرستید. مبلغی که در این جدول می‌بینید اعتبارِ دلاری اضافه‌شده است؛ رقم تومانیِ پرداختی روی رسید بانک است.',
  },
  en: {
    stSuccess: 'Succeeded',
    stPending: 'Pending',
    stFailed: 'Failed',
    title: 'Payments',
    subtitle: 'Top-up history and the status of each transaction',
    asking: 'Asking the gateway…',
    checkStatus: 'Check status',
    topUp: 'Top up',
    credited: (v) => <>Payment confirmed and your balance has been credited. Current balance: {v.balance}</>,
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
    footer: 'To follow up with support, send the invoice ID. The amounts in this table are the USD credit added; the toman amount you paid is on the bank receipt.',
  },
};

export default function Payments() {
  const t = useT(T);
  const [user, setUser] = useState(null);
  const [error, setError] = useState(null);
  const [checking, setChecking] = useState(false);
  const [checked, setChecked] = useState(null);

  const load = () => api.getMe().then(setUser).catch((e) => setError(e.message));
  useEffect(() => { load(); }, []);

  // Asks the gateway about every invoice this account left pending. Safe to
  // repeat: each invoice is credited once, however many times it is asked.
  async function check() {
    setChecking(true);
    setError(null);
    try {
      const res = await api.settleMyPayments();
      setChecked(res);
      await load();
    } catch (e) {
      setError(e.message);
    } finally {
      setChecking(false);
    }
  }

  const payments = [...(user?.payments || [])].sort((a, b) => new Date(b.created_at) - new Date(a.created_at));
  const pending = payments.filter((p) => p.status === 'pending');
  const totalPaid = payments.filter((p) => p.status === 'success').reduce((a, p) => a + (p.amount || 0), 0);

  return (
    <Layout
      title={t('title')}
      subtitle={t('subtitle')}
      actions={
        <>
          <button className="btn btn-outline" onClick={check} disabled={checking || user === null}>
            {checking ? t('asking') : <><Icon name="refresh" size={16} />{t('checkStatus')}</>}
          </button>
          <button className="btn btn-primary" onClick={() => navigate('plans')}><Icon name="plus" size={16} />{t('topUp')}</button>
        </>
      }
    >
      {error && <div className="card banner-error"><span>⚠️</span>{error}</div>}
      {checked?.credited && <div className="card banner-ok"><span>✓</span>{t('credited', { balance: <b className="ltr">{usd(checked.balance)}</b> })}</div>}
      {checked && !checked.credited && checked.payments?.length === 0 && (
        <div className="card banner-ok"><span>✓</span>{t('nonePending')}</div>
      )}

      {pending.length > 0 && (
        <div className="callout warn">
          <span className="ci">⏳</span>
          <div>
            {t('pending', { n: fmtInt(pending.length) })}
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
          <SkeletonTable rows={5} cols={4} />
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
              <tr><th>{t('colInvoice')}</th><th>{t('colAmount')}</th><th>{t('colStatus')}</th><th>{t('colDate')}</th></tr>
            </thead>
            <tbody className="stagger">
              {payments.map((p) => {
                const known = STATUS[p.status];
                const st = known ? { label: t(known.label), cls: known.cls } : { label: p.status, cls: 'badge-muted' };
                return (
                  <tr key={p.id}>
                    <td className="mono" dir="ltr" style={{ fontSize: 12 }}>{p.id}</td>
                    <td className="ltr" style={{ fontWeight: 700 }}>{usd(p.amount)}</td>
                    <td><span className={'badge ' + st.cls}>{st.label}</span></td>
                    <td dir="ltr" style={{ fontSize: 12, color: 'var(--ng-muted)' }}>{fmtDate(p.created_at, 'datetime')}</td>
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
