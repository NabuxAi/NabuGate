import { useEffect, useState } from 'react';
import { navigate } from '../nav.js';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { usd, faInt } from '../data/mock.js';
import { SkeletonTable } from '../components/Skeleton.jsx';
import EmptyState from '../components/EmptyState.jsx';

const STATUS = {
  success: { label: 'موفق', cls: 'badge-ok' },
  pending: { label: 'در انتظار تأیید', cls: 'badge-warn' },
  failed: { label: 'ناموفق', cls: 'badge-fail' },
};

export default function Payments() {
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
      title="پرداخت‌ها"
      subtitle="تاریخچهٔ شارژها و وضعیت هر تراکنش"
      actions={
        <>
          <button className="btn btn-outline" onClick={check} disabled={checking || user === null}>
            {checking ? 'در حال پرسیدن از درگاه…' : '⟳ بررسی وضعیت'}
          </button>
          <button className="btn btn-primary" onClick={() => navigate('plans')}>+ شارژ</button>
        </>
      }
    >
      {error && <div className="card banner-error"><span>⚠️</span>{error}</div>}
      {checked?.credited && <div className="card banner-ok"><span>✓</span>پرداخت تأیید شد و موجودی اضافه شد. موجودی فعلی: <b className="ltr">{usd(checked.balance)}</b></div>}
      {checked && !checked.credited && checked.payments?.length === 0 && (
        <div className="card banner-ok"><span>✓</span>پرداختِ در انتظاری وجود ندارد.</div>
      )}

      {pending.length > 0 && (
        <div className="callout warn">
          <span className="ci">⏳</span>
          <div>
            <strong>{faInt(pending.length)} پرداخت در انتظار تأیید دارید.</strong> اگر مبلغ از حسابتان کم شده، «بررسی وضعیت» را بزنید؛ پنل از خودِ درگاه می‌پرسد و به‌محض تأیید، موجودی یک‌بار اضافه می‌شود. پرداختی که تأیید نشود، ظرف ۷۲ ساعت از طرف بانک برگشت می‌خورد.
          </div>
        </div>
      )}

      <div className="grid-auto">
        <div className="card kpi"><div className="kpi-label">موجودی فعلی</div><div className="kpi-value ltr">{user ? usd(user.balance) : '—'}</div></div>
        <div className="card kpi"><div className="kpi-label">مجموع شارژهای موفق</div><div className="kpi-value ltr">{usd(totalPaid)}</div></div>
        <div className="card kpi"><div className="kpi-label">تعداد تراکنش‌ها</div><div className="kpi-value">{faInt(payments.length)}</div></div>
      </div>

      <div className="card">
        {user === null ? (
          <SkeletonTable rows={5} cols={4} />
        ) : payments.length === 0 ? (
          <EmptyState
            icon="💳"
            title="هنوز پرداختی ثبت نشده"
            hint="اولین شارژ را از بخش «خرید و شارژ» انجام دهید. هر تراکنش، حتی ناموفق، اینجا با شناسه‌اش می‌ماند تا برای پیگیری دست‌تان باشد."
            action={<button className="btn btn-primary btn-sm" onClick={() => navigate('plans')}>شارژ حساب</button>}
          />
        ) : (
          <table className="tbl" style={{ margin: 0 }}>
            <thead>
              <tr><th>شناسهٔ فاکتور</th><th>مبلغ</th><th>وضعیت</th><th>تاریخ</th></tr>
            </thead>
            <tbody className="stagger">
              {payments.map((p) => {
                const st = STATUS[p.status] || { label: p.status, cls: 'badge-muted' };
                return (
                  <tr key={p.id}>
                    <td className="mono" dir="ltr" style={{ fontSize: 12 }}>{p.id}</td>
                    <td className="ltr" style={{ fontWeight: 700 }}>{usd(p.amount)}</td>
                    <td><span className={'badge ' + st.cls}>{st.label}</span></td>
                    <td dir="ltr" style={{ fontSize: 12, color: 'var(--ng-muted)' }}>{new Date(p.created_at).toLocaleString('fa-IR')}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>

      <p className="muted" style={{ fontSize: 12, lineHeight: 1.8 }}>
        برای پیگیری با پشتیبانی، شناسهٔ فاکتور را بفرستید. مبلغی که در این جدول می‌بینید اعتبارِ دلاری اضافه‌شده است؛ رقم تومانیِ پرداختی روی رسید بانک است.
      </p>
    </Layout>
  );
}
