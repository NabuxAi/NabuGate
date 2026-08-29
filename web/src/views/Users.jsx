import { useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';

export default function Users() {
  const [users, setUsers] = useState([]);
  const [admins, setAdmins] = useState([]);
  const [error, setError] = useState(null);
  
  // Recharge form state
  const [selectedUser, setSelectedUser] = useState('');
  const [amount, setAmount] = useState('');
  const [busy, setBusy] = useState(false);
  const [ok, setOk] = useState(null);

  const load = () => {
    api.listAdmins()
      .then((r) => setAdmins(r.admins || []))
      .catch((e) => setError(e.message));

    api.listUsers()
      .then((r) => setUsers(r.users || []))
      .catch((e) => setError(e.message));
  };

  useEffect(load, []);

  const handleRecharge = async (e) => {
    e.preventDefault();
    setError(null);
    setOk(null);
    setBusy(true);
    try {
      await api.adminRechargeUser(selectedUser, amount);
      setOk(`موجودی کاربر ${selectedUser} با موفقیت افزایش یافت.`);
      setSelectedUser('');
      setAmount('');
      await load();
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Layout
      title="کاربران سیستم"
      subtitle="مدیریت کاربران، مدیران، و افزایش دستی موجودی"
    >
      {error && <div className="card banner-error">{error}</div>}
      {ok && <div className="card" style={{ background: 'var(--ng-ok-soft)', color: 'var(--ng-ok-text)', border: '1px solid var(--ng-ok)', padding: 16 }}>{ok}</div>}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(300px, 1fr))', gap: 24, marginBottom: 24 }}>
        <div className="card">
          <div className="card-head">لیست کاربران (مشتریان)</div>
          <table className="tbl">
            <thead>
              <tr>
                <th>ایمیل</th>
                <th>موجودی</th>
                <th>تراکنش‌ها</th>
              </tr>
            </thead>
            <tbody>
              {users.length === 0 && (
                <tr><td colSpan={3} style={{ color: 'var(--ng-muted)', padding: 18 }}>کاربری یافت نشد.</td></tr>
              )}
              {users.map(u => (
                <tr key={u.email}>
                  <td style={{ fontWeight: 700 }} dir="ltr">{u.email}</td>
                  <td className="mono">{Number(u.balance || 0).toLocaleString('fa-IR')}</td>
                  <td className="mono">{u.payments ? u.payments.length : 0}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <div className="card" style={{ display: 'flex', flexDirection: 'column' }}>
          <div className="card-head">افزایش دستی موجودی کاربر</div>
          <p style={{ color: 'var(--ng-muted)', fontSize: 13, marginBottom: 16 }}>از این بخش می‌توانید حساب یک کاربر را بدون نیاز به پرداخت بانکی شارژ کنید (مثلاً برای تست یا هدیه).</p>
          <form onSubmit={handleRecharge} style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            <label style={{ fontSize: 13 }}>
              ایمیل کاربر:
              <input
                className="signin-field"
                placeholder="user@example.com"
                value={selectedUser}
                onChange={(e) => setSelectedUser(e.target.value)}
                dir="ltr"
                required
                style={{ width: '100%', marginTop: 8 }}
              />
            </label>
            <label style={{ fontSize: 13 }}>
              مبلغ (دلار):
              <input
                className="signin-field"
                type="number"
                placeholder="50000"
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
                dir="ltr"
                required
                style={{ width: '100%', marginTop: 8 }}
              />
            </label>
            <button className="btn btn-primary" type="submit" disabled={busy} style={{ marginTop: 12 }}>
              {busy ? 'در حال اعمال...' : 'اعمال شارژ حساب'}
            </button>
          </form>
        </div>
      </div>

      <div className="card">
        <div className="card-head">لیست مدیران سیستم ({admins.length})</div>
        <div className="rows">
          {admins.map((u) => (
            <div key={u} className="row">
              <span className="mono">{u}</span>
              <span className="tag tag-primary">ادمین</span>
            </div>
          ))}
          {admins.length === 0 && <p className="card-sub">هیچ ادمینی جز مدیر اصلی وجود ندارد.</p>}
        </div>
      </div>
    </Layout>
  );
}
