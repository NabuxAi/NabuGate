import Layout from '../components/Layout.jsx';
import { providers } from '../data/mock.js';

const tagClass = (kind) =>
  kind === 'default' ? 'badge badge-info' : kind === 'pass' ? 'badge badge-pass' : 'badge badge-muted';

export default function Providers() {
  const active = providers.filter((p) => p.on).length;
  return (
    <Layout
      title="پرووایدرها"
      subtitle="پرووایدرهای بالادست از config.yaml — سکرت‌ها فقط از env خوانده می‌شوند"
      actions={<span className="pill">{active} فعال · {providers.length - active} خاموش</span>}
    >
      <div className="card">
        <table className="tbl">
          <thead>
            <tr>
              <th style={{ width: 160 }}>پرووایدر</th>
              <th style={{ width: 130 }}>نوع</th>
              <th>base_url</th>
              <th style={{ width: 90 }}>وضعیت</th>
            </tr>
          </thead>
          <tbody>
            {providers.map((p) => (
              <tr key={p.name}>
                <td>
                  <strong className="alias">{p.name}</strong>
                </td>
                <td>
                  <span className={tagClass(p.tagKind)}>{p.tag}</span>
                </td>
                <td>
                  <span className={/[a-zA-Z]/.test(p.url) ? 'mono ltr' : ''} style={{ fontSize: 12, color: 'var(--ng-slate-700)' }}>
                    {p.url}
                  </span>
                </td>
                <td>
                  <span className={'badge ' + (p.on ? 'badge-ok' : 'badge-warn')}>{p.on ? 'فعال' : 'خاموش'}</span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="note">
        پرووایدری که env کلیدش خالی باشد خودکار رد می‌شود تا دروازه با زیرمجموعه‌ای از پرووایدرها هم بالا بیاید.
        پرووایدرهای <strong>passthrough</strong> کل کاتالوگ خود را زنده روی{' '}
        <span className="mono ltr">GET /v1/models</span> عرضه می‌کنند.
      </div>
    </Layout>
  );
}
