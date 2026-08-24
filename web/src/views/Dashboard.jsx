import { useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { faInt, faDigits } from '../data/mock.js';

export default function Dashboard() {
  const [data, setData] = useState(null);
  const [tokens, setTokens] = useState([]);
  const [error, setError] = useState(null);
  const [user, setUser] = useState(null);

  const load = () => {
    api.overview().then(setData).catch((e) => setError(e.message));
    api.listTokens().then((r) => setTokens(r.tokens || [])).catch(() => {});
    api.getMe().then(setUser).catch(() => {});
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

  const activeKeys = tokens.length;
  const balance = user ? user.balance || 0 : 0;
  const username = user?.name || user?.email?.split('@')[0] || "کاربر";

  return (
    <Layout title="داشبورد" subtitle="">
      {error && <div className="card banner-error">{error}</div>}

      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 32 }}>
        <div>
          <h2 style={{ fontSize: 24, fontWeight: 800, margin: 0, color: 'var(--ng-heading)' }}>سلام، {username} 👋</h2>
          <p style={{ color: 'var(--ng-muted)', fontSize: 13, marginTop: 4 }}>نمای کلی حساب، اشتراک و مصرف شما.</p>
        </div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', gap: 20, marginBottom: 24 }}>
        
        {/* Account Status Card */}
        <div className="card" style={{ padding: 24, display: 'flex', flexDirection: 'column' }}>
          <h3 style={{ fontSize: 16, margin: '0 0 20px 0', borderBottom: '1px solid var(--ng-border)', paddingBottom: 12 }}>وضعیت حساب</h3>
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 16 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <span style={{ fontSize: 13, color: 'var(--ng-muted)' }}>ایمیل</span>
              <span style={{ fontSize: 13, fontWeight: 700 }}>{user?.email || "..."}</span>
            </div>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <span style={{ fontSize: 13, color: 'var(--ng-muted)' }}>موجودی فعلی</span>
              <span style={{ fontSize: 15, fontWeight: 800, color: 'var(--ng-ok-text)' }} dir="ltr">
                ${faDigits(balance.toFixed(2))}
              </span>
            </div>
          </div>
          <button style={{ marginTop: 20, background: 'var(--ng-surface-soft)', border: '1px solid var(--ng-border)', color: 'var(--ng-heading)', padding: '10px', borderRadius: 8, fontSize: 13, cursor: 'pointer', width: '100%' }} onClick={() => window.location.hash = '#/account'}>
            مدیریت حساب و شارژ
          </button>
        </div>

        {/* Current Plan Card */}
        <div className="card" style={{ padding: 24, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', textAlign: 'center', minHeight: 220, gridColumn: 'span 2' }}>
          <h3 style={{ fontSize: 16, margin: '0 0 16px 0', alignSelf: 'flex-start', borderBottom: '1px solid var(--ng-border)', paddingBottom: 12, width: '100%', textAlign: 'right' }}>اشتراک فعلی</h3>
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center' }}>
            <div style={{ fontSize: 40, marginBottom: 16, opacity: 0.5 }}>✨</div>
            <div style={{ fontSize: 16, fontWeight: 700, marginBottom: 8 }}>شما از پرداخت پرداخت-به-اندازه-مصرف (PAYG) استفاده می‌کنید</div>
            <p style={{ fontSize: 13, color: 'var(--ng-muted)', marginBottom: 20 }}>موجودی خود را شارژ کنید و به میزان مصرف پرداخت کنید.</p>
            <button onClick={() => window.location.hash = '#/account'} className="btn btn-primary">شارژ حساب</button>
          </div>
        </div>
      </div>

      {/* 4 Stats Cards */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: 20, marginBottom: 24 }}>
        <div className="card" style={{ padding: 24, display: 'flex', flexDirection: 'column', alignItems: 'center', textAlign: 'center' }}>
          <div style={{ color: 'var(--ng-muted)', fontSize: 13, marginBottom: 8 }}>مصرف ۳۰ روز اخیر</div>
          <div style={{ fontSize: 24, fontWeight: 800 }}>{faInt(total.tokens)} <span style={{ fontSize: 12, color: 'var(--ng-muted)', fontWeight: 400 }}>توکن</span></div>
        </div>
        <div className="card" style={{ padding: 24, display: 'flex', flexDirection: 'column', alignItems: 'center', textAlign: 'center' }}>
          <div style={{ color: 'var(--ng-muted)', fontSize: 13, marginBottom: 8 }}>درخواست‌ها</div>
          <div style={{ fontSize: 24, fontWeight: 800 }}>{faInt(total.requests)}</div>
        </div>
        <div className="card" style={{ padding: 24, display: 'flex', flexDirection: 'column', alignItems: 'center', textAlign: 'center' }}>
          <div style={{ color: 'var(--ng-muted)', fontSize: 13, marginBottom: 8 }}>کلیدهای فعال</div>
          <div style={{ fontSize: 24, fontWeight: 800 }}>{faInt(activeKeys)}</div>
        </div>
        <div className="card" style={{ padding: 24, display: 'flex', flexDirection: 'column', alignItems: 'center', textAlign: 'center' }}>
          <div style={{ color: 'var(--ng-muted)', fontSize: 13, marginBottom: 8 }}>تیم‌های من</div>
          <div style={{ fontSize: 24, fontWeight: 800 }}>{faInt(1)}</div>
        </div>
      </div>

    </Layout>
  );
}
