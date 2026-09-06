import { useEffect, useMemo, useState } from 'react';

import Layout from '../components/Layout.jsx';
import VendorIcon from '../components/VendorIcon.jsx';
import * as api from '../api.js';
import { Skeleton } from '../components/Skeleton.jsx';

/*
 * The provider catalogue.
 *
 * This screen used to list only the providers whose adapter came up, in a
 * two-column table. That answered "what is running" and nothing a person
 * actually asks: can I use this, with whose key, and what do I do about it if
 * the answer is no.
 *
 * Every upstream the gateway knows of is here — including ones it does not
 * route to yet, because those are precisely the ones worth asking for, and a
 * request needs somewhere to be made.
 */

const CAP_LABEL = {
  chat: 'گفت‌وگو',
  image: 'تصویر',
  speech: 'گفتار',
  transcription: 'رونویسی',
  embedding: 'امبدینگ',
  video: 'ویدیو',
  documents: 'سند',
  photos: 'عکس',
};

const FILTERS = [
  { id: 'all', label: 'همه' },
  { id: 'live', label: 'الان کار می‌کند' },
  { id: 'mine', label: 'کلید خودم' },
  { id: 'transcription', label: 'رونویسی' },
  { id: 'chat', label: 'گفت‌وگو' },
  { id: 'iran', label: 'از ایران' },
  { id: 'missing', label: 'هنوز وصل نشده' },
];

