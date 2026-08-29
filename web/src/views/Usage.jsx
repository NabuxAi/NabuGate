import { useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { faInt, faDigits, usd } from '../data/mock.js';

export default function Usage() {
  const [byProject, setByProject] = useState({});
  const [byModel, setByModel] = useState({});
  const [byProvider, setByProvider] = useState({});
  const [error, setError] = useState(null);

  const load = () =>
    api
      .usage()
      .then((r) => {
        setByProject(r.by_project || {});
        setByModel(r.by_model || {});
        setByProvider(r.by_provider || {});
      })
      .catch((e) => setError(e.message));

  useEffect(load, []);

  const rows = Object.entries(byProject).sort((a, b) => (b[1].requests || 0) - (a[1].requests || 0));
  const modelRows = Object.entries(byModel).sort((a, b) => (b[1].requests || 0) - (a[1].requests || 0));
  const provRows = Object.entries(byProvider).sort((a, b) => (b[1].requests || 0) - (a[1].requests || 0));
  
  const total = rows.reduce(
    (acc, [, v]) => ({
      requests: acc.requests + (v.requests || 0),
      prompt_tokens: acc.prompt_tokens + (v.prompt_tokens || 0),
      completion_tokens: acc.completion_tokens + (v.completion_tokens || 0),
      cost: acc.cost + (v.cost_usd || 0),
    }),
    { requests: 0, prompt_tokens: 0, completion_tokens: 0, cost: 0 }
  );

  const totalTokens = total.prompt_tokens + total.completion_tokens;

  return (
    <Layout
      title="مصرف"
      subtitle="تحلیل مصرف توکن، درخواست‌ها و هزینه‌ها در بازه‌ی انتخابی."
      actions={
        <select style={{ background: 'var(--ng-surface)', color: 'var(--ng-heading)', border: '1px solid var(--ng-border)', padding: '6px 12px', borderRadius: '6px', fontSize: 13, outline: 'none' }}>
          <option>۳۰ روز اخیر</option>
          <option>هفته اخیر</option>
          <option>امروز</option>
        </select>
      }
    >
      {error && <div className="card banner-error">{error}</div>}

      {/* 6 Stat Cards */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(250px, 1fr))', gap: 16, marginBottom: 24 }}>
        
        {/* Row 1 */}
        <div className="card" style={{ padding: '20px 24px', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <div>
            <div style={{ color: 'var(--ng-muted)', fontSize: 13, marginBottom: 8 }}>توکن خروجی</div>
            <div style={{ fontSize: 18, fontWeight: 700 }}>{faInt(total.completion_tokens)}</div>
          </div>
          <div style={{ background: 'rgba(16, 185, 129, 0.1)', color: '#10b981', padding: 12, borderRadius: 8, fontSize: 20 }}>↑</div>
        </div>

        <div className="card" style={{ padding: '20px 24px', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <div>
            <div style={{ color: 'var(--ng-muted)', fontSize: 13, marginBottom: 8 }}>توکن ورودی</div>
            <div style={{ fontSize: 18, fontWeight: 700 }}>{faInt(total.prompt_tokens)}</div>
          </div>
          <div style={{ background: 'rgba(59, 130, 246, 0.1)', color: '#3b82f6', padding: 12, borderRadius: 8, fontSize: 20 }}>↓</div>
        </div>

        <div className="card" style={{ padding: '20px 24px', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <div>
            <div style={{ color: 'var(--ng-muted)', fontSize: 13, marginBottom: 8 }}>کل توکن</div>
            <div style={{ fontSize: 18, fontWeight: 700 }}>{faInt(totalTokens)}</div>
          </div>
          <div style={{ background: 'rgba(99, 102, 241, 0.1)', color: '#6366f1', padding: 12, borderRadius: 8, fontSize: 20 }}>⊚</div>
        </div>

        {/* Row 2 */}
        <div className="card" style={{ padding: '20px 24px', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <div>
            <div style={{ color: 'var(--ng-muted)', fontSize: 13, marginBottom: 8 }}>هزینه ارائه‌دهنده</div>
            <div style={{ fontSize: 18, fontWeight: 700 }} dir="ltr">$ {faDigits(total.cost.toFixed(3))}</div>
          </div>
          <div style={{ background: 'rgba(59, 130, 246, 0.1)', color: '#3b82f6', padding: 12, borderRadius: 8, fontSize: 20 }}>$</div>
        </div>

        <div className="card" style={{ padding: '20px 24px', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <div>
            <div style={{ color: 'var(--ng-muted)', fontSize: 13, marginBottom: 8 }}>هزینه تخمینی</div>
            {/* This multiplied the dollar figure by a rate written into the
                source as "dummy exchange rate for UI". A made-up number
                rendered in تومان beside real ones reads as a real one. */}
            <div style={{ fontSize: 18, fontWeight: 700 }} dir="ltr">{usd(total.cost)}</div>
          </div>
          <div style={{ background: 'rgba(245, 158, 11, 0.1)', color: '#f59e0b', padding: 12, borderRadius: 8, fontSize: 20 }}>💳</div>
        </div>

        <div className="card" style={{ padding: '20px 24px', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <div>
            <div style={{ color: 'var(--ng-muted)', fontSize: 13, marginBottom: 8 }}>درخواست‌ها</div>
            <div style={{ fontSize: 18, fontWeight: 700 }}>{faInt(total.requests)}</div>
          </div>
          <div style={{ background: 'rgba(99, 102, 241, 0.1)', color: '#6366f1', padding: 12, borderRadius: 8, fontSize: 20 }}>⚡</div>
        </div>

      </div>

      {/* Model & Provider Stats */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(300px, 1fr))', gap: 24, marginBottom: 24 }}>
        <div className="card" style={{ padding: 0, display: 'flex', flexDirection: 'column' }}>
          <div style={{ padding: 24, borderBottom: '1px solid var(--ng-border)' }}>
            <h3 style={{ fontSize: 14, margin: 0 }}>مصرف به تفکیک ارائه دهنده</h3>
          </div>
          {provRows.length === 0 ? (
            <div style={{ flex: 1, padding: 32, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center' }}>
              <div style={{ fontSize: 32, marginBottom: 12, color: 'var(--ng-border)' }}>📭</div>
              <div style={{ fontWeight: 700, fontSize: 14, marginBottom: 8 }}>موردی یافت نشد</div>
            </div>
          ) : (
            <table className="tbl" style={{ border: 'none', margin: 0 }}>
              <thead><tr><th>پروایدر</th><th>توکن کل</th><th>هزینه</th></tr></thead>
              <tbody>
                {provRows.map(([name, v]) => (
                  <tr key={name}>
                    <td style={{ fontWeight: 700, color: 'var(--ng-heading)' }} dir="ltr">{name}</td>
                    <td className="mono">{faInt((v.prompt_tokens || 0) + (v.completion_tokens || 0))}</td>
                    <td className="mono ltr">{faDigits('$' + (v.cost_usd || 0).toFixed(4))}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>

        <div className="card" style={{ padding: 0, display: 'flex', flexDirection: 'column' }}>
          <div style={{ padding: 24, borderBottom: '1px solid var(--ng-border)' }}>
            <h3 style={{ fontSize: 14, margin: 0 }}>مصرف به تفکیک مدل</h3>
          </div>
          {modelRows.length === 0 ? (
            <div style={{ flex: 1, padding: 32, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center' }}>
              <div style={{ fontSize: 32, marginBottom: 12, color: 'var(--ng-border)' }}>📭</div>
              <div style={{ fontWeight: 700, fontSize: 14, marginBottom: 8 }}>موردی یافت نشد</div>
            </div>
          ) : (
            <table className="tbl" style={{ border: 'none', margin: 0 }}>
              <thead><tr><th>مدل</th><th>توکن کل</th><th>هزینه</th></tr></thead>
              <tbody>
                {modelRows.map(([name, v]) => (
                  <tr key={name}>
                    <td style={{ fontWeight: 700, color: 'var(--ng-heading)' }} dir="ltr">{name}</td>
                    <td className="mono">{faInt((v.prompt_tokens || 0) + (v.completion_tokens || 0))}</td>
                    <td className="mono ltr">{faDigits('$' + (v.cost_usd || 0).toFixed(4))}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>

      {/* API Key Usage Table */}
      <div className="card" style={{ padding: 0 }}>
        <div style={{ padding: 24, borderBottom: '1px solid var(--ng-border)' }}>
          <h3 style={{ fontSize: 14, margin: 0 }}>مصرف به تفکیک کلید API</h3>
        </div>
        <table className="tbl" style={{ border: 'none' }}>
          <thead>
            <tr>
              <th>کلید</th>
              <th>توکن خروجی</th>
              <th>توکن ورودی</th>
              <th>درخواست‌ها</th>
              <th>هزینه ارائه‌دهنده</th>
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 && (
              <tr>
                <td colSpan={5} style={{ padding: '60px 24px', textAlign: 'center' }}>
                  <div style={{ fontSize: 32, marginBottom: 12, color: 'var(--ng-border)' }}>📭</div>
                  <div style={{ fontWeight: 700, fontSize: 14, marginBottom: 8 }}>مصرفی برای کلیدها ثبت نشده است</div>
                  <p style={{ color: 'var(--ng-muted)', fontSize: 12 }}>پس از اولین استفاده از کلیدها، آمار مصرف اینجا نمایش داده می‌شود.</p>
                </td>
              </tr>
            )}
            {rows.map(([name, v]) => (
              <tr key={name}>
                <td style={{ fontWeight: 700, color: 'var(--ng-heading)' }}>{name}</td>
                <td className="mono">{faInt(v.completion_tokens || 0)}</td>
                <td className="mono">{faInt(v.prompt_tokens || 0)}</td>
                <td className="mono">{faInt(v.requests || 0)}</td>
                <td className="mono ltr">{faDigits('$' + (v.cost_usd || 0).toFixed(4))}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Layout>
  );
}
