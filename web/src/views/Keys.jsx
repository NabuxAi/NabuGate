import { useEffect, useState } from 'react';

import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { Skeleton } from '../components/Skeleton.jsx';
import { useT } from '../i18n/index.jsx';

const T = {
  fa: {
    title: 'کلیدهای پیکربندی',
    subtitle: 'کلیدهای تعریف‌شده در config.yaml — فقط خواندنی، از env دیپلوی می‌آیند',
    intro: 'برای ساخت یا لغو کلید، به «کلیدهای API» برو. آن‌ها در دروازه ذخیره می‌شوند و از همین کنسول قابل مدیریت‌اند؛ این‌ها با دیپلوی می‌آیند.',
    none: 'هیچ کلید پروژه‌ای در کانفیگ تعریف نشده.',
  },
  en: {
    title: 'Config keys',
    subtitle: 'Keys defined in config.yaml — read-only, supplied by the deployment’s env',
    intro: 'To create or revoke a key, go to “API keys”. Those are stored in the gateway and managed from this console; these ship with the deployment.',
    none: 'No project keys are defined in the config.',
  },
};

/*
 * Keys declared in config.yaml.
 *
 * Read-only, and names only: their secrets live in the deployment's environment
 * and a console that displayed keys would be a console worth stealing. Tokens
 * you can create and revoke live under "API keys".
 *
 * This view previously rendered a fixed list from a mock file, with a
 * "+ new key" button wired to nothing — so the one thing it looked like it
 * could do was the one thing it could not.
 */
export default function Keys() {
  const t = useT(T);
  const [projects, setProjects] = useState(null);
  const [error, setError] = useState(null);

  useEffect(() => {
    api.overview().then((d) => setProjects(d.config_keys || [])).catch((e) => setError(e.message));
  }, []);

  return (
    <Layout title={t('title')} subtitle={t('subtitle')}>
      {error && <div className="card banner-error">{error}</div>}

      <div className="card">
        <p className="sub" style={{ marginBottom: 12 }}>{t('intro')}</p>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
          {projects === null && <><Skeleton w={90} h={22} round /><Skeleton w={120} h={22} round /><Skeleton w={70} h={22} round /></>}
          {projects?.length === 0 && (
            <span style={{ color: 'var(--ng-muted)' }}>{t('none')}</span>
          )}
          {projects?.map((p) => (
            <span key={p} className="tag ltr">{p}</span>
          ))}
        </div>
      </div>
    </Layout>
  );
}