export default function Providers() {
  const [rows, setRows] = useState(null);
  const [canStore, setCanStore] = useState(true);
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState(null);
  const [filter, setFilter] = useState('all');
  const [editing, setEditing] = useState(null); // provider name whose key form is open
  const [draftKey, setDraftKey] = useState('');
  const [busy, setBusy] = useState(null);

  const load = () =>
    api
      .listProviders()
      .then((d) => {
        setRows(d.providers || []);
        setCanStore(d.can_store_keys !== false);
      })
      .catch((e) => setError(e.message));

  useEffect(() => {
    load();
  }, []);

  const shown = useMemo(() => {
    if (!rows) return null;
    switch (filter) {
      case 'live': return rows.filter((p) => p.live);
      case 'mine': return rows.filter((p) => p.have_key);
      case 'iran': return rows.filter((p) => p.iran);
      case 'missing': return rows.filter((p) => !p.configured);
      case 'all': return rows;
      default: return rows.filter((p) => (p.capabilities || []).includes(filter));
    }
  }, [rows, filter]);

  const saveKey = async (name) => {
    setBusy(name);
    setError(null);
    try {
      await api.saveProviderKey(name, draftKey.trim());
      setNotice(`کلید شما برای ${name} ذخیره شد. از این به بعد درخواست‌هایتان اول با همان می‌رود.`);
      setEditing(null);
      setDraftKey('');
      await load();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(null);
    }
  };

  const dropKey = async (name) => {
    setBusy(name);
    try {
      await api.deleteProviderKey(name);
      setNotice(`کلید شما برای ${name} پاک شد.`);
      await load();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(null);
    }
  };

  const ask = async (name) => {
    setBusy(name);
    setError(null);
    try {
      const r = await api.requestProvider(name, '');
      setNotice(
        r.status === 'approved'
          ? `دسترسی ${name} روی کلید ما همان لحظه باز شد.`
          : `درخواست ${name} ثبت شد و منتظر تأیید است.`,
      );
      await load();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(null);
    }
  };

  return (
    <Layout
      title="پرووایدرها"
      subtitle="هر سرویسی که این دروازه می‌شناسد — و اینکه با کلید چه کسی به آن وصل می‌شوی"
    >
      {error && <div className="card banner-error">{error}</div>}
      {notice && <div className="card banner-ok">{notice}</div>}
      {!canStore && (
        <div className="card banner-warn">
          این دروازه <code className="ltr">NABUGATE_SECRET_KEY</code> ندارد، پس کلید را ذخیره نمی‌کند —
          نگه‌داشتن کلیدِ رمزنشده بدتر از ذخیره‌نکردن است. تا وقتی ست شود، کلیدت را با هدرِ
          <code className="ltr"> X-Nabu-Key-&lt;provider&gt;</code> روی همان درخواست بفرست.
        </div>
      )}

      <div className="card" style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
        {FILTERS.map((f) => (
          <button
            key={f.id}
            className={filter === f.id ? 'btn btn-sm' : 'btn btn-sm btn-ghost'}
            onClick={() => setFilter(f.id)}
          >
            {f.label}
          </button>
        ))}
      </div>

      {shown === null && (
        <div className="card"><div className="sk-stack"><Skeleton h={54} /><Skeleton h={54} /><Skeleton h={54} /></div></div>
      )}

      <div className="prov-grid">
        {shown?.map((p) => (
          <div className="card prov-card" key={p.name}>
            <div className="prov-head">
              <VendorIcon vendor={p} />
              <div style={{ minWidth: 0 }}>
                <div className="prov-name">{p.label || p.name}</div>
                <div className="prov-slug ltr">{p.name}</div>
              </div>
              <span className={p.live ? 'tag tag-ok' : p.configured ? 'tag' : 'tag tag-muted'}>
                {p.live ? 'فعال' : p.configured ? 'بی‌کلید' : 'وصل نشده'}
              </span>
            </div>

            {p.blurb && <p className="prov-blurb">{p.blurb}</p>}

            <div className="prov-caps">
              {(p.capabilities || []).map((c) => (
                <span className="tag tag-cap" key={c}>{CAP_LABEL[c] || c}</span>
              ))}
              {p.iran && <span className="tag tag-cap">از ایران</span>}
            </div>

            <div className="prov-state">
              {p.uses_gateway_key ? (
                <span className="prov-ok">✓ با کلید ما در دسترس است</span>
              ) : p.grant === 'pending' ? (
                <span className="prov-wait">⏳ درخواستت ثبت شده، منتظر تأیید</span>
              ) : p.grant === 'denied' ? (
                <span className="prov-no">درخواستت رد شد — می‌توانی دوباره بخواهی</span>
              ) : p.live ? (
                <span className="prov-no">با کلید ما نیاز به تأیید دارد</span>
              ) : (
                <span className="prov-no">با کلید ما هنوز در دسترس نیست</span>
              )}
              {p.have_key && (
                <span className="prov-mine">
                  کلید خودت ذخیره است <code className="ltr">{p.key_prefix}</code>
                </span>
              )}
            </div>

            <div className="prov-actions">
              {p.byok && canStore && editing !== p.name && (
                <button className="btn btn-sm" onClick={() => { setEditing(p.name); setDraftKey(''); }}>
                  {p.have_key ? 'تعویض کلیدم' : 'کلید خودم را بگذار'}
                </button>
              )}
              {p.have_key && (
                <button className="btn btn-sm btn-ghost" disabled={busy === p.name} onClick={() => dropKey(p.name)}>
                  حذف کلیدم
                </button>
              )}
              {!p.uses_gateway_key && p.grant !== 'pending' && (
                <button className="btn btn-sm btn-ghost" disabled={busy === p.name} onClick={() => ask(p.name)}>
                  {p.configured ? 'درخواست کلید ما' : 'درخواست اضافه‌شدن'}
                </button>
              )}
              {p.keys_url && (
                <a className="btn btn-sm btn-ghost ltr" href={p.keys_url} target="_blank" rel="noreferrer">
                  دریافت کلید ↗
                </a>
              )}
            </div>

            {editing === p.name && (
              <div className="prov-form">
                <input
                  className="inp ltr"
                  type="password"
                  autoComplete="off"
                  placeholder={`کلید ${p.label || p.name}`}
                  value={draftKey}
                  onChange={(e) => setDraftKey(e.target.value)}
                />
                <button className="btn btn-sm" disabled={!draftKey.trim() || busy === p.name} onClick={() => saveKey(p.name)}>
                  ذخیره
                </button>
                <button className="btn btn-sm btn-ghost" onClick={() => setEditing(null)}>انصراف</button>
                <p className="prov-hint">
                  رمزنگاری‌شده ذخیره می‌شود و هرگز در هیچ پاسخی برنمی‌گردد — فقط چند حرف اولش را می‌بینی.
                  فراخوانی‌ای که با کلید خودت انجام شود، از اعتبار تو نزد ما کم نمی‌کند.
                </p>
              </div>
            )}
            {!p.configured && !p.keys_url && (
              <p className="prov-hint">این سرویس هنوز به دروازه وصل نشده. درخواست بده تا اضافه شود.</p>
            )}
          </div>
        ))}
      </div>
    </Layout>
  );
}
