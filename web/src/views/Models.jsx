import { useEffect, useState } from 'react';

import Layout from '../components/Layout.jsx';
import * as api from '../api.js';

/*
 * The aliases and agents the gateway is actually serving, read from the router
 * rather than from a list someone kept up to date by hand.
 */
export default function Models() {
  const [data, setData] = useState(null);
  const [error, setError] = useState(null);

  useEffect(() => {
    api.overview().then(setData).catch((e) => setError(e.message));
  }, []);

  const aliases = data?.aliases || [];
  const agents = data?.agents || [];

  return (
    <Layout
      title="مدل‌ها و آلیاس‌ها"
      subtitle="آلیاس‌های پیکربندی‌شده و ساب‌اجنت‌های بارگذاری‌شده"
    >
      {error && <div className="card banner-error">{error}</div>}

      <div className="card">
        <h3 style={{ marginBottom: 12 }}>آلیاس‌ها ({aliases.length})</h3>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
          {aliases.length === 0 && <span style={{ color: 'var(--ng-muted)' }}>…</span>}
          {aliases.map((a) => (
            <span key={a.id} className="tag ltr">{a.id}</span>
          ))}
        </div>
      </div>

      <div className="card" style={{ marginTop: 16 }}>
        <h3 style={{ marginBottom: 12 }}>ساب‌اجنت‌ها ({agents.length})</h3>
        <p className="sub" style={{ marginBottom: 10 }}>
          هرکدام یک system prompt + پارامترهای پیش‌فرض روی یک آلیاس‌اند و مثل یک مدل صدا زده می‌شوند.
        </p>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
          {agents.length === 0 && <span style={{ color: 'var(--ng-muted)' }}>…</span>}
          {agents.map((a) => (
            <span key={a} className="tag ltr tag-pass">{a}</span>
          ))}
        </div>
      </div>
    </Layout>
  );
}
