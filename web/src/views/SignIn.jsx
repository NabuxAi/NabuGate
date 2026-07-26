import { useState } from 'react';

import * as api from '../api.js';

/*
 * The console's gate.
 *
 * On a fresh gateway there is no account, so this offers to create the first
 * one — the setup endpoint refuses forever after, so it cannot be used to add
 * a second from outside. That first-run window is the one moment the console is
 * open, which is why the gateway logs a line about it at startup.
 */
export default function SignIn({ needsSetup, onAuthenticated }) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);

  const creating = needsSetup;

  async function submit(e) {
    e.preventDefault();
    setError(null);

    if (creating && password !== confirm) {
      setError('رمز عبور و تکرارش یکی نیستند.');
      return;
    }
    setBusy(true);
    try {
      if (creating) await api.setup(username, password);
      else await api.login(username, password);
      onAuthenticated();
    } catch (err) {
      setError(err.message || 'ورود ناموفق بود.');
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="signin">
      <form className="signin-card card" onSubmit={submit}>
        <div className="signin-brand">
          NabuGate
          <span>دروازهٔ مرکزی هوش مصنوعی</span>
        </div>

        {creating && (
          <p className="signin-note">
            هنوز حسابی ساخته نشده. اولین حساب را همین‌جا بساز — بعد از آن این فرم
            بسته می‌شود و کسی نمی‌تواند از بیرون حساب اضافه کند.
          </p>
        )}

        <label className="signin-field">
          نام کاربری
          <input
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoComplete="username"
            dir="ltr"
            required
          />
        </label>

        <label className="signin-field">
          رمز عبور
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete={creating ? 'new-password' : 'current-password'}
            dir="ltr"
            required
          />
          {creating && (
            <span className="signin-hint">
              حداقل ۱۰ نویسه. یک عبارت بلند از یک رمز کوتاهِ پیچیده امن‌تر است.
            </span>
          )}
        </label>

        {creating && (
          <label className="signin-field">
            تکرار رمز عبور
            <input
              type="password"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
              autoComplete="new-password"
              dir="ltr"
              required
            />
          </label>
        )}

        {error && <p className="signin-error">{error}</p>}

        <button className="btn btn-primary signin-submit" disabled={busy}>
          {busy ? '…' : creating ? 'ساخت حساب مدیر' : 'ورود'}
        </button>
      </form>
    </div>
  );
}
