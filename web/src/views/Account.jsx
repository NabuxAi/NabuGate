import { useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { faInt, faDigits, usd } from '../data/mock.js';
import { usePayment } from '../components/usePayment.js';

export default function Account() {
  const [user, setUser] = useState(null);
  const [usage, setUsage] = useState(null);
  const [amount, setAmount] = useState(10);
  const [error, setError] = useState(null);

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

  if (!user) {
    return (
      <Layout title="حساب کاربری">
        <div className="app-boot">…</div>
      </Layout>
    );
  }

  const projects = Object.entries(usage?.projects || {}).sort(
    (a, b) => (b[1].cost_usd || 0) - (a[1].cost_usd || 0),
  );

  return (
    <Layout title="حساب کاربری" subtitle="موجودی، مصرف کلیدهای شما و شارژ حساب.">
      {(error || payment.error) && <div className="card banner-error">{error || payment.error}</div>}
      {payment.settled?.credited && (
        <div className="card banner-ok">پرداخت تأیید شد و موجودی اضافه شد.</div>
      )}
      {payment.settled && !payment.settled.credited && payment.settled.payments?.length > 0 && (
        <div className="card banner-error">
          پرداختِ در انتظار دارید. اگر مبلغ از حسابتان کم شده، همین صفحه را
          دوباره باز کنید؛ هر بار که این صفحه باز شود وضعیت از خودِ درگاه پرسیده
          می‌شود و به‌محضِ تأیید، موجودی اضافه می‌شود.
        </div>
      )}

      <div className="stats">
        <div className="stat">
          <div className="stat-label">ایمیل</div>
          <div className="stat-value" style={{ fontSize: 16 }} dir="ltr">{user.email}</div>
        </div>
        <div className="stat">
          <div className="stat-label">موجودی</div>
          <div className="stat-value ltr">{usd(user.balance)}</div>
        </div>
        <div className="stat">
          <div className="stat-label">هزینهٔ مصرف‌شده</div>
          <div className="stat-value ltr">{usd(usage?.cost_usd)}</div>
        </div>
        <div className="stat">
          <div className="stat-label">درخواست‌ها</div>
          <div className="stat-value">{faInt(usage?.requests)}</div>
        </div>
      </div>

      <div className="card" style={{ marginTop: 16 }}>
        <h3 style={{ marginBottom: 4 }}>مصرف کلیدهای شما</h3>
        <p className="muted" style={{ marginBottom: 16, fontSize: 13 }}>
          تفکیک به‌ازای هر کلید. فقط کلیدهای خودتان؛ مصرف کلِ دروازه در پنل مدیریت است.
        </p>

        {usage === null ? (
          <div className="app-boot">…</div>
        ) : projects.length === 0 ? (
          <div style={{ textAlign: 'center', padding: '32px 0', color: 'var(--ng-muted)' }}>
            <div style={{ fontSize: 32, marginBottom: 12, opacity: 0.5 }}>◴</div>
            هنوز هیچ درخواستی با کلیدهای شما ثبت نشده است.
          </div>
        ) : (
          <table className="tbl" style={{ margin: 0 }}>
            <thead>
              <tr>
                <th>کلید</th>
                <th>درخواست</th>
                <th>توکن ورودی</th>
                <th>توکن خروجی</th>
                <th>رد‌شده</th>
                <th>هزینه</th>
              </tr>
            </thead>
            <tbody>
              {projects.map(([name, c]) => (
                <tr key={name}>
                  <td className="mono" dir="ltr">{name}</td>
                  <td>{faInt(c.requests)}</td>
                  <td>{faInt(c.prompt_tokens)}</td>
                  <td>{faInt(c.completion_tokens)}</td>
                  <td>
                    {/* A refused request is the one thing a token total hides
                        completely, and the first thing to look at when a key
                        seems to have stopped working. */}
                    {c.denied ? (
                      <span className="badge badge-fail" style={{ fontSize: 11 }}>{faInt(c.denied)}</span>
                    ) : (
                      <span className="muted">۰</span>
                    )}
                  </td>
                  <td className="ltr" style={{ fontWeight: 700 }}>{usd(c.cost_usd)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="card" style={{ marginTop: 16 }}>
        <h3 style={{ marginBottom: 4 }}>افزایش موجودی</h3>
        <p className="muted" style={{ marginBottom: 16, fontSize: 13, lineHeight: 1.7 }}>
          به درگاه بانکی منتقل می‌شوید. اطلاعات کارت را همان‌جا وارد می‌کنید و
          هرگز در این پنل ذخیره نمی‌شود؛ موجودی بعد از تأیید خودِ درگاه اضافه می‌شود.
        </p>
        <form onSubmit={(e) => { e.preventDefault(); payment.pay(amount); }} style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <input
            type="number"
            className="input"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            min="1"
            step="1"
            style={{ width: 120 }}
            dir="ltr"
          />
          <span>دلار</span>
          <button className="btn btn-primary" disabled={payment.busy}>
            {payment.busy ? 'در حال انتقال به درگاه…' : 'پرداخت'}
          </button>
        </form>
      </div>

      <p className="muted" style={{ marginTop: 12, fontSize: 12 }}>
        {faDigits('همهٔ مبالغ به دلار (USD) است — همان واحدی که دروازه هزینهٔ مدل‌ها را با آن حساب می‌کند.')}
      </p>
    </Layout>
  );
}
