import { useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { usd } from '../data/mock.js';

export default function Payments() {
  const [user, setUser] = useState(null);
  const [error, setError] = useState(null);

  const load = () => {
    api.getMe().then(setUser).catch((e) => setError(e.message));
  };

  useEffect(load, []);

  const payments = user?.payments || [];
  
  // Sort descending by date
  const sorted = [...payments].sort((a, b) => new Date(b.created_at) - new Date(a.created_at));

  return (
    <Layout title="تاریخچه پرداخت‌ها" subtitle="شارژهای انجام‌شدهٔ حساب">
      {error && <div className="card banner-error">{error}</div>}

      <div className="card">
        {sorted.length === 0 ? (
          <div style={{ textAlign: 'center', padding: '40px 0', color: 'var(--ng-muted)' }}>
            <div style={{ fontSize: 40, marginBottom: 16, opacity: 0.5 }}>💳</div>
            هیچ پرداختی یافت نشد.
          </div>
        ) : (
          <table className="table" style={{ width: '100%', textAlign: 'right' }}>
            <thead>
              <tr>
                <th style={{ padding: '12px', borderBottom: '1px solid var(--ng-border)' }}>شناسه تراکنش</th>
                <th style={{ padding: '12px', borderBottom: '1px solid var(--ng-border)' }}>مبلغ</th>
                <th style={{ padding: '12px', borderBottom: '1px solid var(--ng-border)' }}>وضعیت</th>
                <th style={{ padding: '12px', borderBottom: '1px solid var(--ng-border)' }}>تاریخ</th>
              </tr>
            </thead>
            <tbody>
              {sorted.map(p => (
                <tr key={p.id}>
                  <td style={{ padding: '12px', borderBottom: '1px solid var(--ng-border)', fontFamily: 'monospace' }}>{p.id}</td>
                  <td style={{ padding: '12px', borderBottom: '1px solid var(--ng-border)', fontWeight: 700 }}>
                    {usd(p.amount)}
                  </td>
                  <td style={{ padding: '12px', borderBottom: '1px solid var(--ng-border)' }}>
                    {/* --ng-ok-soft and --ng-error-soft are not tokens this
                        theme defines, so both badges used to render with no
                        background — a failed payment looked like a successful
                        one in everything but the word. */}
                    <span className={'badge ' + (p.status === 'success' ? 'badge-pass' : 'badge-fail')} style={{ fontSize: 11 }}>
                      {p.status === 'success' ? 'موفق' : 'ناموفق'}
                    </span>
                  </td>
                  <td style={{ padding: '12px', borderBottom: '1px solid var(--ng-border)' }} dir="ltr">
                    {new Date(p.created_at).toLocaleString('fa-IR')}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </Layout>
  );
}
