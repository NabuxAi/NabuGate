import { useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { fmtDate, fmtInt, usd, useT } from '../i18n/index.jsx';
import { SkeletonTable } from '../components/Skeleton.jsx';
import EmptyState from '../components/EmptyState.jsx';
import Icon from '../components/Icon.jsx';

const T = {
  fa: {
    title: 'درخواست‌های اخیر',
    subtitle: 'آخرین تماس‌های کلیدهای شما با دروازه.',
    emptyTitle: 'هنوز درخواستی ثبت نشده',
    emptyHint: 'به‌محض اولین تماس با کلیدتان، این فهرست هر ۱۰ ثانیه تازه می‌شود و دلیل هر رد شدن را هم می‌گوید.',
    time: 'زمان',
    key: 'کلید',
    model: 'مدل',
    provider: 'پروایدر',
    tokens: 'توکن',
    cost: 'هزینه',
    result: 'نتیجه',
    denied: 'رد شد',
    ok: 'موفق',
    volatile: 'این فهرست در حافظه نگه‌داری می‌شود و با هر ری‌استارت دروازه خالی می‌شود؛ پس فهرستِ خالی لزوماً یعنی «ترافیکی نبوده» نیست. آمارِ تجمعی در «حساب و مصرف» است و ماندگار می‌ماند.',
  },
  en: {
    title: 'Recent requests',
    subtitle: 'The latest calls your keys made to the gateway.',
    emptyTitle: 'No requests yet',
    emptyHint: 'As soon as your key makes its first call, this list refreshes every 10 seconds and shows the reason for every refusal.',
    time: 'Time',
    key: 'Key',
    model: 'Model',
    provider: 'Provider',
    tokens: 'Tokens',
    cost: 'Cost',
    result: 'Result',
    denied: 'Denied',
    ok: 'Success',
    volatile: 'This list is kept in memory and is emptied whenever the gateway restarts, so an empty list doesn’t necessarily mean there was no traffic. Cumulative stats live under “Balance & usage” and persist.',
  },
};

export default function Requests() {
  const t = useT(T);
  const [data, setData] = useState(null);
  const [error, setError] = useState(null);

  const load = () => api.recentRequests().then(setData).catch((e) => setError(e.message));

  useEffect(() => {
    load();
    // The log is a ring the gateway writes to as calls arrive, so a screen left
    // open goes stale within seconds of anything happening.
    const timer = setInterval(load, 10000);
    return () => clearInterval(timer);
  }, []);

  const rows = data?.requests || [];

  return (
    <Layout title={t('title')} subtitle={t('subtitle')}>
      {error && <div className="card banner-error">{error}</div>}

      <div className="card">
        {data === null ? (
          <SkeletonTable rows={6} cols={7} />
        ) : rows.length === 0 ? (
          <EmptyState icon={<Icon name="activity" size={22} />} title={t('emptyTitle')} hint={t('emptyHint')} />
        ) : (
          <table className="tbl" style={{ margin: 0 }}>
            <thead>
              <tr>
                <th>{t('time')}</th>
                <th>{t('key')}</th>
                <th>{t('model')}</th>
                <th>{t('provider')}</th>
                <th>{t('tokens')}</th>
                <th>{t('cost')}</th>
                <th>{t('result')}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((e, i) => (
                <tr key={i}>
                  <td dir="ltr" style={{ fontSize: 12, color: 'var(--ng-muted)' }}>
                    {fmtDate(e.at, 'time')}
                  </td>
                  <td className="mono" dir="ltr">{e.project}</td>
                  <td className="mono" dir="ltr">{e.model || '—'}</td>
                  <td className="mono" dir="ltr">{e.provider || '—'}</td>
                  <td>{e.denied ? '—' : fmtInt(e.tokens)}</td>
                  <td className="ltr">{e.denied ? '—' : usd(e.cost_usd)}</td>
                  <td>
                    {e.denied ? (
                      /* The reason, not just the fact. "Refused" alone sends
                         somebody to read the gateway's logs to learn what this
                         row already knows. */
                      <span className="badge badge-fail" style={{ fontSize: 11 }} title={e.reason}>
                        {e.reason || t('denied')}
                      </span>
                    ) : (
                      <span className="badge badge-pass" style={{ fontSize: 11 }}>{t('ok')}</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {data?.volatile && (
        <p className="muted" style={{ marginTop: 12, fontSize: 12, lineHeight: 1.7 }}>{t('volatile')}</p>
      )}
    </Layout>
  );
}
