import { useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { useT } from '../i18n/index.jsx';

const T = {
  fa: {
    title: 'امنیت',
    subtitle: 'تغییر رمز عبور حساب.',
    mismatch: 'رمز جدید و تکرار آن یکی نیستند.',
    tooShort: 'رمز جدید باید دست‌کم ۸ کاراکتر باشد.',
    changed: 'رمز عبور تغییر کرد.',
    current: 'رمز فعلی',
    next: 'رمز جدید',
    confirm: 'تکرار رمز جدید',
    saving: 'در حال ذخیره…',
    change: 'تغییر رمز',
    ssoNote: 'اگر با حساب نابو (SSO) وارد شده‌اید، این حساب رمزی برای تغییر ندارد و رمز عبورتان در NabuAuth مدیریت می‌شود.',
  },
  en: {
    title: 'Security',
    subtitle: 'Change your account password.',
    mismatch: 'The new password and its confirmation don’t match.',
    tooShort: 'The new password must be at least 8 characters.',
    changed: 'Password changed.',
    current: 'Current password',
    next: 'New password',
    confirm: 'Confirm new password',
    saving: 'Saving…',
    change: 'Change password',
    ssoNote: 'If you signed in with a Nabu account (SSO), this account has no password to change — your password is managed in NabuAuth.',
  },
};

export default function Security() {
  const t = useT(T);
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [confirm, setConfirm] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [done, setDone] = useState(false);

  async function submit(e) {
    e.preventDefault();
    setError(null);
    setDone(false);

    // Checked here as well as on the server: the mismatch is a typo the person
    // can see and fix, and a round trip to be told about their own typo is a
    // worse way to find out.
    if (next !== confirm) {
      setError(t('mismatch'));
      return;
    }
    if (next.length < 8) {
      setError(t('tooShort'));
      return;
    }

    setBusy(true);
    try {
      await api.changeMyPassword(current, next);
      setDone(true);
      setCurrent('');
      setNext('');
      setConfirm('');
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  }

  const field = { width: '100%', marginTop: 6 };
  const label = { display: 'block', fontSize: 13, color: 'var(--ng-muted)' };

  return (
    <Layout title={t('title')} subtitle={t('subtitle')}>
      {error && <div className="card banner-error">{error}</div>}
      {done && <div className="card banner-ok">{t('changed')}</div>}

      <div className="card" style={{ maxWidth: 460 }}>
        <form onSubmit={submit} style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          <div>
            <label style={label} htmlFor="current-password">{t('current')}</label>
            <input
              id="current-password"
              type="password"
              className="input"
              style={field}
              dir="ltr"
              autoComplete="current-password"
              value={current}
              onChange={(e) => setCurrent(e.target.value)}
              required
            />
          </div>
          <div>
            <label style={label} htmlFor="new-password">{t('next')}</label>
            <input
              id="new-password"
              type="password"
              className="input"
              style={field}
              dir="ltr"
              autoComplete="new-password"
              value={next}
              onChange={(e) => setNext(e.target.value)}
              required
            />
          </div>
          <div>
            <label style={label} htmlFor="confirm-password">{t('confirm')}</label>
            <input
              id="confirm-password"
              type="password"
              className="input"
              style={field}
              dir="ltr"
              autoComplete="new-password"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
              required
            />
          </div>
          <button className="btn btn-primary" disabled={busy} style={{ alignSelf: 'flex-start' }}>
            {busy ? t('saving') : t('change')}
          </button>
        </form>
      </div>

      <p className="muted" style={{ marginTop: 16, fontSize: 13, maxWidth: 460, lineHeight: 1.7 }}>
        {t('ssoNote')}
      </p>
    </Layout>
  );
}
