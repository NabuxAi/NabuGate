import { useEffect, useState } from 'react';
import { navigate } from '../nav.js';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { faInt, usd } from '../data/mock.js';
import { usePayment } from '../components/usePayment.js';
import { Skeleton, SkeletonStats, SkeletonTable } from '../components/Skeleton.jsx';
import EmptyState from '../components/EmptyState.jsx';

const QUICK = [5, 10, 25, 50];

export default function Account() {
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
      payment.setError(res?.credited ? null : 'هنوز تأییدی از درگاه نرسیده. چند دقیقه بعد دوباره بررسی کنید.');
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
    <Layout title="حساب و مصرف" subtitle="موجودی، مصرف کلیدهای شما و شارژ حساب">
      {(error || payment.error) && <div className="card banner-error"><span>⚠️</span>{error || payment.error}</div>}
      {payment.settled?.credited && <div className="card banner-ok"><span>✓</span>پرداخت تأیید شد و موجودی اضافه شد.</div>}
      {pending.length > 0 && !payment.settled?.credited && (
        <div className="callout warn">
          <span className="ci">⏳</span>
          <div style={{ flex: 1 }}>
            <strong>{faInt(pending.length)} پرداخت در انتظار تأیید.</strong> اگر مبلغ از حسابتان کم شده، دکمهٔ کنار را بزنید؛ وضعیت از خودِ درگاه پرسیده می‌شود و به‌محض تأیید، موجودی اضافه می‌شود.
          </div>
          <button className="btn btn-outline btn-sm" onClick={checkAgain} disabled={recheck}>{recheck ? '…' : 'بررسی دوباره'}</button>
        </div>
      )}
      {user && user.balance <= 0 && (
        <div className="callout danger">
          <span className="ci">⛔</span>
          <div><strong>موجودی صفر است.</strong> همهٔ کلیدهای شما با خطای ۴۰۲ رد می‌شوند تا شارژ کنید. با هر مبلغی، همان لحظه دوباره کار می‌کنند.</div>
        </div>
      )}
      {low && (
        <div className="callout warn">
          <span className="ci">⚠️</span>
          <div><strong>موجودی زیر ۱ دلار.</strong> پاسخ‌ها هدر <code dir="ltr">X-Nabu-Balance-Warning: low</code> دارند؛ پیش از توقف کلیدها شارژ کنید.</div>
        </div>
      )}

      {user === null ? (
        <SkeletonStats n={4} />
      ) : (
        <div className="grid-auto stagger">
          <div className="card kpi"><div className="kpi-label">ایمیل</div><div className="kpi-value" style={{ fontSize: 15 }} dir="ltr">{user.email}</div></div>
          <div className="card kpi"><div className="kpi-label">موجودی</div><div className="kpi-value ltr">{usd(user.balance)}</div></div>
          <div className="card kpi"><div className="kpi-label">هزینهٔ مصرف‌شده</div><div className="kpi-value ltr">{usage ? usd(usage.cost_usd) : <Skeleton w={80} h={26} />}</div></div>
          <div className="card kpi"><div className="kpi-label">درخواست‌ها</div><div className="kpi-value">{usage ? faInt(usage.requests) : <Skeleton w={60} h={26} />}</div></div>
        </div>
      )}

      <div className="grid grid-2-wide">
        <div className="card">
          <div className="card-head">
            <div>
              <h3>مصرف کلیدهای شما</h3>
              <p className="card-sub" style={{ marginBottom: 0 }}>تفکیک به‌ازای هر کلید. فقط کلیدهای خودتان.</p>
            </div>
            <button className="btn btn-ghost" onClick={() => navigate('requests')}>درخواست‌های اخیر</button>
          </div>
          {usage === null ? (
            <SkeletonTable rows={4} cols={6} />
          ) : projects.length === 0 ? (
            <EmptyState icon="◴" title="هنوز درخواستی ثبت نشده" hint="بعد از اولین تماس با کلیدتان، مصرف هر کلید اینجا با توکن و هزینه دیده می‌شود." />
          ) : (
            <table className="tbl" style={{ margin: 0 }}>
              <thead>
                <tr><th>کلید</th><th>درخواست</th><th>توکن ورودی</th><th>توکن خروجی</th><th>رد‌شده</th><th>هزینه</th></tr>
              </thead>
              <tbody className="stagger">
                {projects.map(([name, c]) => (
                  <tr key={name}>
                    <td className="mono" dir="ltr">{name}</td>
                    <td>{faInt(c.requests)}</td>
                    <td>{faInt(c.prompt_tokens)}</td>
                    <td>{faInt(c.completion_tokens)}</td>
                    <td>{c.denied ? <span className="badge badge-fail">{faInt(c.denied)}</span> : <span className="muted">۰</span>}</td>
                    <td className="ltr" style={{ fontWeight: 700 }}>{usd(c.cost_usd)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>

        <div className="card">
          <div className="card-head"><h3>افزایش موجودی</h3></div>
          <p className="muted" style={{ marginBottom: 14, fontSize: 13, lineHeight: 1.8 }}>
            به درگاه بانکی منتقل می‌شوید. اطلاعات کارت را همان‌جا وارد می‌کنید و هرگز در این پنل ذخیره نمی‌شود؛ موجودی بعد از تأیید خودِ درگاه اضافه می‌شود.
          </p>
          <div className="chips" style={{ marginBottom: 12 }}>
            {QUICK.map((a) => (
              <button key={a} type="button" className={'chip' + (Number(amount) === a ? ' active' : '')} onClick={() => setAmount(a)} dir="ltr">{usd(a)}</button>
            ))}
          </div>
          <form onSubmit={(e) => { e.preventDefault(); payment.pay(amount); }} style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
            <input type="number" className="input" value={amount} onChange={(e) => setAmount(e.target.value)} min="1" max="5000" step="1" style={{ width: 110 }} dir="ltr" />
            <span className="muted" style={{ fontSize: 13 }}>دلار</span>
            <button className="btn btn-primary" disabled={payment.busy || Number(amount) < 1}>
              {payment.busy ? 'در حال انتقال به درگاه…' : 'پرداخت'}
            </button>
          </form>
          <p className="muted" style={{ marginTop: 14, fontSize: 12, lineHeight: 1.8 }}>
            بسته‌های آماده و پاسخ به مشکلات پرداخت در <button type="button" className="linklike" onClick={() => navigate('plans')}>خرید و شارژ</button>.
          </p>
        </div>
      </div>

      <p className="muted" style={{ fontSize: 12 }}>
        همهٔ مبالغ به دلار (USD) است، همان واحدی که دروازه هزینهٔ مدل‌ها را با آن حساب می‌کند.
      </p>
    </Layout>
  );
}
