import { useEffect, useState } from 'react';

import Layout from '../components/Layout.jsx';
import * as api from '../api.js';

/*
 * Sub-agents (read-only). Agents are declared in config/YAML and baked into the
 * image, so the console lists them rather than editing them — to change one, edit
 * its YAML in NabuxAi/NabuGate and redeploy.
 */
export default function Agents() {
  const [agents, setAgents] = useState([]);
  const [error, setError] = useState(null);

  useEffect(() => {
    api
      .listAgents()
      .then((r) => setAgents(r.agents || []))
      .catch((e) => setError(e.message));
  }, []);

  return (
    <Layout
      title="ساب‌اجنت‌ها"
      subtitle="دستیارهای نام‌دار: system prompt + پارامترهای پیش‌فرض روی یک آلیاس"
    >
      {error && <div className="banner-error">{error}</div>}
      {!error && agents.length === 0 && (
        <div className="card">
          <p className="card-sub">
            هنوز ساب‌اجنتی تعریف نشده است. ساب‌اجنت‌ها در پوشهٔ <span className="mono">agents/</span> مخزنِ
            NabuGate (YAML) تعریف و در ایمیج baked می‌شوند؛ بعد از افزودن، دوباره دیپلوی کن.
          </p>
        </div>
      )}
      <div className="grid grid-3">
        {agents.map((a) => (
          <div key={a.name} className="card">
            <div className="card-head">
              <span className="mono">{a.name}</span>
            </div>
            {a.description && <p className="card-sub">{a.description}</p>}
            <p className="card-sub">
              آلیاس: <span className="mono">{a.model || '—'}</span>
            </p>
          </div>
        ))}
      </div>
    </Layout>
  );
}
