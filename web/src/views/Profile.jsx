import { navigate } from "../nav.js";

import { useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';

export default function Profile() {
  const [user, setUser] = useState(null);
  const [error, setError] = useState(null);

  const load = () => {
    api.getMe().then(setUser).catch((e) => setError(e.message));
  };

  useEffect(load, []);

  const balance = user ? user.balance || 0 : 0;
  const payments = (user && user.payments) ? [...user.payments].reverse() : [];

  return (
    <Layout title="پروفایل کاربری" subtitle="مدیریت اطلاعات حساب و تاریخچه تراکنش‌ها">
      {error && <div className="card banner-error">{error}</div>}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(320px, 1fr))', gap: 24, paddingBottom: 32 }}>
        
        {/* Account Info */}
        <div className="card">
          <h3 style={{ fontSize: 16, marginBottom: 20, paddingBottom: 12, borderBottom: '1px solid var(--ng-border)' }}>اطلاعات حساب</h3>
          
          <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
            <div>
              <label style={{ display: 'block', fontSize: 13, color: 'var(--ng-muted)', marginBottom: 6 }}>ایمیل (شناسه ورود)</label>
              <input type="text" className="input" value={user?.email || ''} readOnly style={{ width: '100%', opacity: 0.7 }} dir="ltr" />
              <p style={{ fontSize: 12, color: 'var(--ng-muted)', marginTop: 4 }}>این ایمیل برای ورود و ارتباطات استفاده می‌شود.</p>
            </div>
            
            <div>
              <label style={{ display: 'block', fontSize: 13, color: 'var(--ng-muted)', marginBottom: 6 }}>موجودی حساب</label>
              <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                <input type="text" className="input" value={balance.toLocaleString('fa-IR')} readOnly style={{ width: 150, fontWeight: 700 }} />
                <span style={{ fontSize: 13, color: 'var(--ng-muted)' }}>تومان</span>
                <button className="btn btn-primary" style={{ marginRight: 'auto' }} onClick={() => navigate('plans')}>
                  شارژ حساب
                </button>
              </div>
            </div>
            
            <div>
              <label style={{ display: 'block', fontSize: 13, color: 'var(--ng-muted)', marginBottom: 6 }}>رمز عبور و امنیت</label>
              <div style={{ fontSize: 13, padding: '12px 16px', background: 'var(--ng-surface-soft)', borderRadius: 8, border: '1px solid var(--ng-border)', lineHeight: 1.6 }}>
                شما از طریق <strong>SSO (NabuAuth)</strong> وارد شده‌اید. برای تغییر رمز عبور یا مدیریت امنیت حساب خود، لطفاً از حساب کاربری خارج شده و از طریق پنل اصلی NabuAuth اقدام کنید.
              </div>
            </div>
          </div>
        </div>

        {/* Transaction History */}
        <div className="card" style={{ display: 'flex', flexDirection: 'column' }}>
          <h3 style={{ fontSize: 16, marginBottom: 20, paddingBottom: 12, borderBottom: '1px solid var(--ng-border)' }}>تاریخچه پرداخت‌ها (NabuPay)</h3>
          
          <div style={{ flex: 1 }}>
            {payments.length === 0 ? (
              <div style={{ padding: '40px 20px', textAlign: 'center', color: 'var(--ng-muted)' }}>
                <div style={{ fontSize: 32, marginBottom: 12, opacity: 0.5 }}>🧾</div>
                <div style={{ fontSize: 14, fontWeight: 700, marginBottom: 8, color: 'var(--ng-heading)' }}>تراکنشی یافت نشد</div>
                <div style={{ fontSize: 13 }}>شما هنوز هیچ پرداختی انجام نداده‌اید.</div>
              </div>
            ) : (
              <table className="tbl" style={{ border: 'none', margin: 0 }}>
                <thead>
                  <tr>
                    <th>تاریخ</th>
                    <th>مبلغ (تومان)</th>
                    <th>کد پیگیری</th>
                    <th>وضعیت</th>
                  </tr>
                </thead>
                <tbody>
                  {payments.map(p => (
                    <tr key={p.id}>
                      <td style={{ fontSize: 13, color: 'var(--ng-muted)' }} dir="ltr">
                        {new Date(p.created_at).toLocaleDateString('fa-IR')}
                      </td>
                      <td style={{ fontWeight: 700 }}>
                        {p.amount.toLocaleString('fa-IR')}
                      </td>
                      <td className="mono" style={{ fontSize: 12 }}>
                        {p.id.split('_')[1] || p.id}
                      </td>
                      <td>
                        <span className="badge badge-pass" style={{ fontSize: 11 }}>
                          موفق
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>

      </div>
    </Layout>
  );
}
