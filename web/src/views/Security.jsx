import { useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';

export default function Security() {
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
      setError('رمز جدید و تکرار آن یکی نیستند.');
      return;
    }
    if (next.length < 8) {
      setError('رمز جدید باید دست‌کم ۸ کاراکتر باشد.');
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
    <Layout title="امنیت" subtitle="تغییر رمز عبور حساب.">
      {error && <div className="card banner-error">{error}</div>}
      {done && <div className="card banner-ok">رمز عبور تغییر کرد.</div>}

      <div className="card" style={{ maxWidth: 460 }}>
        <form onSubmit={submit} style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          <div>
            <label style={label} htmlFor="current-password">رمز فعلی</label>
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
            <label style={label} htmlFor="new-password">رمز جدید</label>
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
            <label style={label} htmlFor="confirm-password">تکرار رمز جدید</label>
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
            {busy ? 'در حال ذخیره…' : 'تغییر رمز'}
          </button>
        </form>
      </div>

      <p className="muted" style={{ marginTop: 16, fontSize: 13, maxWidth: 460, lineHeight: 1.7 }}>
        اگر با حساب نابو (SSO) وارد شده‌اید، این حساب رمزی برای تغییر ندارد و رمز
        عبورتان در NabuAuth مدیریت می‌شود.
      </p>
    </Layout>
  );
}
