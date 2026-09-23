import { useEffect, useMemo, useState } from 'react';

import Layout from '../components/Layout.jsx';
import VendorIcon from '../components/VendorIcon.jsx';
import * as api from '../api.js';
import { fmtDate, fmtInt, useI18n, useT } from '../i18n/index.jsx';
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

// A capability with no label here renders as its raw name, so a new one on the
// server never blanks a tag.
const CAP_LABEL = {
  fa: {
    chat: 'گفت‌وگو',
    image: 'تصویر',
    speech: 'گفتار',
    transcription: 'رونویسی',
    embedding: 'امبدینگ',
    video: 'ویدیو',
    documents: 'سند',
    photos: 'عکس',
    decisions: 'تصمیم‌گیری',
  },
  en: {
    chat: 'Chat',
    image: 'Image',
    speech: 'Speech',
    transcription: 'Transcription',
    embedding: 'Embeddings',
    video: 'Video',
    documents: 'Documents',
    photos: 'Photos',
    decisions: 'Decisions',
  },
};

const FILTERS = ['all', 'live', 'mine', 'transcription', 'chat', 'iran', 'missing'];

const T = {
  fa: {
    title: 'پرووایدرها',
    subtitle: 'هر سرویسی که این دروازه می‌شناسد — و اینکه با کلید چه کسی به آن وصل می‌شوی',
    f_all: 'همه',
    f_live: 'الان کار می‌کند',
    f_mine: 'کلید خودم',
    f_transcription: 'رونویسی',
    f_chat: 'گفت‌وگو',
    f_iran: 'از ایران',
    f_missing: 'هنوز وصل نشده',
    saved: 'کلید شما برای {name} ذخیره شد. کلیدها به ترتیبِ افزودن امتحان می‌شوند؛ اگر اولی جواب ندهد، خودکار سراغ بعدی می‌رود.',
    dropped: 'کلید شما برای {name} پاک شد.',
    approvedNow: 'دسترسی {name} روی کلید ما همان لحظه باز شد.',
    requested: 'درخواست {name} ثبت شد و منتظر تأیید است.',
    subActive: (v) => (
      <>
        اشتراک <strong>{v.name}</strong> فعال است تا <span className="ltr">{v.date}</span> — سرویس‌هایی که
        پوشش می‌دهد با کلید خودِ ما در دسترس‌اند و مصرفشان از اعتبارت کم می‌شود.
      </>
    ),
    noSecret: () => (
      <>
        این دروازه <code className="ltr">NABUGATE_SECRET_KEY</code> ندارد، پس کلید را ذخیره نمی‌کند —
        نگه‌داشتن کلیدِ رمزنشده بدتر از ذخیره‌نکردن است. تا وقتی ست شود، کلیدت را با هدرِ
        <code className="ltr"> X-Nabu-Key-&lt;provider&gt;</code> روی همان درخواست بفرست.
      </>
    ),
    stLive: 'فعال',
    stNoKey: 'بی‌کلید',
    stMissing: 'وصل نشده',
    onOurKey: '✓ با کلید ما در دسترس است',
    pending: '⏳ درخواستت ثبت شده، منتظر تأیید',
    denied: 'درخواستت رد شد — می‌توانی دوباره بخواهی',
    needsApproval: 'با کلید ما نیاز به تأیید دارد',
    notOnOurKey: 'با کلید ما هنوز در دسترس نیست',
    planCovers: 'اشتراکت این را باز می‌کند',
    myKeys: '{n} کلید خودت — به ترتیب امتحان می‌شوند',
    myKey: 'کلید خودت ذخیره است',
    remove: 'حذف',
    addAnother: 'کلید دیگری اضافه کن',
    useMine: 'کلید خودم را بگذار',
    askOurKey: 'درخواست کلید ما',
    askAdd: 'درخواست اضافه‌شدن',
    getKey: 'دریافت کلید ↗',
    keyPh: 'کلید {name}',
    labelPh: 'اسمی برایش بگذار (اختیاری) — مثلاً «حساب کاری»',
    save: 'ذخیره',
    cancel: 'انصراف',
    formHint: 'رمزنگاری‌شده ذخیره می‌شود و هرگز در هیچ پاسخی برنمی‌گردد — فقط چند حرف اولش را می‌بینی. فراخوانی‌ای که با کلید خودت انجام شود، از اعتبار تو نزد ما کم نمی‌کند. می‌توانی تا {n} کلید برای هر سرویس بگذاری؛ اگر یکی از کار بیفتد، درخواست خودش می‌رود سراغ بعدی.',
    notWired: 'این سرویس هنوز به دروازه وصل نشده. درخواست بده تا اضافه شود.',
  },
  en: {
    title: 'Providers',
    subtitle: 'Every service this gateway knows about — and whose key you reach it with',
    f_all: 'All',
    f_live: 'Working now',
    f_mine: 'My keys',
    f_transcription: 'Transcription',
    f_chat: 'Chat',
    f_iran: 'Available in Iran',
    f_missing: 'Not connected yet',
    saved: 'Your key for {name} was saved. Keys are tried in the order you added them; if the first one fails, the next is used automatically.',
    dropped: 'Your key for {name} was removed.',
    approvedNow: 'Access to {name} on our key was granted right away.',
    requested: 'Your request for {name} was submitted and is awaiting approval.',
    subActive: (v) => (
      <>
        Your <strong>{v.name}</strong> subscription is active until <span className="ltr">{v.date}</span> — the
        services it covers are available on our own key, and their usage is deducted from your credit.
      </>
    ),
    noSecret: () => (
      <>
        This gateway has no <code className="ltr">NABUGATE_SECRET_KEY</code>, so it won’t store keys — keeping a
        key unencrypted is worse than not keeping it at all. Until it is set, send your key with each request in
        the <code className="ltr">X-Nabu-Key-&lt;provider&gt;</code> header.
      </>
    ),
    stLive: 'Live',
    stNoKey: 'No key',
    stMissing: 'Not connected',
    onOurKey: '✓ Available on our key',
    pending: '⏳ Request submitted, awaiting approval',
    denied: 'Your request was declined — you can ask again',
    needsApproval: 'Using our key needs approval',
    notOnOurKey: 'Not available on our key yet',
    planCovers: 'Your subscription unlocks this',
    myKeys: '{n} of your own keys — tried in order',
    myKey: 'Your own key is saved',
    remove: 'Remove',
    addAnother: 'Add another key',
    useMine: 'Use my own key',
    askOurKey: 'Request our key',
    askAdd: 'Request this provider',
    getKey: 'Get a key ↗',
    keyPh: '{name} key',
    labelPh: 'Give it a name (optional) — e.g. “Work account”',
    save: 'Save',
    cancel: 'Cancel',
    formHint: 'Stored encrypted and never returned in any response — you only ever see its first few characters. Calls made with your own key don’t draw on your credit with us. You can add up to {n} keys per service; if one stops working, the request moves on to the next by itself.',
    notWired: 'This service isn’t connected to the gateway yet. Request it to have it added.',
  },
};

