import { Fragment, useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { SkeletonCards } from '../components/Skeleton.jsx';
import Icon from '../components/Icon.jsx';
import { fmtDate, usd, useT } from '../i18n/index.jsx';

const T = {
  fa: {
    title: 'مدل‌ها و آلیاس‌ها',
    subtitle: 'هر آلیاس، مدل‌هایی که جوابش را می‌دهند، به همان ترتیبی که امتحان می‌شوند، و قیمت هر کدام.',
    aliasesTitle: 'مدل‌های پایه (Aliases)',
    aliasesIntro: 'نبوگیت به جای دسترسی مستقیم به پروایدرها، مدل‌ها را در قالب «آلیاس» (Alias) ارائه می‌کند تا در صورت قطعی هر سرویس دهنده، به صورت خودکار مدل جایگزین (Fallback) استفاده شود. شما در کد خود این نام‌ها را صدا می‌زنید.',
    noAliases: 'هیچ aliasی تعریف نشده است.',
    all: 'همه',
    kind_models: 'چت',
    kind_images: 'تصویر',
    kind_audio: 'صدا',
    kind_transcription: 'رونویسی',
    kind_embeddings: 'بردار',
    kind_decisions: 'تصمیم',
    kind_live: 'تماس زنده',
    order: 'به این ترتیب امتحان می‌شود:',
    unavailable: 'کلیدش روی این دروازه نیست',
    unpriced: 'بدون قیمت',
    unpricedHint: 'درخواستی که این مدل جواب بدهد با هزینهٔ صفر ثبت می‌شود.',
    free: 'رایگان',
    perMTokens: 'به ازای ۱M توکن (ورودی / خروجی)',
    perMinute: 'دقیقه‌ای',
    perImage: 'هر تصویر',
    perMChars: 'هر ۱M کاراکتر',
    perCredit: 'هر کردیت',
    voice: 'صدا',
    vendorDefault: 'صدای پیش‌فرض هر ارائه‌دهنده',
    changed: 'تغییر داده شده',
    changedBy: 'به دست {who}، {when}',
    change: 'تغییر',
    dialogTitle: 'تنظیم {alias}',
    primary: 'مدلِ اول',
    primaryHint: 'بقیه به ترتیب خودشان پشتِ آن می‌مانند. فقط بین مدل‌هایی که این آلیاس دارد می‌شود انتخاب کرد.',
    asShipped: '(همان که در پیکربندی است)',
    voiceHint: 'اگر محصول صدایی نفرستد همین پخش می‌شود. صدای نام‌دار برای هر ارائه‌دهنده به صدای خودش تبدیل می‌شود.',
    namedVoices: 'صداهای نام‌دار',
    customVoice: 'صدای دیگری از ارائه‌دهنده',
    customPlaceholder: 'مثلاً alloy یا Rachel',
    reset: 'برگرداندن به پیکربندی',
    cancel: 'انصراف',
    save: 'ذخیره',
    saved: '{alias} ذخیره شد و از درخواست بعدی اعمال می‌شود.',
    agentsTitle: 'عامل‌های هوشمند (Sub-agents)',
    agentsIntro: 'ساب‌اجنت‌ها (Sub-agents) پرامپت‌ها و پارامترهای از پیش تعریف شده‌ای هستند که روی یکی از آلیاس‌ها سوار می‌شوند. با صدا زدن نام ساب‌اجنت به عنوان Model، شما نیازی به تنظیم پرامپت سیستم در سمت کلاینت نخواهید داشت.',
    noAgents: 'ساب‌اجنتی یافت نشد.',
    agentReady: 'ساب‌اجنت آماده',
    agentNote: 'سیستم پرامپت و پارامترهای پیش‌فرض این اجنت در بک‌اند دروازه (Gateway) نگهداری می‌شود.',
  },
  en: {
    title: 'Models & aliases',
    subtitle: 'Each alias, the models that answer it in the order they are tried, and what each one costs.',
    aliasesTitle: 'Base models (aliases)',
    aliasesIntro: 'Rather than exposing providers directly, NabuGate offers models as aliases, so that when a provider goes down a fallback model takes over automatically. These are the names you call from your code.',
    noAliases: 'No aliases are defined.',
    all: 'All',
    kind_models: 'Chat',
    kind_images: 'Images',
    kind_audio: 'Speech',
    kind_transcription: 'Transcription',
    kind_embeddings: 'Embeddings',
    kind_decisions: 'Decisions',
    kind_live: 'Live calls',
    order: 'Tried in this order:',
    unavailable: 'no key on this gateway',
    unpriced: 'unpriced',
    unpricedHint: 'A request this model answers is recorded at no cost.',
    free: 'free',
    perMTokens: 'per 1M tokens (in / out)',
    perMinute: 'a minute',
    perImage: 'an image',
    perMChars: 'per 1M characters',
    perCredit: 'a credit',
    voice: 'Voice',
    vendorDefault: "each provider's default voice",
    changed: 'changed',
    changedBy: 'by {who}, {when}',
    change: 'Change',
    dialogTitle: 'Configure {alias}',
    primary: 'First model',
    primaryHint: 'The others stay behind it in their own order. Only this alias’s own models can be chosen.',
    asShipped: '(as configured)',
    voiceHint: 'Played when a product names no voice. A named voice becomes each provider’s own voice.',
    namedVoices: 'Named voices',
    customVoice: 'Another provider voice',
    customPlaceholder: 'e.g. alloy or Rachel',
    reset: 'Back to the configuration',
    cancel: 'Cancel',
    save: 'Save',
    saved: '{alias} saved; it applies from the next request.',
    agentsTitle: 'Sub-agents',
    agentsIntro: 'Sub-agents are predefined prompts and parameters layered on top of one of the aliases. Pass a sub-agent’s name as the model and you don’t need to set a system prompt on the client.',
    noAgents: 'No sub-agents found.',
    agentReady: 'Sub-agent ready',
    agentNote: 'This agent’s system prompt and default parameters are kept in the gateway’s backend.',
  },
};

const KIND_ORDER = ['models', 'live', 'audio', 'transcription', 'images', 'embeddings', 'decisions'];

/* A price in the units its vendor bills, e.g. "$0.15 / $0.60 per 1M tokens". */
function priceText(t, coordinate) {
  if (!coordinate.priced) return null;
  const p = coordinate.price || {};
  const parts = [];
  if (p.input || p.output) parts.push(`${usd(p.input)} / ${usd(p.output)} ${t('perMTokens')}`);
  if (p.per_minute) parts.push(`${usd(p.per_minute)} ${t('perMinute')}`);
  if (p.per_image) parts.push(`${usd(p.per_image)} ${t('perImage')}`);
  if (p.per_million_chars) parts.push(`${usd(p.per_million_chars)} ${t('perMChars')}`);
  if (p.per_credit) parts.push(`${usd(p.per_credit)} ${t('perCredit')}`);
  return parts.length ? parts.join(' · ') : t('free');
}

function AliasCard({ alias, canEdit, onEdit }) {
  const t = useT(T);
  const changed = alias.primary !== alias.config_primary || (alias.voice || '') !== (alias.config_voice || '');
  const unpriced = alias.rungs.some((r) => r.coordinates.some((c) => !c.priced));
  return (
    <div className="card" style={{ padding: 20, display: 'flex', flexDirection: 'column', gap: 14, border: `1px solid ${unpriced ? 'var(--ng-danger, #ef4444)' : 'var(--ng-border)'}` }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ fontWeight: 700, fontSize: 16, color: 'var(--ng-heading)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} dir="ltr">{alias.alias}</div>
          <div style={{ fontSize: 12, color: 'var(--ng-muted)', marginTop: 4, display: 'flex', gap: 8, flexWrap: 'wrap' }}>
            <span>{t('kind_' + alias.kind)}</span>
            {changed && <span className="badge" style={{ color: 'var(--ng-accent)' }}>{t('changed')}</span>}
          </div>
        </div>
        {canEdit && (
          <button type="button" className="btn btn-secondary btn-sm" onClick={() => onEdit(alias)}>
            <Icon name="route" size={14} />{t('change')}
          </button>
        )}
      </div>

      <div style={{ fontSize: 12, color: 'var(--ng-muted)', background: 'var(--ng-surface)', padding: '10px 12px', borderRadius: 6, display: 'flex', flexDirection: 'column', gap: 8 }}>
        <span style={{ fontSize: 11, fontWeight: 700 }}>{t('order')}</span>
        {alias.rungs.map((rung, i) => (
          <div key={rung.key + i} style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <span dir="ltr" style={{ fontWeight: i === 0 ? 700 : 500, color: i === 0 ? 'var(--ng-heading)' : 'inherit', textAlign: 'start' }}>
              {i + 1}. {rung.key}
            </span>
            {rung.coordinates.length === 0 && <span style={{ paddingInlineStart: 14 }}>{t('unavailable')}</span>}
            {rung.coordinates.map((c) => {
              const price = priceText(t, c);
              return (
                <span key={c.provider + '/' + c.model} style={{ paddingInlineStart: 14, display: 'flex', gap: 6, flexWrap: 'wrap' }}>
                  {rung.coordinates.length > 1 && <span dir="ltr">{c.provider}</span>}
                  {price ? (
                    <span className="ltr">{price}</span>
                  ) : (
                    <span title={t('unpricedHint')} style={{ color: 'var(--ng-danger, #ef4444)', fontWeight: 700 }}>
                      <Icon name="alert" size={12} /> {t('unpriced')}
                    </span>
                  )}
                </span>
              );
            })}
          </div>
        ))}
      </div>

      {alias.voiced && (
        <div style={{ fontSize: 12, display: 'flex', gap: 6, alignItems: 'center' }}>
          <Icon name="mic" size={13} />
          <span>{t('voice')}:</span>
          <b dir="ltr">{alias.voice || t('vendorDefault')}</b>
        </div>
      )}
      {changed && alias.updated_by && (
        <div style={{ fontSize: 11, color: 'var(--ng-muted)' }}>
          {t('changedBy', { who: alias.updated_by, when: fmtDate(alias.updated_at, 'datetime') })}
        </div>
      )}
    </div>
  );
}

function AliasDialog({ alias, voices, onClose, onSaved }) {
  const t = useT(T);
  const named = Object.keys(voices || {}).sort();
  const [primary, setPrimary] = useState(alias.primary);
  // '' is each provider's own default, a name from `voices:` is that name, and
  // anything else is a vendor voice typed by hand.
  const [choice, setChoice] = useState(!alias.voice ? '' : named.includes(alias.voice) ? alias.voice : '__custom');
  const [customVoice, setCustomVoice] = useState(alias.voice && !named.includes(alias.voice) ? alias.voice : '');
  const voice = choice === '__custom' ? customVoice.trim() : choice;
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  // The rungs in the config's own order, so the list does not reshuffle as the
  // choice changes.
  const configured = [alias.config_primary, ...alias.rungs.map((r) => r.key).filter((k) => k !== alias.config_primary)];

  const save = (setting) => {
    setBusy(true);
    setError(null);
    api
      .saveAlias(alias.alias, setting)
      .then((updated) => onSaved(updated))
      .catch((e) => {
        setError(e.message);
        setBusy(false);
      });
  };

  return (
    <div className="modal-backdrop" role="dialog" aria-modal="true" aria-label={t('dialogTitle', { alias: alias.alias })}>
      <div className="card modal glass-card" style={{ maxWidth: 560 }}>
        <h3 style={{ borderBottom: '1px solid var(--ng-border-faint)', paddingBottom: 16, marginBottom: 20 }}>
          {t('dialogTitle', { alias: '' })}<span dir="ltr">{alias.alias}</span>
        </h3>
        {error && <div className="card banner-error" style={{ marginBottom: 16 }}>{error}</div>}
        <form
          onSubmit={(e) => {
            e.preventDefault();
            save({ primary, voice: alias.voiced ? voice : '' });
          }}
        >
          <div style={{ marginBottom: 16 }}>
            <label className="label" htmlFor="alias-primary">{t('primary')}</label>
            <select id="alias-primary" className="input mono" dir="ltr" value={primary} onChange={(e) => setPrimary(e.target.value)}>
              {configured.map((key) => (
                <option key={key} value={key}>
                  {key} {key === alias.config_primary ? t('asShipped') : ''}
                </option>
              ))}
            </select>
            <p style={{ fontSize: 12, color: 'var(--ng-muted)', marginTop: 6 }}>{t('primaryHint')}</p>
          </div>

          {alias.voiced && (
            <div style={{ marginBottom: 20 }}>
              <label className="label" htmlFor="alias-voice">{t('voice')}</label>
              <select id="alias-voice" className="input" value={choice} onChange={(e) => setChoice(e.target.value)}>
                <option value="">{t('vendorDefault')}</option>
                <optgroup label={t('namedVoices')}>
                  {named.map((v) => (
                    <option key={v} value={v}>
                      {v} ({(voices[v] || []).join(', ')}) {v === alias.config_voice ? t('asShipped') : ''}
                    </option>
                  ))}
                </optgroup>
                <option value="__custom">{t('customVoice')}</option>
              </select>
              {choice === '__custom' && (
                <input
                  className="input mono"
                  dir="ltr"
                  style={{ marginTop: 8 }}
                  maxLength={64}
                  value={customVoice}
                  placeholder={t('customPlaceholder')}
                  onChange={(e) => setCustomVoice(e.target.value)}
                  aria-label={t('customVoice')}
                />
              )}
              <p style={{ fontSize: 12, color: 'var(--ng-muted)', marginTop: 6 }}>{t('voiceHint')}</p>
            </div>
          )}

          <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end', flexWrap: 'wrap' }}>
            <button type="button" className="btn btn-ghost" disabled={busy} onClick={() => save({ primary: '', voice: '' })}>
              {t('reset')}
            </button>
            <button type="button" className="btn btn-secondary" disabled={busy} onClick={onClose}>
              {t('cancel')}
            </button>
            <button type="submit" className="btn btn-primary" disabled={busy}>
              <Icon name="check" size={15} />{t('save')}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

export default function Models() {
  const t = useT(T);
  const [data, setData] = useState(null);
  const [agents, setAgents] = useState([]);
  const [kind, setKind] = useState('');
  const [editing, setEditing] = useState(null);
  const [notice, setNotice] = useState(null);
  const [error, setError] = useState(null);

  // Braces, not useEffect(load): a promise returned from an effect is called as
  // its cleanup, which takes the console down when the page is left.
  useEffect(() => {
    api.aliases().then(setData).catch((e) => setError(e.message));
    api.overview().then((o) => setAgents(o?.agents || [])).catch(() => {});
  }, []);

  const aliases = data?.aliases || [];
  const kinds = KIND_ORDER.filter((k) => aliases.some((a) => a.kind === k));
  const shown = aliases.filter((a) => !kind || a.kind === kind);

  const onSaved = (updated) => {
    setData((d) => ({ ...d, aliases: d.aliases.map((a) => (a.alias === updated.alias ? updated : a)) }));
    setEditing(null);
    setNotice(t('saved', { alias: updated.alias }));
  };

  return (
    <Layout title={t('title')} subtitle={t('subtitle')}>
      {error && <div className="card banner-error">{error}</div>}
      {notice && <div className="card banner-ok" role="status"><span>✓</span><span dir="auto">{notice}</span></div>}

      <div style={{ marginBottom: 32 }}>
        <h3 style={{ fontSize: 18, marginBottom: 8, color: 'var(--ng-heading)' }}>{t('aliasesTitle')}</h3>
        <p style={{ color: 'var(--ng-muted)', fontSize: 13, marginBottom: 16 }}>{t('aliasesIntro')}</p>

        {kinds.length > 1 && (
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: 16 }} role="group" aria-label={t('aliasesTitle')}>
            {['', ...kinds].map((k) => (
              <button key={k || 'all'} type="button" className={`btn btn-sm ${kind === k ? 'btn-primary' : 'btn-secondary'}`} onClick={() => setKind(k)}>
                {k ? t('kind_' + k) : t('all')}
              </button>
            ))}
          </div>
        )}

        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(min(300px, 100%), 1fr))', gap: 16 }}>
          {data === null && !error && <div style={{ gridColumn: '1 / -1' }}><SkeletonCards n={6} h={140} /></div>}
          {data !== null && aliases.length === 0 && <div className="card" style={{ padding: 24, textAlign: 'center', color: 'var(--ng-muted)', gridColumn: '1 / -1' }}>{t('noAliases')}</div>}
          {shown.map((a) => (
            <Fragment key={a.alias}>
              <AliasCard alias={a} canEdit={Boolean(data?.can_edit)} onEdit={(x) => { setNotice(null); setEditing(x); }} />
            </Fragment>
          ))}
        </div>
      </div>

      <div style={{ marginBottom: 32 }}>
        <h3 style={{ fontSize: 18, marginBottom: 8, color: 'var(--ng-heading)' }}>{t('agentsTitle')}</h3>
        <p style={{ color: 'var(--ng-muted)', fontSize: 13, marginBottom: 20 }}>{t('agentsIntro')}</p>

        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(min(280px, 100%), 1fr))', gap: 16 }}>
          {agents.length === 0 && <div className="card" style={{ padding: 24, textAlign: 'center', color: 'var(--ng-muted)' }}>{t('noAgents')}</div>}
          {agents.map((a) => (
            <div key={a} className="card" style={{ padding: 20, border: '1px solid var(--ng-border)' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12 }}>
                <div style={{ width: 40, height: 40, borderRadius: 10, background: 'rgba(139, 92, 246, 0.1)', color: '#8b5cf6', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                  <Icon name="bot" size={20} />
                </div>
                <div style={{ flex: 1, overflow: 'hidden' }}>
                  <div style={{ fontWeight: 700, fontSize: 16, color: 'var(--ng-heading)', whiteSpace: 'nowrap', textOverflow: 'ellipsis', overflow: 'hidden' }} dir="ltr">{a}</div>
                  <div style={{ fontSize: 12, color: '#8b5cf6', marginTop: 4 }}>{t('agentReady')}</div>
                </div>
              </div>
              <div style={{ fontSize: 12, color: 'var(--ng-muted)', background: 'var(--ng-surface)', padding: '8px 12px', borderRadius: 6 }}>
                {t('agentNote')}
              </div>
            </div>
          ))}
        </div>
      </div>

      {editing && <AliasDialog alias={editing} voices={data?.voices} onClose={() => setEditing(null)} onSaved={onSaved} />}
    </Layout>
  );
}
