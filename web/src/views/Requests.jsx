import { useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { faInt, usd } from '../data/mock.js';
import { SkeletonTable } from '../components/Skeleton.jsx';
import EmptyState from '../components/EmptyState.jsx';

export default function Requests() {
  const [data, setData] = useState(null);
  const [error, setError] = useState(null);

  const load = () => api.recentRequests().then(setData).catch((e) => setError(e.message));

  useEffect(() => {
    load();
    // The log is a ring the gateway writes to as calls arrive, so a screen left
    // open goes stale within seconds of anything happening.
    const t = setInterval(load, 10000);
    return () => clearInterval(t);
  }, []);

  const rows = data?.requests || [];

  return (
    <Layout title="درخواست‌های اخیر" subtitle="آخرین تماس‌های کلیدهای شما با دروازه.">
      {error && <div className="card banner-error">{error}</div>}

      <div className="card">
        {data === null ? (
          <SkeletonTable rows={6} cols={7} />
        ) : rows.length === 0 ? (
          <EmptyState icon="➤" title="هنوز درخواستی ثبت نشده" hint="به‌محض اولین تماس با کلیدتان، این فهرست هر ۱۰ ثانیه تازه می‌شود و دلیل هر رد شدن را هم می‌گوید." />
        ) : (
          <table className="tbl" style={{ margin: 0 }}>
            <thead>
              <tr>
                <th>زمان</th>
                <th>کلید</th>
                <th>مدل</th>
                <th>پروایدر</th>
                <th>توکن</th>
                <th>هزینه</th>
                <th>نتیجه</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((e, i) => (
                <tr key={i}>
                  <td dir="ltr" style={{ fontSize: 12, color: 'var(--ng-muted)' }}>
                    {new Date(e.at).toLocaleTimeString('fa-IR')}
                  </td>
                  <td className="mono" dir="ltr">{e.project}</td>
                  <td className="mono" dir="ltr">{e.model || '—'}</td>
                  <td className="mono" dir="ltr">{e.provider || '—'}</td>
                  <td>{e.denied ? '—' : faInt(e.tokens)}</td>
                  <td className="ltr">{e.denied ? '—' : usd(e.cost_usd)}</td>
                  <td>
                    {e.denied ? (
                      /* The reason, not just the fact. "Refused" alone sends
                         somebody to read the gateway's logs to learn what this
                         row already knows. */
                      <span className="badge badge-fail" style={{ fontSize: 11 }} title={e.reason}>
                        {e.reason || 'رد شد'}
                      </span>
                    ) : (
                      <span className="badge badge-pass" style={{ fontSize: 11 }}>موفق</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {data?.volatile && (
        <p className="muted" style={{ marginTop: 12, fontSize: 12, lineHeight: 1.7 }}>
          این فهرست در حافظه نگه‌داری می‌شود و با هر ری‌استارت دروازه خالی می‌شود؛
          پس فهرستِ خالی لزوماً یعنی «ترافیکی نبوده» نیست. آمارِ تجمعی در
          «حساب و مصرف» است و ماندگار می‌ماند.
        </p>
      )}
    </Layout>
  );
}
