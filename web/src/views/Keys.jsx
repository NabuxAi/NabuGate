import Layout from '../components/Layout.jsx';
import { keys } from '../data/mock.js';

export default function Keys() {
  return (
    <Layout
      title="کلیدهای پروژه"
      subtitle="پالیسی هر کلید (allow-list + rate limit) — کنترل دسترسی و سهمیه"
      actions={<button className="btn btn-primary">+ کلید جدید</button>}
    >
      <div className="card">
        <h3 style={{ marginBottom: 12 }}>کلیدهای پروژه</h3>
        <table className="tbl">
          <thead>
            <tr>
              <th>کلید</th>
              <th style={{ width: 110 }}>پروژه</th>
              <th>دسترسی (allow)</th>
              <th style={{ width: 90 }}>نرخ/دقیقه</th>
              <th style={{ width: 80 }} />
            </tr>
          </thead>
          <tbody>
            {keys.map((k) => (
              <tr key={k.key}>
                <td>
                  <span className="mono ltr" style={{ color: 'var(--ng-heading)', fontSize: 12 }}>
                    {k.key}
                  </span>
                  {k.full && (
                    <div>
                      <span className="badge badge-info" style={{ marginTop: 4 }}>
                        دسترسی کامل
                      </span>
                    </div>
                  )}
                </td>
                <td style={{ fontSize: 12.5, color: 'var(--ng-heading)', fontWeight: 700 }}>{k.project}</td>
                <td>
                  <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                    {k.allow.map((a) => (
                      <span key={a} className={'tag ltr' + (a.includes('*') && a !== '*' ? ' tag-pass' : '')}>
                        {a}
                      </span>
                    ))}
                  </div>
                </td>
                <td style={{ fontSize: 12.5, color: 'var(--ng-slate-700)' }}>{k.rate}</td>
                <td>
                  <button className="btn btn-ghost">ویرایش</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="note">
        درخواست برای آلیاسی خارج از <span className="mono ltr">allow</span> کد <strong>۴۰۳</strong> و عبور از{' '}
        <span className="mono ltr">rate_limit</span> کد <strong>۴۲۹</strong> برمی‌گرداند. اگر همهٔ کلیدها خالی باشند
        دروازه برای جلوگیری از باز ماندن، بالا نمی‌آید.
      </div>
    </Layout>
  );
}
