import { useEffect, useState } from 'react';

import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { faInt, faDigits } from '../data/mock.js';

/*
 * Real per-project usage.
 *
 * This view used to render from src/data/mock.js — invented numbers that looked
 * like a working gateway. These come from the persisted counters, so they also
 * survive a redeploy; the in-memory tracker behind /v1/usage resets to zero
 * every time the container restarts.
 */
export default function Usage() {
  const [byProject, setByProject] = useState({});
  const [error, setError] = useState(null);

  const load = () =>
    api
      .usage()
      .then((r) => setByProject(r.by_project || {}))
      .catch((e) => setError(e.message));

  useEffect(load, []);

  const rows = Object.entries(byProject).sort((a, b) => (b[1].requests || 0) - (a[1].requests || 0));
  const total = rows.reduce(
    (acc, [, v]) => ({
      requests: acc.requests + (v.requests || 0),
      tokens: acc.tokens + (v.prompt_tokens || 0) + (v.completion_tokens || 0),
      cost: acc.cost + (v.cost_usd || 0),
      denied: acc.denied + (v.denied || 0),
    }),
    { requests: 0, tokens: 0, cost: 0, denied: 0 }
  );

  return (
    <Layout
      title="مصرف و هزینه"
      subtitle="شمارنده‌های واقعی، به تفکیک اپ — ماندگار بین ری‌دیپلوی‌ها"
      actions={
        <button
          className="btn"
          onClick={async () => {
            if (!confirm('همهٔ شمارنده‌ها صفر شوند؟')) return;
            await api.resetUsage();
            load();
          }}
        >
          صفر کردن
        </button>
      }
    >
      {error && <div className="card banner-error">{error}</div>}

      <div className="stats">
        <div className="stat"><div className="stat-label">کل درخواست</div><div className="stat-value">{faInt(total.requests)}</div></div>
        <div className="stat"><div className="stat-label">کل توکن</div><div className="stat-value">{faInt(total.tokens)}</div></div>
        <div className="stat"><div className="stat-label">هزینه</div><div className="stat-value ltr">{faDigits('$' + total.cost.toFixed(2))}</div></div>
        <div className="stat"><div className="stat-label">ردشده</div><div className="stat-value">{faInt(total.denied)}</div></div>
      </div>

      <div className="card" style={{ marginTop: 16 }}>
        <table className="tbl">
          <thead>
            <tr>
              <th>اپ</th>
              <th style={{ width: 110 }}>درخواست</th>
              <th style={{ width: 120 }}>توکن ورودی</th>
              <th style={{ width: 120 }}>توکن خروجی</th>
              <th style={{ width: 100 }}>هزینه</th>
              <th style={{ width: 90 }}>ردشده</th>
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 && (
              <tr>
                <td colSpan={6} style={{ color: 'var(--ng-muted)', padding: 18 }}>
                  هنوز درخواستی ثبت نشده.
                </td>
              </tr>
            )}
            {rows.map(([name, v]) => (
              <tr key={name}>
                <td style={{ fontWeight: 700, color: 'var(--ng-heading)' }}>{name}</td>
                <td className="mono">{faInt(v.requests || 0)}</td>
                <td className="mono">{faInt(v.prompt_tokens || 0)}</td>
                <td className="mono">{faInt(v.completion_tokens || 0)}</td>
                <td className="mono ltr">{faDigits('$' + (v.cost_usd || 0).toFixed(3))}</td>
                <td className="mono">{faInt(v.denied || 0)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Layout>
  );
}
