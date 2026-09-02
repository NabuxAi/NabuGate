import { navigate } from "../nav.js";

import { useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { usd } from '../data/mock.js';
import { SkeletonTable, Skeleton } from '../components/Skeleton.jsx';

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
                <input type="text" className="input" value={usd(balance)} readOnly style={{ width: 150, fontWeight: 700 }} dir="ltr" />
                <button className="btn btn-primary" style={{ marginRight: 'auto' }} onClick={() => navigate('plans')}>
                  شارژ حساب
                </button>
              </div>
            </div>
            
            <div>
              <label style={{ display: 'block', fontSize: 13, color: 'var(--ng-muted)', marginBottom: 6 }}>رمز عبور و امنیت</label>
              {/* This used to state flatly that the visitor had signed in
                  through NabuAuth and had to leave the panel to change a
                  password. Accounts created with the local sign-up form have a
                  password stored right here, and were being sent away to change
                  something this panel owns. */}
              <div style={{ fontSize: 13, padding: '12px 16px', background: 'var(--ng-surface-soft)', borderRadius: 8, border: '1px solid var(--ng-border)', lineHeight: 1.6 }}>
                رمز عبور حساب را در بخش <button type="button" className="linklike" onClick={() => navigate('security')}>امنیت</button> تغییر دهید.
                اگر با حساب نابو (SSO) وارد شده‌اید، رمزتان در NabuAuth مدیریت می‌شود و اینجا رمزی برای تغییر وجود ندارد.
              </div>
            </div>
          </div>
        </div>

        {/* Transaction History */}
        <div className="card" style={{ display: 'flex', flexDirection: 'column' }}>
          <h3 style={{ fontSize: 16, marginBottom: 20, paddingBottom: 12, borderBottom: '1px solid var(--ng-border)' }}>تاریخچه پرداخت‌ها</h3>
          
          <div style={{ flex: 1 }}>
            {user === null ? (
              <SkeletonTable rows={4} cols={4} />
            ) : payments.length === 0 ? (
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
                    <th>مبلغ</th>
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
                      <td style={{ fontWeight: 700 }} dir="ltr">
                        {usd(p.amount)}
                      </td>
                      <td className="mono" style={{ fontSize: 12 }}>
                        {p.id.split('_')[1] || p.id}
                      </td>
                      <td>
                        {/* Read from the record. This was hard-coded to
                            "موفق", so a failed payment was reported as a
                            successful one. */}
                        <span className={'badge ' + (p.status === 'success' ? 'badge-ok' : p.status === 'pending' ? 'badge-warn' : 'badge-fail')} style={{ fontSize: 11 }}>
                          {p.status === 'success' ? 'موفق' : p.status === 'pending' ? 'در انتظار' : 'ناموفق'}
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
