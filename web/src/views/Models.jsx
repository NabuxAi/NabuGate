import Layout from '../components/Layout.jsx';
import Chain from '../components/Chain.jsx';
import { chatAliases, mediaAliases, faInt } from '../data/mock.js';

function MediaCard({ title, items }) {
  return (
    <div className="card">
      <h3 style={{ marginBottom: 10, fontSize: 15 }}>{title}</h3>
      <div className="rows">
        {items.map((m, i) => (
          <div key={m.alias}>
            {i > 0 && <div className="hr" style={{ marginBottom: 9 }} />}
            <strong className="alias ltr">{m.alias}</strong>
            <div style={{ marginTop: 5 }}>
              <Chain primary={m.chain[0]} fallbacks={m.chain.slice(1)} />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

export default function Models() {
  return (
    <Layout
      title="مدل‌ها و آلیاس‌ها"
      subtitle="جدول روتینگ آلیاس → اولیه و زنجیرهٔ فالبک (از config.yaml)"
      actions={<button className="btn btn-primary">+ آلیاس جدید</button>}
    >
      <div className="card">
        <div className="card-head" style={{ marginBottom: 10 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <h3 style={{ fontSize: 15 }}>چت</h3>
            <span className="pill">{faInt(chatAliases.length)} آلیاس</span>
          </div>
        </div>
        <table className="tbl">
          <thead>
            <tr>
              <th style={{ width: 130 }}>آلیاس</th>
              <th>اولیه → فالبک</th>
              <th style={{ width: 80 }}>وضعیت</th>
            </tr>
          </thead>
          <tbody>
            {chatAliases.map((a) => (
              <tr key={a.alias}>
                <td>
                  <strong className="alias ltr">{a.alias}</strong>
                </td>
                <td>
                  <Chain primary={a.primary} fallbacks={a.fallbacks} note={a.note} />
                </td>
                <td>
                  <span className={'badge ' + (a.on ? 'badge-ok' : 'badge-warn')}>{a.on ? 'فعال' : 'خاموش'}</span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="grid grid-3">
        <MediaCard title="تصویر" items={mediaAliases.image} />
        <MediaCard title="صدا" items={mediaAliases.audio} />
        <MediaCard title="امبدینگ" items={mediaAliases.embed} />
      </div>

      <div className="note">
        پرووایدرهای <strong>passthrough</strong> (parspack، avalai، gapgpt) کل کاتالوگ خود را زنده روی{' '}
        <span className="mono ltr">GET /v1/models</span> عرضه می‌کنند؛ بدون آلیاس هم با{' '}
        <span className="mono ltr">provider/model</span> قابل فراخوانی‌اند.
      </div>
    </Layout>
  );
}
