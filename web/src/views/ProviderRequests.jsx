import { useEffect, useState } from 'react';

import Layout from '../components/Layout.jsx';
import VendorIcon from '../components/VendorIcon.jsx';
import * as api from '../api.js';
import { SkeletonTable } from '../components/Skeleton.jsx';
import EmptyState from '../components/EmptyState.jsx';
import { usd, useT } from '../i18n/index.jsx';

/*
 * The approval queue.
 *
 * Approving access and funding it are one decision, so they are one action
 * here: the credit box sits next to the approve button rather than sending an
 * admin to the users screen afterwards to finish the job they thought they had
 * done.
 */
const T = {
  fa: {
    title: 'درخواست‌های دسترسی',
    subtitle: 'چه کسی می‌خواهد با کلید ما به کدام سرویس وصل شود',
    pendingTitle: 'در انتظار تأیید',
    emptyTitle: 'چیزی در صف نیست',
    emptyHint: 'درخواست‌های خودکار همان لحظه تأیید می‌شوند و اینجا نمی‌آیند.',
    service: 'سرویس',
    user: 'کاربر',
    note: 'توضیح',
    decision: 'تصمیم',
    creditPh: 'اعتبار $',
    approve: 'تأیید',
    deny: 'رد',
    decidedTitle: 'تصمیم‌گرفته‌شده',
    status: 'وضعیت',
    credit: 'اعتبار',
    by: 'توسط',
    auto: 'خودکار',
    approved: 'تأیید',
    denied: 'رد',
    revoke: 'لغو دسترسی',
  },
  en: {
    title: 'Access requests',
    subtitle: 'Who wants to reach which service on our key',
    pendingTitle: 'Awaiting approval',
    emptyTitle: 'Nothing in the queue',
    emptyHint: 'Automatic requests are approved instantly and never show up here.',
    service: 'Service',
    user: 'User',
    note: 'Note',
    decision: 'Decision',
    creditPh: 'Credit $',
    approve: 'Approve',
    deny: 'Deny',
    decidedTitle: 'Decided',
    status: 'Status',
    credit: 'Credit',
    by: 'By',
    auto: 'Automatic',
    approved: 'Approved',
    denied: 'Denied',
    revoke: 'Revoke access',
  },
};

export default function ProviderRequests() {
  const t = useT(T);
  const [rows, setRows] = useState(null);
  const [error, setError] = useState(null);
  const [credit, setCredit] = useState({});
  const [busy, setBusy] = useState(null);

  const load = () =>
    api.listProviderRequests().then((d) => setRows(d.requests || [])).catch((e) => setError(e.message));

  useEffect(() => { load(); }, []);

  const decide = async (row, approve) => {
    setBusy(row.id);
    setError(null);
    try {
      await api.decideProviderRequest(row.id, approve, credit[row.id] || 0, '');
      await load();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(null);
    }
  };

  const pending = rows?.filter((r) => r.status === 'pending') || [];
  const decided = rows?.filter((r) => r.status !== 'pending') || [];

  return (
    <Layout title={t('title')} subtitle={t('subtitle')}>
      {error && <div className="card banner-error">{error}</div>}

      <div className="card">
        <div className="card-head"><h3>{t('pendingTitle')}</h3></div>
        {rows === null ? (
          <SkeletonTable rows={3} cols={4} />
        ) : pending.length === 0 ? (
          <EmptyState title={t('emptyTitle')} hint={t('emptyHint')} />
        ) : (
          <table className="tbl">
            <thead>
              <tr><th>{t('service')}</th><th>{t('user')}</th><th>{t('note')}</th><th style={{ width: 260 }}>{t('decision')}</th></tr>
            </thead>
            <tbody>
              {pending.map((r) => (
                <tr key={r.id}>
                  <td>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                      <VendorIcon vendor={{ name: r.provider, label: r.provider }} size={26} />
                      <span className="ltr">{r.provider}</span>
                    </div>
                  </td>
                  <td className="ltr">{r.owner}</td>
                  <td className="cell-wrap" style={{ color: 'var(--ng-muted)' }}>{r.note || '—'}</td>
                  <td>
                    <div style={{ display: 'flex', gap: 6, alignItems: 'center', flexWrap: 'wrap' }}>
                      <input
                        className="input ltr"
                        style={{ width: 92 }}
                        type="number"
                        min="0"
                        step="0.5"
                        placeholder={t('creditPh')}
                        value={credit[r.id] ?? ''}
                        onChange={(e) => setCredit({ ...credit, [r.id]: e.target.value })}
                      />
                      <button className="btn btn-sm" disabled={busy === r.id} onClick={() => decide(r, true)}>{t('approve')}</button>
                      <button className="btn btn-sm btn-ghost" disabled={busy === r.id} onClick={() => decide(r, false)}>{t('deny')}</button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {decided.length > 0 && (
        <div className="card">
          <div className="card-head"><h3>{t('decidedTitle')}</h3></div>
          <table className="tbl">
            <thead>
              <tr><th>{t('service')}</th><th>{t('user')}</th><th>{t('status')}</th><th>{t('credit')}</th><th>{t('by')}</th><th style={{ width: 120 }}></th></tr>
            </thead>
            <tbody>
              {decided.map((r) => (
                <tr key={r.id}>
                  <td className="ltr">{r.provider}</td>
                  <td className="ltr">{r.owner}</td>
                  <td>
                    <span className={r.status === 'approved' ? 'tag tag-ok' : 'tag tag-muted'}>
                      {r.status === 'approved' ? (r.auto ? t('auto') : t('approved')) : t('denied')}
                    </span>
                  </td>
                  <td className="ltr">{r.credit_usd ? usd(r.credit_usd) : '—'}</td>
                  <td className="ltr">{r.decided_by || '—'}</td>
                  <td>
                    {/* A decision that cannot be undone is not a decision, it is
                        a trapdoor. The endpoint always allowed this; the screen
                        shipped without the button. */}
                    {r.status === 'approved' ? (
                      <button className="btn btn-sm btn-ghost" disabled={busy === r.id} onClick={() => decide(r, false)}>
                        {t('revoke')}
                      </button>
                    ) : (
                      <button className="btn btn-sm btn-ghost" disabled={busy === r.id} onClick={() => decide(r, true)}>
                        {t('approve')}
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Layout>
  );
}
