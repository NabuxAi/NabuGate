import { useEffect, useState } from 'react';

import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { Skeleton } from '../components/Skeleton.jsx';

/*
 * Providers that actually came up.
 *
 * This listed a fixed set from a mock file, including several that had no key
 * and were skipped at startup — so the console showed a gateway with twelve
 * live providers when it had two. A provider is listed here only if its adapter
 * exists, which is the same test the router applies when routing.
 */
export default function Providers() {
  const [providers, setProviders] = useState(null);
  const [error, setError] = useState(null);

  useEffect(() => {
    api.overview().then((d) => setProviders(d.providers || [])).catch((e) => setError(e.message));
  }, []);

  return (
    <Layout title="پرووایدرها" subtitle="فقط آن‌هایی که واقعاً بالا آمده‌اند — بی‌کلید‌ها در استارتاپ رد می‌شوند">
      {error && <div className="card banner-error">{error}</div>}
      <div className="card">
        <table className="tbl">
          <thead>
            <tr>
              <th>پرووایدر</th>
              <th style={{ width: 160 }}>مسیر مستقیم</th>
            </tr>
          </thead>
          <tbody>
            {providers === null && (
              <tr><td colSpan={2} style={{ padding: 12 }}><div className="sk-stack"><Skeleton h={14} /><Skeleton h={14} w="80%" /><Skeleton h={14} w="60%" /></div></td></tr>
            )}
            {providers?.length === 0 && (
              <tr>
                <td colSpan={2} style={{ color: 'var(--ng-muted)', padding: 18 }}>
                  هیچ پرووایدری بالا نیامده. یعنی هیچ کلیدی ست نشده.
                </td>
              </tr>
            )}
            {providers?.map((p) => (
              <tr key={p.name}>
                <td style={{ fontWeight: 700, color: 'var(--ng-heading)' }}>{p.name}</td>
                <td>
                  {p.passthrough ? (
                    <span className="tag ltr tag-pass">{p.name}/*</span>
                  ) : (
                    <span style={{ color: 'var(--ng-muted)', fontSize: 12 }}>—</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Layout>
  );
}