export default function Providers() {
  const t = useT(T);
  const { lang } = useI18n();
  const caps = CAP_LABEL[lang] || CAP_LABEL.fa;
  // The catalogue carries an English label and blurb next to the Persian ones;
  // either may be absent (a provider nobody catalogued has neither), and then
  // the Persian — or the bare name — is what there is.
  const labelOf = (p) => (lang === 'en' && p.label_en) || p.label || p.name;
  const blurbOf = (p) => (lang === 'en' && p.blurb_en) || p.blurb;
  const [rows, setRows] = useState(null);
  const [canStore, setCanStore] = useState(true);
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState(null);
  const [filter, setFilter] = useState('all');
  const [editing, setEditing] = useState(null); // provider name whose key form is open
  const [draftKey, setDraftKey] = useState('');
  const [draftLabel, setDraftLabel] = useState('');
  const [busy, setBusy] = useState(null);
  const [maxKeys, setMaxKeys] = useState(8);
  const [sub, setSub] = useState(null);

  const load = () =>
    api
      .listProviders()
      .then((d) => {
        setRows(d.providers || []);
        setCanStore(d.can_store_keys !== false);
        if (d.max_keys) setMaxKeys(d.max_keys);
        setSub(d.subscribed ? d.subscription : null);
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
      await api.saveProviderKey(name, draftKey.trim(), draftLabel.trim());
      setNotice(t('saved', { name }));
      setEditing(null);
      setDraftKey('');
      setDraftLabel('');
      await load();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(null);
    }
  };

  const dropKey = async (name, id) => {
    setBusy(name);
    try {
      await api.deleteProviderKey(name, id);
      setNotice(t('dropped', { name }));
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
        r.status === 'approved' ? t('approvedNow', { name }) : t('requested', { name }),
      );
      await load();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(null);
    }
  };

  return (
    <Layout title={t('title')} subtitle={t('subtitle')}>
      {error && <div className="card banner-error">{error}</div>}
      {notice && <div className="card banner-ok">{notice}</div>}
      {sub && (
        <div className="card banner-ok">
          {t('subActive', { name: sub.name || sub.plan_id, date: fmtDate(sub.expires_at) })}
        </div>
      )}
      {!canStore && (
        <div className="card banner-warn">{t('noSecret')}</div>
      )}

      <div className="card" style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
        {FILTERS.map((id) => (
          <button
            key={id}
            className={filter === id ? 'btn btn-sm' : 'btn btn-sm btn-ghost'}
            onClick={() => setFilter(id)}
          >
            {t('f_' + id)}
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
              <VendorIcon vendor={{ ...p, label: labelOf(p) }} />
              <div style={{ minWidth: 0 }}>
                <div className="prov-name">{labelOf(p)}</div>
                <div className="prov-slug ltr">{p.name}</div>
              </div>
              <span className={p.live ? 'tag tag-ok' : p.configured ? 'tag' : 'tag tag-muted'}>
                {p.live ? t('stLive') : p.configured ? t('stNoKey') : t('stMissing')}
              </span>
            </div>

            {blurbOf(p) && <p className="prov-blurb">{blurbOf(p)}</p>}

            <div className="prov-caps">
              {(p.capabilities || []).map((c) => (
                <span className="tag tag-cap" key={c}>{caps[c] || c}</span>
              ))}
              {p.iran && <span className="tag tag-cap">{t('f_iran')}</span>}
            </div>

            <div className="prov-state">
              {p.uses_gateway_key ? (
                <span className="prov-ok">{t('onOurKey')}</span>
              ) : p.grant === 'pending' ? (
                <span className="prov-wait">{t('pending')}</span>
              ) : p.grant === 'denied' ? (
                <span className="prov-no">{t('denied')}</span>
              ) : p.live ? (
                <span className="prov-no">{t('needsApproval')}</span>
              ) : (
                <span className="prov-no">{t('notOnOurKey')}</span>
              )}
              {p.plan_covers && !p.uses_gateway_key && (
                <span className="prov-ok">{t('planCovers')}</span>
              )}
              {p.have_key && (
                <span className="prov-mine">
                  {p.keys.length > 1 ? t('myKeys', { n: fmtInt(p.keys.length) }) : t('myKey')}
                </span>
              )}
            </div>

            {p.have_key && (
              <ol className="prov-keys">
                {p.keys.map((k, i) => (
                  <li key={k.id}>
                    <span className="prov-key-n">{fmtInt(i + 1)}</span>
                    <code className="ltr">{k.prefix}</code>
                    {k.label && <span className="prov-key-label">{k.label}</span>}
                    <button
                      className="btn btn-sm btn-ghost"
                      disabled={busy === p.name}
                      onClick={() => dropKey(p.name, k.id)}
                    >
                      {t('remove')}
                    </button>
                  </li>
                ))}
              </ol>
            )}

            <div className="prov-actions">
              {p.byok && canStore && editing !== p.name && (p.keys?.length || 0) < maxKeys && (
                <button
                  className="btn btn-sm"
                  onClick={() => { setEditing(p.name); setDraftKey(''); setDraftLabel(''); }}
                >
                  {p.have_key ? t('addAnother') : t('useMine')}
                </button>
              )}
              {!p.uses_gateway_key && p.grant !== 'pending' && (
                <button className="btn btn-sm btn-ghost" disabled={busy === p.name} onClick={() => ask(p.name)}>
                  {p.configured ? t('askOurKey') : t('askAdd')}
                </button>
              )}
              {p.keys_url && (
                <a className="btn btn-sm btn-ghost ltr" href={p.keys_url} target="_blank" rel="noreferrer">
                  {t('getKey')}
                </a>
              )}
            </div>

            {editing === p.name && (
              <div className="prov-form">
                <input
                  className="input ltr"
                  type="password"
                  autoComplete="off"
                  placeholder={t('keyPh', { name: labelOf(p) })}
                  value={draftKey}
                  onChange={(e) => setDraftKey(e.target.value)}
                />
                <input
                  className="input"
                  type="text"
                  autoComplete="off"
                  placeholder={t('labelPh')}
                  value={draftLabel}
                  onChange={(e) => setDraftLabel(e.target.value)}
                />
                <button className="btn btn-sm" disabled={!draftKey.trim() || busy === p.name} onClick={() => saveKey(p.name)}>
                  {t('save')}
                </button>
                <button className="btn btn-sm btn-ghost" onClick={() => setEditing(null)}>{t('cancel')}</button>
                <p className="prov-hint">{t('formHint', { n: fmtInt(maxKeys) })}</p>
              </div>
            )}
            {!p.configured && !p.keys_url && (
              <p className="prov-hint">{t('notWired')}</p>
            )}
          </div>
        ))}
      </div>
    </Layout>
  );
}
