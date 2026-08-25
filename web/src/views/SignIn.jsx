import { useState, useEffect } from 'react';
import * as api from '../api.js';

/*
 * The console's gate.
 */
export default function SignIn({ needsSetup, onAuthenticated }) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);
  const [mode, setMode] = useState(needsSetup ? 'setup' : 'login'); // 'setup', 'login', 'signup'
  const [nabu, setNabu] = useState(null);

  useEffect(() => {
    if (!needsSetup) {
      api.statusNabu().then(setNabu).catch(console.error);
    }
  }, [needsSetup]);

  const creating = mode === 'setup' || mode === 'signup';
  const isPanel = window.location.pathname.startsWith('/panel');

  async function submit(e) {
    e.preventDefault();
    setError(null);

    if (creating && password !== confirm) {
      setError('رمز عبور و تکرارش یکی نیستند.');
      return;
    }
    setBusy(true);
    try {
      if (mode === 'setup') await api.setup(username, password);
      else if (mode === 'signup') await api.signup(username, password);
      else await api.login(username, password);
      onAuthenticated();
    } catch (err) {
      setError(err.message || 'عملیات ناموفق بود.');
    } finally {
      setBusy(false);
    }
  }

  const ssoClass = "btn btn-outline";
  const ssoStyle = { width: '100%', marginBottom: 12, display: 'flex', justifyContent: 'center', gap: 8 };

  return (
    <div className="signin">
      <form className="signin-card card" onSubmit={submit}>
        <div className="signin-brand">
          NabuGate
          <span>دروازهٔ مرکزی هوش مصنوعی</span>
        </div>

        {mode === 'setup' && (
          <p className="signin-note">
            هنوز حسابی ساخته نشده. اولین حساب را همین‌جا بساز — بعد از آن این فرم
            بسته می‌شود و کسی نمی‌تواند از بیرون حساب اضافه کند.
          </p>
        )}

        {mode === 'signup' && (
          <p className="signin-note">
            ایجاد حساب کاربری جدید
          </p>
        )}

        {mode === 'login' && !isPanel && (
          <p className="signin-note" style={{ textAlign: 'center', fontWeight: 'bold' }}>
            ورود به بخش مدیریت (Admin)
          </p>
        )}

        <label className="signin-field">
          {mode === 'setup' ? 'نام کاربری' : 'ایمیل'}
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
          {busy ? '…' : mode === 'setup' ? 'ساخت حساب مدیر' : mode === 'signup' ? 'ثبت‌نام' : 'ورود'}
        </button>

        {nabu && nabu.enabled && mode === 'login' && (
          <div style={{ marginTop: 24 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 16 }}>
              <div style={{ flex: 1, height: 1, background: 'var(--ng-border)' }}></div>
              <span style={{ fontSize: 12, color: 'var(--ng-muted)' }}>یا</span>
              <div style={{ flex: 1, height: 1, background: 'var(--ng-border)' }}></div>
            </div>
            <a href="/api/nabu" className={ssoClass} style={ssoStyle}>
              ورود با Nabu SSO
            </a>
            <a href="/api/nabu?provider=google" className={ssoClass} style={ssoStyle}>
              <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor"><path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" fill="#4285F4"/><path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" fill="#34A853"/><path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" fill="#FBBC05"/><path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" fill="#EA4335"/></svg>
              ورود با حساب گوگل
            </a>
          </div>
        )}
        
        {!needsSetup && (
          <div style={{ marginTop: 16, textAlign: 'center', fontSize: 13 }}>
            {mode === 'login' ? (
              <a href="#" onClick={(e) => { e.preventDefault(); setMode('signup'); setError(null); }}>
                حساب کاربری ندارید؟ ثبت‌نام کنید
              </a>
            ) : (
              <a href="#" onClick={(e) => { e.preventDefault(); setMode('login'); setError(null); }}>
                قبلاً ثبت‌نام کرده‌اید؟ وارد شوید
              </a>
            )}
          </div>
        )}
      </form>
    </div>
  );
}
