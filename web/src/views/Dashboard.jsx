import { useEffect, useState } from 'react';

import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { faInt, faDigits } from '../data/mock.js';

export default function Dashboard() {
  const [data, setData] = useState(null);
  const [tokens, setTokens] = useState([]);
  const [error, setError] = useState(null);
  const [payModal, setPayModal] = useState(false);
  const [amountUsd, setAmountUsd] = useState(10);
  const [creating, setCreating] = useState(false);
  const [minted, setMinted] = useState(null);

  const load = () => {
    api.overview().then(setData).catch((e) => setError(e.message));
    api.listTokens().then((r) => setTokens(r.tokens || [])).catch(() => {});
  };

  useEffect(load, []);

  const usage = data?.usage || {};
  const rows = Object.entries(usage).sort((a, b) => (b[1].requests || 0) - (a[1].requests || 0));
  const total = rows.reduce(
    (a, [, v]) => ({
      requests: a.requests + (v.requests || 0),
      tokens: a.tokens + (v.prompt_tokens || 0) + (v.completion_tokens || 0),
      cost: a.cost + (v.cost_usd || 0),
    }),
    { requests: 0, tokens: 0, cost: 0 }
  );

  // Fake balance logic for UI simulation based on spend
  const fakeBalance = Math.max(0, 50 - total.cost).toFixed(2);

  const startPayment = (method) => {
    alert(`انتقال به درگاه NabuPay (${method})...`);
    setPayModal(false);
  };

  const createToken = async (e) => {
    e.preventDefault();
    setCreating(true);
    setError(null);
    setMinted(null);
    try {
      // Create a default app token for this user
      const name = "app-" + Math.random().toString(36).substring(2, 8);
      const allow = ['nabu-*'];
      const res = await api.createToken(name, allow, 0, [], [], false);
      setMinted({ name, secret: res.secret });
      load();
    } catch (err) {
      setError(err.message);
    } finally {
      setCreating(false);
    }
  };

  return (
    <Layout title="داشبورد کاربری" subtitle="مدیریت توکن‌ها، موجودی حساب و اتصال به Claude Code">
      {error && <div className="card banner-error">{error}</div>}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', gap: '20px', marginBottom: '24px' }}>
        
        {/* Balance Card */}
        <div style={{ background: 'linear-gradient(135deg, #0f172a 0%, #1e293b 100%)', borderRadius: '16px', padding: '24px', color: '#fff', position: 'relative', overflow: 'hidden' }}>
          <div style={{ position: 'absolute', right: '-20px', top: '-20px', fontSize: '120px', opacity: '0.05' }}>$</div>
          <div style={{ fontSize: '13px', color: '#94a3b8', marginBottom: '8px' }}>موجودی حساب شما</div>
          <div style={{ fontSize: '36px', fontWeight: '800', marginBottom: '24px' }} dir="ltr">
            ${faDigits(fakeBalance)}
          </div>
          <div style={{ display: 'flex', gap: '12px' }}>
            <button onClick={() => setPayModal(true)} style={{ background: '#3b82f6', border: 'none', color: '#fff', padding: '8px 16px', borderRadius: '8px', fontSize: '13px', fontWeight: '700', cursor: 'pointer', flex: 1 }}>افزایش موجودی</button>
          </div>
        </div>

        {/* Stats Card */}
        <div style={{ background: '#fff', borderRadius: '16px', padding: '24px', border: '1px solid #e2e8f0', display: 'flex', flexDirection: 'column', justifyContent: 'center' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '16px' }}>
            <span style={{ fontSize: '13px', color: '#64748b' }}>کل درخواست‌ها</span>
            <strong style={{ fontSize: '16px', color: '#0f172a' }}>{faInt(total.requests)}</strong>
          </div>
          <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '16px' }}>
            <span style={{ fontSize: '13px', color: '#64748b' }}>توکن مصرفی</span>
            <strong style={{ fontSize: '16px', color: '#0f172a' }}>{faInt(total.tokens)}</strong>
          </div>
          <div style={{ display: 'flex', justifyContent: 'space-between' }}>
            <span style={{ fontSize: '13px', color: '#64748b' }}>هزینه مصرفی</span>
            <strong style={{ fontSize: '16px', color: '#ef4444' }} dir="ltr">${faDigits(total.cost.toFixed(2))}</strong>
          </div>
        </div>
      </div>

      {payModal && (
        <div style={{ background: '#fff', borderRadius: '16px', padding: '24px', border: '1px solid #e2e8f0', marginBottom: '24px' }}>
          <h3 style={{ margin: '0 0 16px 0', fontSize: '16px', color: '#0f172a' }}>افزایش موجودی (NabuPay)</h3>
          <div style={{ display: 'flex', gap: '12px', alignItems: 'center' }}>
            <div style={{ position: 'relative' }}>
              <input type="number" value={amountUsd} onChange={(e) => setAmountUsd(e.target.value)} style={{ padding: '10px 12px 10px 30px', borderRadius: '8px', border: '1px solid #cbd5e1', width: '120px', fontSize: '16px', textAlign: 'left' }} dir="ltr" />
              <span style={{ position: 'absolute', left: '12px', top: '10px', color: '#64748b' }}>$</span>
            </div>
            <div style={{ fontSize: '13px', color: '#64748b' }}>معادل {faInt(amountUsd * 85000)} تومان</div>
          </div>
          <div style={{ display: 'flex', gap: '12px', marginTop: '20px' }}>
            <button onClick={() => startPayment('toman')} style={{ background: '#10b981', border: 'none', color: '#fff', padding: '10px 20px', borderRadius: '8px', fontSize: '13px', fontWeight: '700', cursor: 'pointer' }}>پرداخت ریالی (شتاب)</button>
            <button onClick={() => startPayment('usdt')} style={{ background: '#f59e0b', border: 'none', color: '#fff', padding: '10px 20px', borderRadius: '8px', fontSize: '13px', fontWeight: '700', cursor: 'pointer' }}>پرداخت تتری (USDT)</button>
            <button onClick={() => setPayModal(false)} style={{ background: '#f1f5f9', border: 'none', color: '#475569', padding: '10px 20px', borderRadius: '8px', fontSize: '13px', fontWeight: '700', cursor: 'pointer' }}>انصراف</button>
          </div>
        </div>
      )}

      {/* Quick Setup */}
      <div style={{ background: '#fff', borderRadius: '16px', padding: '24px', border: '1px solid #e2e8f0', marginBottom: '24px' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '20px' }}>
          <div>
            <h3 style={{ margin: '0 0 4px 0', fontSize: '16px', color: '#0f172a' }}>کلیدهای API شما</h3>
            <p style={{ margin: 0, fontSize: '13px', color: '#64748b' }}>برای استفاده در Claude Code, Cursor, Python و...</p>
          </div>
          <button onClick={createToken} disabled={creating} style={{ background: '#0f172a', border: 'none', color: '#fff', padding: '10px 20px', borderRadius: '8px', fontSize: '13px', fontWeight: '700', cursor: 'pointer' }}>
            {creating ? 'در حال ساخت...' : '+ ساخت کلید جدید'}
          </button>
        </div>

        {minted && (
          <div style={{ background: '#ecfdf5', border: '1px solid #10b981', padding: '16px', borderRadius: '12px', marginBottom: '20px' }}>
            <strong style={{ color: '#065f46', display: 'block', marginBottom: '8px' }}>کلید ساخته شد! (فقط یک‌بار نمایش داده می‌شود)</strong>
            <code style={{ display: 'block', background: '#fff', padding: '12px', borderRadius: '8px', border: '1px solid #a7f3d0', color: '#047857', fontSize: '15px', direction: 'ltr', textAlign: 'left' }}>
              {minted.secret}
            </code>
          </div>
        )}

        {tokens.length === 0 && !minted ? (
          <div style={{ textAlign: 'center', padding: '40px', color: '#64748b', fontSize: '14px', background: '#f8fafc', borderRadius: '12px' }}>
            هیچ کلیدی ندارید. برای شروع روی ساخت کلید جدید کلیک کنید.
          </div>
        ) : (
          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'right', fontSize: '13px' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid #e2e8f0', color: '#64748b' }}>
                  <th style={{ padding: '12px 8px' }}>نام اپ</th>
                  <th style={{ padding: '12px 8px' }}>پیشوند کلید</th>
                  <th style={{ padding: '12px 8px' }}>تاریخ ایجاد</th>
                  <th style={{ padding: '12px 8px' }}>آخرین استفاده</th>
                </tr>
              </thead>
              <tbody>
                {tokens.map(t => (
                  <tr key={t.name} style={{ borderBottom: '1px solid #f1f5f9' }}>
                    <td style={{ padding: '12px 8px', fontWeight: '600', color: '#0f172a' }}>{t.name}</td>
                    <td style={{ padding: '12px 8px', color: '#64748b' }} dir="ltr">{t.prefix}...</td>
                    <td style={{ padding: '12px 8px', color: '#64748b' }} dir="ltr">{new Date(t.created_at).toLocaleDateString('fa-IR')}</td>
                    <td style={{ padding: '12px 8px', color: '#64748b' }} dir="ltr">{t.last_used ? new Date(t.last_used).toLocaleDateString('fa-IR') : 'هرگز'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

    </Layout>
  );
}
