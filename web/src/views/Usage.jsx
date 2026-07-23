import Layout from '../components/Layout.jsx';
import { usageByModel, pricing, totalToday, faInt, faDigits } from '../data/mock.js';

export default function Usage() {
  return (
    <Layout title="مصرف و هزینه" subtitle="مصرف تفکیک‌شده بر اساس مدل و جدول قیمت‌گذاری">
      <div className="grid grid-2-keys">
        <div className="card">
          <div className="card-head">
            <h3>مصرف بر اساس مدل</h3>
            <span className="pill ltr">GET /v1/usage</span>
          </div>
          <table className="tbl">
            <thead>
              <tr>
                <th>مدل</th>
                <th style={{ width: 80 }}>درخواست</th>
                <th style={{ width: 70 }}>توکن</th>
                <th style={{ width: 80 }}>هزینه</th>
              </tr>
            </thead>
            <tbody>
              {usageByModel.map((u) => (
                <tr key={u.model}>
                  <td className="mono ltr" style={{ fontSize: 11.5, color: 'var(--ng-heading)' }}>
                    {u.model}
                  </td>
                  <td style={{ fontSize: 12, color: 'var(--ng-slate-700)' }}>{faInt(u.requests)}</td>
                  <td style={{ fontSize: 12, color: 'var(--ng-slate-700)' }}>{faDigits(u.tokens)}</td>
                  <td>
                    {u.cost ? (
                      <span className="ltr" style={{ fontSize: 12, color: 'var(--ng-ok-text)', fontWeight: 700 }}>
                        {faDigits(u.cost)}
                      </span>
                    ) : (
                      <span style={{ fontSize: 12, color: 'var(--ng-subtle)' }}>بدون قیمت</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <div className="card">
          <h3 style={{ marginBottom: 4 }}>جدول قیمت‌گذاری</h3>
          <p className="card-sub">USD به‌ازای هر ۱M توکن (ورودی / خروجی)</p>
          <div style={{ display: 'grid', gap: 8 }}>
            {pricing.map((p) => (
              <div className="price-row" key={p.model}>
                <span className="ltr">{p.model}</span>
                <strong className="ltr">{faDigits(p.price)}</strong>
              </div>
            ))}
          </div>
          <div className="total-row">
            <span className="lbl">هزینهٔ کل امروز</span>
            <strong className="amt ltr">{faDigits(totalToday)}</strong>
          </div>
        </div>
      </div>

      <div className="demo-banner">
        اعداد این صفحه <strong>نمونه</strong> هستند. برای داده‌های زنده، این نما به{' '}
        <span className="mono ltr">GET /v1/usage</span> دروازه وصل می‌شود (مصرف و هزینه per-project/per-model).
      </div>
    </Layout>
  );
}
