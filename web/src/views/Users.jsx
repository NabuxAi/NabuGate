import { useEffect, useState } from 'react';

import Layout from '../components/Layout.jsx';
import * as api from '../api.js';

/*
 * Admin accounts. Lists the console usernames and lets a signed-in admin add
 * another. Passwords are never shown or returned — only PBKDF2 hashes are stored.
 */
export default function Users() {
  const [admins, setAdmins] = useState([]);
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState(null);
  const [ok, setOk] = useState(null);
  const [busy, setBusy] = useState(false);

  const load = () =>
    api
      .listAdmins()
      .then((r) => setAdmins(r.admins || []))
      .catch((e) => setError(e.message));

  useEffect(load, []);

  const submit = async (e) => {
    e.preventDefault();
    setError(null);
    setOk(null);
    setBusy(true);
    try {
      await api.createAdmin(username.trim(), password);
      setOk(`کاربر «${username.trim().toLowerCase()}» اضافه شد.`);
      setUsername('');
      setPassword('');
      await load();
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Layout
      title="کاربران ادمین"
      subtitle="حساب‌های کنسول — افزودن ادمین جدید و مشاهدهٔ فهرست"
    >
      <div className="card">
        <div className="card-head">افزودن ادمین جدید</div>
        {error && <div className="banner-error">{error}</div>}
        {ok && <p className="card-sub" style={{ color: '#38c172' }}>{ok}</p>}
        <form onSubmit={submit} className="rows" style={{ maxWidth: 420 }}>
          <input
            className="signin-field"
            placeholder="نام کاربری"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoComplete="off"
            dir="ltr"
          />
          <input
            className="signin-field"
            type="password"
            placeholder="گذرواژه (حداقل ۱۰ کاراکتر)"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="new-password"
            dir="ltr"
          />
          <button className="btn btn-primary" type="submit" disabled={busy}>
            {busy ? '…' : 'افزودن ادمین'}
          </button>
        </form>
      </div>

      <div className="card">
        <div className="card-head">ادمین‌های فعلی ({admins.length})</div>
        <div className="rows">
          {admins.map((u) => (
            <div key={u} className="row">
              <span className="mono">{u}</span>
              <span className="tag tag-primary">ادمین</span>
            </div>
          ))}
          {admins.length === 0 && <p className="card-sub">هنوز ادمینی وجود ندارد.</p>}
        </div>
      </div>
    </Layout>
  );
}
