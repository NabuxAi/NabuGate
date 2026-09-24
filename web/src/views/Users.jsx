import { useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { fmtInt, fmtNum, useT } from '../i18n/index.jsx';

const T = {
  fa: {
    title: 'کاربران سیستم',
    subtitle: 'مدیریت کاربران، مدیران، و افزایش دستی موجودی',
    recharged: 'موجودی کاربر {user} با موفقیت افزایش یافت.',
    usersList: 'لیست کاربران (مشتریان)',
    email: 'ایمیل',
    balance: 'موجودی',
    transactions: 'تراکنش‌ها',
    noUsers: 'کاربری یافت نشد.',
    rechargeTitle: 'افزایش دستی موجودی کاربر',
    rechargeHint: 'از این بخش می‌توانید حساب یک کاربر را بدون نیاز به پرداخت بانکی شارژ کنید (مثلاً برای تست یا هدیه).',
    userEmail: 'ایمیل کاربر:',
    amount: 'مبلغ (دلار):',
    applying: 'در حال اعمال...',
    apply: 'اعمال شارژ حساب',
    adminsList: 'لیست مدیران سیستم ({n})',
    admin: 'ادمین',
    noAdmins: 'هیچ ادمینی جز مدیر اصلی وجود ندارد.',
  },
  en: {
    title: 'Users',
    subtitle: 'Manage users and admins, and top up balances by hand',
    recharged: 'Balance for {user} was topped up.',
    usersList: 'Users (customers)',
    email: 'Email',
    balance: 'Balance',
    transactions: 'Transactions',
    noUsers: 'No users found.',
    rechargeTitle: 'Manual top-up',
    rechargeHint: 'Credit a user’s account without a bank payment (e.g. for testing or as a gift).',
    userEmail: 'User email:',
    amount: 'Amount (USD):',
    applying: 'Applying...',
    apply: 'Apply top-up',
    adminsList: 'Admins ({n})',
    admin: 'Admin',
    noAdmins: 'No admins besides the primary administrator.',
  },
};

export default function Users() {
  const t = useT(T);
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
      setOk(t('recharged', { user: selectedUser }));
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
      title={t('title')}
      subtitle={t('subtitle')}
    >
      {error && <div className="card banner-error">{error}</div>}
      {ok && <div className="card" style={{ background: 'var(--ng-ok-soft)', color: 'var(--ng-ok-text)', border: '1px solid var(--ng-ok)', padding: 16 }}>{ok}</div>}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(300px, 1fr))', gap: 24, marginBottom: 24 }}>
        <div className="card">
          <div className="card-head">{t('usersList')}</div>
          <table className="tbl">
            <thead>
              <tr>
                <th>{t('email')}</th>
                <th>{t('balance')}</th>
                <th>{t('transactions')}</th>
              </tr>
            </thead>
            <tbody>
              {users.length === 0 && (
                <tr><td colSpan={3} style={{ color: 'var(--ng-muted)', padding: 18 }}>{t('noUsers')}</td></tr>
              )}
              {users.map(u => (
                <tr key={u.email}>
                  <td style={{ fontWeight: 700 }} dir="ltr">{u.email}</td>
                  <td className="mono">{fmtNum(u.balance)}</td>
                  <td className="mono">{fmtInt(u.payments ? u.payments.length : 0)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <div className="card" style={{ display: 'flex', flexDirection: 'column' }}>
          <div className="card-head">{t('rechargeTitle')}</div>
          <p style={{ color: 'var(--ng-muted)', fontSize: 13, marginBottom: 16 }}>{t('rechargeHint')}</p>
          <form onSubmit={handleRecharge} style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            <label style={{ fontSize: 13 }}>
              {t('userEmail')}
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
              {t('amount')}
              <input
                className="signin-field"
                type="number"
                placeholder="50"
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
                dir="ltr"
                required
                style={{ width: '100%', marginTop: 8 }}
              />
            </label>
            <button className="btn btn-primary" type="submit" disabled={busy} style={{ marginTop: 12 }}>
              {busy ? t('applying') : t('apply')}
            </button>
          </form>
        </div>
      </div>

      <div className="card">
        <div className="card-head">{t('adminsList', { n: fmtInt(admins.length) })}</div>
        <div className="rows">
          {admins.map((u) => (
            <div key={u} className="row">
              <span className="mono">{u}</span>
              <span className="tag tag-primary">{t('admin')}</span>
            </div>
          ))}
          {admins.length === 0 && <p className="card-sub">{t('noAdmins')}</p>}
        </div>
      </div>
    </Layout>
  );
}
