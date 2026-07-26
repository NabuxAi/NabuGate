import { useEffect, useState } from 'react';

import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { faInt, faDigits } from '../data/mock.js';

/*
 * Every figure here used to come from src/data/mock.js — "۱۲٬۴۸۰ requests
 * today" on a gateway that had served nothing. These are the persisted
 * counters and the live router state.
 */
export default function Dashboard() {
  const [data, setData] = useState(null);
  const [error, setError] = useState(null);

  useEffect(() => {
    api.overview().then(setData).catch((e) => setError(e.message));
  }, []);

  const usage = data?.usage || {};
  const rows = Object.entries(usage).sort((a, b) => (b[1].requests || 0) - (a[1].requests || 0));
  const total = rows.reduce(
    (a, [, v]) => ({
      requests: a.requests + (v.requests || 0),
      tokens: a.tokens + (v.prompt_tokens || 0) + (v.completion_tokens || 0),
      cost: a.cost + (v.cost_usd || 0),
      denied: a.denied + (v.denied || 0),
    }),
    { requests: 0, tokens: 0, cost: 0, denied: 0 }
  );

  return (
    <Layout title="داشبورد" subtitle="وضعیت واقعی دروازه — شمارنده‌های ماندگار و روتر زنده">
      {error && <div className="card banner-error">{error}</div>}

      <div className="stats">
        <Stat label="کل درخواست" value={faInt(total.requests)} />
        <Stat label="کل توکن" value={faInt(total.tokens)} />
        <Stat label="هزینه" value={faDigits('$' + total.cost.toFixed(2))} ltr />
        <Stat label="پرووایدر فعال" value={faInt((data?.providers || []).length)} />
        <Stat label="ردشده" value={faInt(total.denied)} />
      </div>

      <div className="card" style={{ marginTop: 16 }}>
        <h3 style={{ marginBottom: 12 }}>مصرف به تفکیک اپ</h3>
        <table className="tbl">
          <thead>
            <tr>
              <th>اپ</th>
              <th style={{ width: 110 }}>درخواست</th>
              <th style={{ width: 120 }}>توکن</th>
              <th style={{ width: 100 }}>هزینه</th>
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 && (
              <tr>
                <td colSpan={4} style={{ color: 'var(--ng-muted)', padding: 18 }}>
                  هنوز درخواستی ثبت نشده.
                </td>
              </tr>
            )}
            {rows.map(([name, v]) => (
              <tr key={name}>
                <td style={{ fontWeight: 700, color: 'var(--ng-heading)' }}>{name}</td>
                <td className="mono">{faInt(v.requests || 0)}</td>
                <td className="mono">{faInt((v.prompt_tokens || 0) + (v.completion_tokens || 0))}</td>
                <td className="mono ltr">{faDigits('$' + (v.cost_usd || 0).toFixed(3))}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Layout>
  );
}

function Stat({ label, value, ltr }) {
  return (
    <div className="stat">
      <div className="stat-label">{label}</div>
      <div className={'stat-value' + (ltr ? ' ltr' : '')}>{value}</div>
    </div>
  );
}
