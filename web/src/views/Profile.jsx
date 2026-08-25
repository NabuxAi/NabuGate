import { navigate } from "../nav.js";

import { useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';

export default function Profile() {
  const [user, setUser] = useState(null);
  const [error, setError] = useState(null);

  const load = () => {
    api.getMe().then(setUser).catch((e) => setError(e.message));
  };

  useEffect(load, []);

  const balance = user ? user.balance || 0 : 0;

  return (
    <Layout title="پروفایل کاربری" subtitle="اطلاعات حساب و تنظیمات پایه">
      {error && <div className="card banner-error">{error}</div>}

      <div className="card" style={{ maxWidth: 600 }}>
        <h3 style={{ fontSize: 16, marginBottom: 20 }}>اطلاعات حساب</h3>
        
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          <div>
            <label style={{ display: 'block', fontSize: 13, color: 'var(--ng-muted)', marginBottom: 6 }}>ایمیل (شناسه ورود)</label>
            <input type="text" className="input" value={user?.email || ''} readOnly style={{ width: '100%', opacity: 0.7 }} dir="ltr" />
            <p style={{ fontSize: 12, color: 'var(--ng-muted)', marginTop: 4 }}>ایمیل ثبت‌نامی شما.</p>
          </div>
          
          <div>
            <label style={{ display: 'block', fontSize: 13, color: 'var(--ng-muted)', marginBottom: 6 }}>موجودی حساب</label>
            <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
              <input type="text" className="input" value={balance.toLocaleString('fa-IR')} readOnly style={{ width: 150 }} />
              <span style={{ fontSize: 13, color: 'var(--ng-muted)' }}>تومان</span>
              <button className="btn btn-primary" style={{ marginRight: 'auto' }} onClick={() => navigate('plans')}>شارژ</button>
            </div>
          </div>
          
          <div>
            <label style={{ display: 'block', fontSize: 13, color: 'var(--ng-muted)', marginBottom: 6 }}>رمز عبور</label>
            <div style={{ fontSize: 13, padding: '10px 12px', background: 'var(--ng-surface-soft)', borderRadius: 8, border: '1px solid var(--ng-border)' }}>
              برای تغییر رمز عبور، لطفاً از حساب کاربری خود خارج شوید و از گزینه فراموشی رمز یا در صورت ورود از طریق SSO از طریق آن سیستم اقدام کنید.
            </div>
          </div>
        </div>
      </div>
    </Layout>
  );
}
