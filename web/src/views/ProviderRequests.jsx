import { useEffect, useState } from 'react';

import Layout from '../components/Layout.jsx';
import VendorIcon from '../components/VendorIcon.jsx';
import * as api from '../api.js';
import { SkeletonTable } from '../components/Skeleton.jsx';
import EmptyState from '../components/EmptyState.jsx';
import { usd } from '../data/mock.js';

/*
 * The approval queue.
 *
 * Approving access and funding it are one decision, so they are one action
 * here: the credit box sits next to the approve button rather than sending an
 * admin to the users screen afterwards to finish the job they thought they had
 * done.
 */
export default function ProviderRequests() {
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
    <Layout title="درخواست‌های دسترسی" subtitle="چه کسی می‌خواهد با کلید ما به کدام سرویس وصل شود">
      {error && <div className="card banner-error">{error}</div>}

      <div className="card">
        <div className="card-head"><h3>در انتظار تأیید</h3></div>
        {rows === null ? (
          <SkeletonTable rows={3} cols={4} />
        ) : pending.length === 0 ? (
          <EmptyState title="چیزی در صف نیست" hint="درخواست‌های خودکار همان لحظه تأیید می‌شوند و اینجا نمی‌آیند." />
        ) : (
          <table className="tbl">
            <thead>
              <tr><th>سرویس</th><th>کاربر</th><th>توضیح</th><th style={{ width: 260 }}>تصمیم</th></tr>
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
                  <td style={{ color: 'var(--ng-muted)' }}>{r.note || '—'}</td>
                  <td>
                    <div style={{ display: 'flex', gap: 6, alignItems: 'center', flexWrap: 'wrap' }}>
                      <input
                        className="input ltr"
                        style={{ width: 92 }}
                        type="number"
                        min="0"
                        step="0.5"
                        placeholder="اعتبار $"
                        value={credit[r.id] ?? ''}
                        onChange={(e) => setCredit({ ...credit, [r.id]: e.target.value })}
                      />
                      <button className="btn btn-sm" disabled={busy === r.id} onClick={() => decide(r, true)}>تأیید</button>
                      <button className="btn btn-sm btn-ghost" disabled={busy === r.id} onClick={() => decide(r, false)}>رد</button>
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
          <div className="card-head"><h3>تصمیم‌گرفته‌شده</h3></div>
          <table className="tbl">
            <thead>
              <tr><th>سرویس</th><th>کاربر</th><th>وضعیت</th><th>اعتبار</th><th>توسط</th><th style={{ width: 120 }}></th></tr>
            </thead>
            <tbody>
              {decided.map((r) => (
                <tr key={r.id}>
                  <td className="ltr">{r.provider}</td>
                  <td className="ltr">{r.owner}</td>
                  <td>
                    <span className={r.status === 'approved' ? 'tag tag-ok' : 'tag tag-muted'}>
                      {r.status === 'approved' ? (r.auto ? 'خودکار' : 'تأیید') : 'رد'}
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
                        لغو دسترسی
                      </button>
                    ) : (
                      <button className="btn btn-sm btn-ghost" disabled={busy === r.id} onClick={() => decide(r, true)}>
                        تأیید
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
