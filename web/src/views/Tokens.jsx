import { useEffect, useState } from 'react';

import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { fmtInt, fmtDigits, useT } from '../i18n/index.jsx';
import { Skeleton } from '../components/Skeleton.jsx';
import Icon from '../components/Icon.jsx';

const T = {
  fa: {
    title: 'کلیدهای API',
    subtitle: 'یک توکن برای هر برنامه — مصرف جداگانه، دسترسی محدود، فیلتر مبدأ',
    newToken: 'توکن جدید',
    confirmDelete: 'توکن «{name}» حذف شود؟ برنامه‌ای که از آن استفاده می‌کند بلافاصله قطع می‌شود.',
    colApp: 'اپ',
    colToken: 'توکن',
    colAccess: 'دسترسی',
    colProviders: 'پروایدر مجاز',
    colOrigins: 'مبدأ مجاز',
    colRequests: 'درخواست',
    colCost: 'هزینه',
    colDenied: 'ردشده',
    empty: 'هنوز توکنی ساخته نشده. کلیدهای تعریف‌شده در config.yaml جداگانه کار می‌کنند و اینجا نمایش داده نمی‌شوند.',
    disabled: 'غیرفعال',
    all: 'همه',
    anywhere: 'هرجا',
    enable: 'فعال',
    disable: 'غیرفعال',
    edit: 'ویرایش',
    remove: 'حذف',
    createTitle: 'توکن جدید',
    appName: 'نام اپ',
    appNameHint: 'مصرف با همین نام ثبت می‌شود.',
    access: 'دسترسی',
    accessHint: 'الگوهای مجاز، با فاصله یا ویرگول. اجباری است: توکنی که به همه‌چیز برسد، توکنِ ادمین است.',
    providers: 'پروایدرهای مجاز',
    providersHint: 'خالی یعنی همه. نام پروایدرها با فاصله یا ویرگول.',
    origins: 'مبدأ مجاز',
    originsHint: 'خالی یعنی هرجا. برای کلیدی که داخل یک وب‌اپ می‌نشیند پرش کن — آنجا کلید قابل مخفی‌ماندن نیست.',
    rate: 'سقف نرخ (در دقیقه)',
    rateHint: '۰ یعنی بی‌حد.',
    cancel: 'انصراف',
    create: 'ساخت',
    mintedTitle: 'توکن «{name}» ساخته شد',
    mintedNote: 'این تنها باری است که نمایش داده می‌شود. فقط هشِ آن ذخیره شده، پس دوباره قابل بازیابی نیست — همین حالا کپی‌اش کن.',
    copied: 'کپی شد',
    copy: 'کپی',
    close: 'بستن',
    editTitle: 'ویرایش توکن «{name}»',
    providersHintShort: 'خالی یعنی همه.',
    originsHintShort: 'خالی یعنی هرجا.',
    save: 'ذخیره',
  },
  en: {
    title: 'API keys',
    subtitle: 'One token per app — separate usage, scoped access, origin filtering',
    newToken: 'New token',
    confirmDelete: 'Delete token “{name}”? Any app using it is cut off immediately.',
    colApp: 'App',
    colToken: 'Token',
    colAccess: 'Access',
    colProviders: 'Allowed providers',
    colOrigins: 'Allowed origins',
    colRequests: 'Requests',
    colCost: 'Cost',
    colDenied: 'Denied',
    empty: 'No tokens yet. Keys defined in config.yaml work separately and are not listed here.',
    disabled: 'Disabled',
    all: 'All',
    anywhere: 'Anywhere',
    enable: 'Enable',
    disable: 'Disable',
    edit: 'Edit',
    remove: 'Delete',
    createTitle: 'New token',
    appName: 'App name',
    appNameHint: 'Usage is recorded under this name.',
    access: 'Access',
    accessHint: 'Allowed patterns, separated by spaces or commas. Required: a token that reaches everything is an admin token.',
    providers: 'Allowed providers',
    providersHint: 'Empty means all. Provider names, separated by spaces or commas.',
    origins: 'Allowed origins',
    originsHint: 'Empty means anywhere. Fill it in for a key that lives inside a web app — a key cannot stay hidden there.',
    rate: 'Rate limit (per minute)',
    rateHint: '0 means unlimited.',
    cancel: 'Cancel',
    create: 'Create',
    mintedTitle: 'Token “{name}” created',
    mintedNote: 'This is the only time it is shown. Only its hash is stored, so it cannot be recovered — copy it now.',
    copied: 'Copied',
    copy: 'Copy',
    close: 'Close',
    editTitle: 'Edit token “{name}”',
    providersHintShort: 'Empty means all.',
    originsHintShort: 'Empty means anywhere.',
    save: 'Save',
  },
};

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
  const t = useT(T);
  const [tokens, setTokens] = useState(null);
  const [usage, setUsage] = useState({});
  const [error, setError] = useState(null);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState(null);
  const [minted, setMinted] = useState(null);

  const load = () => {
    api.listTokens().then((r) => setTokens(r.tokens || [])).catch((e) => setError(e.message));
    api.usage().then((r) => setUsage(r.by_project || {})).catch(() => {});
  };

  useEffect(load, []);

  async function remove(name) {
    if (!confirm(t('confirmDelete', { name }))) return;
    try {
      await api.deleteToken(name);
      load();
    } catch (e) {
      setError(e.message);
    }
  }

  async function toggle(tok) {
    try {
      await api.setTokenDisabled(tok.name, !tok.disabled);
      load();
    } catch (e) {
      setError(e.message);
    }
  }

  return (
    <Layout
      title={t('title')}
      subtitle={t('subtitle')}
      actions={
        <button className="btn btn-primary" onClick={() => setCreating(true)}>
          <Icon name="plus" size={16} />{t('newToken')}
        </button>
      }
    >
      {error && <div className="card banner-error">{error}</div>}

      {minted && <MintedDialog minted={minted} onClose={() => setMinted(null)} />}
            {editing && (
        <EditDialog
          token={editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            load();
          }}
        />
      )}
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
              <th>{t('colApp')}</th>
              <th style={{ width: 120 }}>{t('colToken')}</th>
              <th>{t('colAccess')}</th>
              <th>{t('colProviders')}</th>
              <th>{t('colOrigins')}</th>
              <th style={{ width: 90 }}>{t('colRequests')}</th>
              <th style={{ width: 90 }}>{t('colCost')}</th>
              <th style={{ width: 80 }}>{t('colDenied')}</th>
              <th style={{ width: 130 }} />
            </tr>
          </thead>
          <tbody>
            {tokens === null && [0, 1, 2].map((i) => (
              <tr key={'sk' + i}>
                {Array.from({ length: 9 }).map((_, c) => (
                  <td key={c}><Skeleton h={12} w={(40 + ((i * 17 + c * 23) % 50)) + '%'} /></td>
                ))}
              </tr>
            ))}
            {tokens !== null && tokens.length === 0 && (
              <tr>
                <td colSpan={9} style={{ color: 'var(--ng-muted)', padding: 18 }}>
                  {t('empty')}
                </td>
              </tr>
            )}
            {(tokens || []).map((tok) => {
              const u = usage[tok.name] || {};
              return (
                <tr key={tok.name} style={tok.disabled ? { opacity: 0.5 } : undefined}>
                  <td style={{ fontWeight: 700, color: 'var(--ng-heading)' }}>
                    {tok.name}
                    {tok.disabled && (
                      <div>
                        <span className="badge badge-warn" style={{ marginTop: 4 }}>{t('disabled')}</span>
                      </div>
                    )}
                  </td>
                  <td>
                    <span className="mono ltr" style={{ fontSize: 12 }}>{tok.prefix}…</span>
                  </td>
                  <td>
                    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                      {(tok.allow || []).map((a) => (
                        <span key={a} className="tag ltr">{a}</span>
                      ))}
                    </div>
                  </td>
                                    <td>
                    {(tok.providers || []).length === 0 ? (
                      <span style={{ color: 'var(--ng-muted)', fontSize: 12 }}>{t('all')}</span>
                    ) : (
                      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                        {tok.providers.map((p) => (
                          <span key={p} className="tag ltr">{p}</span>
                        ))}
                      </div>
                    )}
                  </td>
                  <td>
                    {(tok.allowed_origins || []).length === 0 ? (
                      <span style={{ color: 'var(--ng-muted)', fontSize: 12 }}>{t('anywhere')}</span>
                    ) : (
                      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                        {tok.allowed_origins.map((o) => (
                          <span key={o} className="tag ltr tag-pass">{o}</span>
                        ))}
                      </div>
                    )}
                  </td>
                  <td className="mono">{fmtInt(u.requests || 0)}</td>
                                    <td className="mono ltr">{fmtDigits('$' + (u.cost_usd || 0).toFixed(3))}</td>
                  <td className="mono" style={u.denied ? { color: 'var(--ng-danger, #c53030)' } : undefined}>
                    {fmtInt(u.denied || 0)}
                  </td>
                  <td style={{ textAlign: 'end' }}>
                    <button className="btn" onClick={() => toggle(tok)}>
                      {tok.disabled ? t('enable') : t('disable')}
                    </button>{' '}
                    <button className="btn" onClick={() => setEditing(tok)}>{t('edit')}</button>{' '}
                    <button className="btn" onClick={() => remove(tok.name)}>{t('remove')}</button>
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
  const t = useT(T);
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
        <h3>{t('createTitle')}</h3>

        <label className="signin-field">
          {t('appName')}
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder="nabuwrite" dir="ltr" required />
          <span className="signin-hint">{t('appNameHint')}</span>
        </label>

        <label className="signin-field">
          {t('access')}
          <input value={allow} onChange={(e) => setAllow(e.target.value)} placeholder="write-*  nabu-fast" dir="ltr" required />
          <span className="signin-hint">{t('accessHint')}</span>
        </label>

                <label className="signin-field">
          {t('providers')}
          <input value={providers} onChange={(e) => setProviders(e.target.value)} placeholder="openai, groq" dir="ltr" />
          <span className="signin-hint">{t('providersHint')}</span>
        </label>

        <label className="signin-field">
          {t('origins')}
          <input value={origins} onChange={(e) => setOrigins(e.target.value)} placeholder="*.nabuxai.com" dir="ltr" />
          <span className="signin-hint">{t('originsHint')}</span>
        </label>

        <label className="signin-field">
          {t('rate')}
          <input type="number" min="0" value={rate} onChange={(e) => setRate(e.target.value)} dir="ltr" />
          <span className="signin-hint">{t('rateHint')}</span>
        </label>

        {error && <p className="signin-error">{error}</p>}

        <div className="modal-actions">
          <button type="button" className="btn" onClick={onClose}>{t('cancel')}</button>
          <button className="btn btn-primary" disabled={busy}>{busy ? '…' : t('create')}</button>
        </div>
      </form>
    </div>
  );
}

function MintedDialog({ minted, onClose }) {
  const t = useT(T);
  const [copied, setCopied] = useState(false);
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal card" onClick={(e) => e.stopPropagation()}>
        <h3>{t('mintedTitle', { name: minted.token?.name ?? '' })}</h3>
        <p className="signin-note">{t('mintedNote')}</p>
        <pre className="mono ltr token-secret">{minted.secret}</pre>
        <div className="modal-actions">
          <button
            className="btn"
            onClick={() => {
              navigator.clipboard?.writeText(minted.secret);
              setCopied(true);
            }}
          >
            {copied ? t('copied') : t('copy')}
          </button>
          <button className="btn btn-primary" onClick={onClose}>{t('close')}</button>
        </div>
      </div>
    </div>
  );
}


function EditDialog({ token, onClose, onSaved }) {
  const t = useT(T);
  const [origins, setOrigins] = useState((token.allowed_origins || []).join(', '));
  const [providers, setProviders] = useState((token.providers || []).join(', '));
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);

  const split = (s) => s.split(/[\s,،]+/).map((v) => v.trim()).filter(Boolean);

  async function submit(e) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api.patchToken(token.name, split(origins), split(providers));
      onSaved();
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <form className="modal card" onClick={(e) => e.stopPropagation()} onSubmit={submit}>
        <h3>{t('editTitle', { name: token.name })}</h3>

        <label className="signin-field">
          {t('providers')}
          <input value={providers} onChange={(e) => setProviders(e.target.value)} placeholder="openai, groq" dir="ltr" />
          <span className="signin-hint">{t('providersHintShort')}</span>
        </label>

        <label className="signin-field">
          {t('origins')}
          <input value={origins} onChange={(e) => setOrigins(e.target.value)} placeholder="*.nabuxai.com" dir="ltr" />
          <span className="signin-hint">{t('originsHintShort')}</span>
        </label>

        {error && <p className="signin-error">{error}</p>}

        <div className="modal-actions">
          <button type="button" className="btn" onClick={onClose}>{t('cancel')}</button>
          <button className="btn btn-primary" disabled={busy}>{busy ? '…' : t('save')}</button>
        </div>
      </form>
    </div>
  );
}
