import { useEffect, useState } from 'react';

import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { Skeleton } from '../components/Skeleton.jsx';

/*
 * Keys declared in config.yaml.
 *
 * Read-only, and names only: their secrets live in the deployment's environment
 * and a console that displayed keys would be a console worth stealing. Tokens
 * you can create and revoke live under "توکن هر اپ".
 *
 * This view previously rendered a fixed list from a mock file, with a
 * "+ new key" button wired to nothing — so the one thing it looked like it
 * could do was the one thing it could not.
 */
export default function Keys() {
  const [projects, setProjects] = useState(null);
  const [error, setError] = useState(null);

  useEffect(() => {
    api.overview().then((d) => setProjects(d.config_keys || [])).catch((e) => setError(e.message));
  }, []);

  return (
    <Layout
      title="کلیدهای پیکربندی"
      subtitle="کلیدهای تعریف‌شده در config.yaml — فقط خواندنی، از env دیپلوی می‌آیند"
    >
      {error && <div className="card banner-error">{error}</div>}

      <div className="card">
        <p className="sub" style={{ marginBottom: 12 }}>
          برای ساخت یا لغو کلید، به «توکن هر اپ» برو. آن‌ها در دروازه ذخیره می‌شوند
          و از همین کنسول قابل مدیریت‌اند؛ این‌ها با دیپلوی می‌آیند.
        </p>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
          {projects === null && <><Skeleton w={90} h={22} round /><Skeleton w={120} h={22} round /><Skeleton w={70} h={22} round /></>}
          {projects?.length === 0 && (
            <span style={{ color: 'var(--ng-muted)' }}>هیچ کلید پروژه‌ای در کانفیگ تعریف نشده.</span>
          )}
          {projects?.map((p) => (
            <span key={p} className="tag ltr">{p}</span>
          ))}
        </div>
      </div>
    </Layout>
  );
}
