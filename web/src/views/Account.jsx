import { useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { faDigits } from '../data/mock.js';

export default function Account() {
  const [user, setUser] = useState(null);
  const [amount, setAmount] = useState(10);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);

  const load = () =>
    api.getMe()
      .then(setUser)
      .catch((e) => setError(e.message));

  useEffect(() => {
    load();
  }, []);

  async function recharge(e) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api.rechargeMe(amount);
      load();
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  }

  if (!user) return <Layout title="حساب کاربری"><div className="app-boot">…</div></Layout>;

  return (
    <Layout title="حساب کاربری" subtitle="نمای کلی حساب، اشتراک و مصرف شما.">
      {error && <div className="card banner-error">{error}</div>}

      <div className="stats">
        <div className="stat">
          <div className="stat-label">ایمیل</div>
          <div className="stat-value" style={{fontSize: 16}}>{user.email}</div>
        </div>
        <div className="stat">
          <div className="stat-label">موجودی فعلی</div>
          <div className="stat-value ltr">{faDigits('$' + (user.balance || 0).toFixed(2))}</div>
        </div>
      </div>

      <div className="card" style={{ marginTop: 16 }}>
        <h3 style={{ marginBottom: 12 }}>افزایش موجودی (شبیه‌سازی)</h3>
        <form onSubmit={recharge} style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <input 
            type="number" 
            value={amount} 
            onChange={e => setAmount(e.target.value)} 
            min="1" 
            style={{ width: 100, padding: 8, borderRadius: 6, border: '1px solid var(--ng-border)' }}
            dir="ltr"
          />
          <span>دلار</span>
          <button className="btn btn-primary" disabled={busy}>
            {busy ? 'در حال پرداخت…' : 'شارژ حساب'}
          </button>
        </form>
      </div>
    </Layout>
  );
}
