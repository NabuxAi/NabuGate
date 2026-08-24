import { useEffect, useState } from 'react';

import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { faInt, faDigits } from '../data/mock.js';

/*
 * Per-app tokens.
 *
 * One token per application, so usage is attributed per app rather than pooled
 * under a single shared key — which is what happened before, when Dadebaran ran
 * on the gateway's admin key and its spend was indistinguishable from anyone
 * else's.
 *
 * The secret is shown exactly once. Only its hash is stored, so this dialog is
 * the only chance to copy it.
 */
export default function Tokens() {
  const [tokens, setTokens] = useState([]);
  const [usage, setUsage] = useState({});
  const [error, setError] = useState(null);
  const [creating, setCreating] = useState(false);
  const [minted, setMinted] = useState(null);

  const load = () => {
    api.listTokens().then((r) => setTokens(r.tokens || [])).catch((e) => setError(e.message));
    api.usage().then((r) => setUsage(r.by_project || {})).catch(() => {});
  };

  useEffect(load, []);

  async function remove(name) {
    if (!confirm(`توکن «${name}» حذف شود؟ برنامه‌ای که از آن استفاده می‌کند بلافاصله قطع می‌شود.`)) return;
    try {
      await api.deleteToken(name);
      load();
    } catch (e) {
      setError(e.message);
    }
  }

  async function toggle(t) {
    try {
      await api.setTokenDisabled(t.name, !t.disabled);
      load();
    } catch (e) {
      setError(e.message);
    }
  }

  return (
    <Layout
      title="توکن هر اپ"
      subtitle="یک توکن برای هر برنامه — مصرف جداگانه، دسترسی محدود، فیلتر مبدأ"
      actions={
        <button className="btn btn-primary" onClick={() => setCreating(true)}>
          + توکن جدید
        </button>
      }
    >
      {error && <div className="card banner-error">{error}</div>}

      {minted && <MintedDialog minted={minted} onClose={() => setMinted(null)} />}
      {creating && (
        <CreateDialog
          onClose={() => setCreating(false)}
          onCreated={(m) => {
            setCreating(false);
            setMinted(m);
            load();
          }}
        />
      )}

      <div className="card">
        <table className="tbl">
          <thead>
            <tr>
              <th>اپ</th>
              <th style={{ width: 120 }}>توکن</th>
              <th>دسترسی</th>
              <th>پروایدر مجاز</th>
              <th>مبدأ مجاز</th>
              <th style={{ width: 90 }}>درخواست</th>
              <th style={{ width: 90 }}>هزینه</th>
              <th style={{ width: 80 }}>ردشده</th>
              <th style={{ width: 130 }} />
            </tr>
          </thead>
          <tbody>
            {tokens.length === 0 && (
              <tr>
                <td colSpan={9} style={{ color: 'var(--ng-muted)', padding: 18 }}>
                  هنوز توکنی ساخته نشده. کلیدهای تعریف‌شده در config.yaml جداگانه کار
                  می‌کنند و اینجا نمایش داده نمی‌شوند.
                </td>
              </tr>
            )}
            {tokens.map((t) => {
              const u = usage[t.name] || {};
              return (
                <tr key={t.name} style={t.disabled ? { opacity: 0.5 } : undefined}>
                  <td style={{ fontWeight: 700, color: 'var(--ng-heading)' }}>
                    {t.name}
                    {t.disabled && (
                      <div>
                        <span className="badge badge-warn" style={{ marginTop: 4 }}>غیرفعال</span>
                      </div>
                    )}
                  </td>
                  <td>
                    <span className="mono ltr" style={{ fontSize: 12 }}>{t.prefix}…</span>
                  </td>
                  <td>
                    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                      {(t.allow || []).map((a) => (
                        <span key={a} className="tag ltr">{a}</span>
                      ))}
                    </div>
                  </td>
                                    <td>
                    {(t.providers || []).length === 0 ? (
                      <span style={{ color: 'var(--ng-muted)', fontSize: 12 }}>همه</span>
                    ) : (
                      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                        {t.providers.map((p) => (
                          <span key={p} className="tag ltr">{p}</span>
                        ))}
                      </div>
                    )}
                  </td>
                  <td>
                    {(t.allowed_origins || []).length === 0 ? (
                      <span style={{ color: 'var(--ng-muted)', fontSize: 12 }}>هرجا</span>
                    ) : (
                      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                        {t.allowed_origins.map((o) => (
                          <span key={o} className="tag ltr tag-pass">{o}</span>
                        ))}
                      </div>
                    )}
                  </td>
                  <td className="mono">{faInt(u.requests || 0)}</td>
                                    <td className="mono ltr">{faDigits('$' + (u.cost_usd || 0).toFixed(3))}</td>
                  <td className="mono" style={u.denied ? { color: 'var(--ng-danger, #c53030)' } : undefined}>
                    {faInt(u.denied || 0)}
                  </td>
                  <td style={{ textAlign: 'left' }}>
                    <button className="btn" onClick={() => toggle(t)}>
                      {t.disabled ? 'فعال' : 'غیرفعال'}
                    </button>{' '}
                    <button className="btn" onClick={() => remove(t.name)}>حذف</button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </Layout>
  );
}

function CreateDialog({ onClose, onCreated }) {
  const [name, setName] = useState('');
  const [allow, setAllow] = useState('');
  const [origins, setOrigins] = useState('');
  const [providers, setProviders] = useState('');
  const [rate, setRate] = useState(120);
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);

  const split = (s) => s.split(/[\s,،]+/).map((v) => v.trim()).filter(Boolean);

  async function submit(e) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const r = await api.createToken({
        name: name.trim(),
        allow: split(allow),
        rateLimit: Number(rate) || 0,
        allowedOrigins: split(origins),
        providers: split(providers),
      });
      onCreated(r);
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <form className="modal card" onClick={(e) => e.stopPropagation()} onSubmit={submit}>
        <h3>توکن جدید</h3>

        <label className="signin-field">
          نام اپ
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder="nabuwrite" dir="ltr" required />
          <span className="signin-hint">مصرف با همین نام ثبت می‌شود.</span>
        </label>

        <label className="signin-field">
          دسترسی
          <input value={allow} onChange={(e) => setAllow(e.target.value)} placeholder="write-*  nabu-fast" dir="ltr" required />
          <span className="signin-hint">
            الگوهای مجاز، با فاصله یا ویرگول. اجباری است: توکنی که به همه‌چیز برسد،
            توکنِ ادمین است.
          </span>
        </label>

                <label className="signin-field">
          پروایدرهای مجاز
          <input value={providers} onChange={(e) => setProviders(e.target.value)} placeholder="openai, groq" dir="ltr" />
          <span className="signin-hint">
            خالی یعنی همه. نام پروایدرها با فاصله یا ویرگول.
          </span>
        </label>

        <label className="signin-field">
          مبدأ مجاز
          <input value={origins} onChange={(e) => setOrigins(e.target.value)} placeholder="*.nabuxai.com" dir="ltr" />
          <span className="signin-hint">
            خالی یعنی هرجا. برای کلیدی که داخل یک وب‌اپ می‌نشیند پرش کن — آنجا کلید
            قابل مخفی‌ماندن نیست.
          </span>
        </label>

        <label className="signin-field">
          سقف نرخ (در دقیقه)
          <input type="number" min="0" value={rate} onChange={(e) => setRate(e.target.value)} dir="ltr" />
          <span className="signin-hint">۰ یعنی بی‌حد.</span>
        </label>

        {error && <p className="signin-error">{error}</p>}

        <div className="modal-actions">
          <button type="button" className="btn" onClick={onClose}>انصراف</button>
          <button className="btn btn-primary" disabled={busy}>{busy ? '…' : 'ساخت'}</button>
        </div>
      </form>
    </div>
  );
}

function MintedDialog({ minted, onClose }) {
  const [copied, setCopied] = useState(false);
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal card" onClick={(e) => e.stopPropagation()}>
        <h3>توکن «{minted.token?.name}» ساخته شد</h3>
        <p className="signin-note">
          این تنها باری است که نمایش داده می‌شود. فقط هشِ آن ذخیره شده، پس دوباره
          قابل بازیابی نیست — همین حالا کپی‌اش کن.
        </p>
        <pre className="mono ltr token-secret">{minted.secret}</pre>
        <div className="modal-actions">
          <button
            className="btn"
            onClick={() => {
              navigator.clipboard?.writeText(minted.secret);
              setCopied(true);
            }}
          >
            {copied ? 'کپی شد' : 'کپی'}
          </button>
          <button className="btn btn-primary" onClick={onClose}>بستن</button>
        </div>
      </div>
    </div>
  );
}
