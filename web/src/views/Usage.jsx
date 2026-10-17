import { useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { fmtInt, usd, useT } from '../i18n/index.jsx';
import Icon from '../components/Icon.jsx';

const T = {
  fa: {
    title: 'مصرف',
    subtitle: 'توکن، درخواست و هزینه از آخرین بازنشانی شمارنده‌ها.',
    outTokens: 'توکن خروجی',
    inTokens: 'توکن ورودی',
    totalTokens: 'کل توکن',
    providerCost: 'هزینهٔ ارائه‌دهنده',
    providerCostHint: 'آنچه ارائه‌دهنده‌ها با قیمت رسمی برای همین درخواست‌ها از دروازه گرفته‌اند. ثبتش از این نسخه شروع شده است.',
    charged: 'کسرشده از اعتبار',
    requests: 'درخواست‌ها',
    byProvider: 'مصرف به تفکیک ارائه دهنده',
    byModel: 'مصرف به تفکیک مدل',
    byKey: 'مصرف به تفکیک کلید API',
    notFound: 'موردی یافت نشد',
    provider: 'پروایدر',
    model: 'مدل',
    tokensTotal: 'توکن کل',
    cost: 'هزینه',
    key: 'کلید',
    emptyKeys: 'مصرفی برای کلیدها ثبت نشده است',
    emptyKeysHint: 'پس از اولین استفاده از کلیدها، آمار مصرف اینجا نمایش داده می‌شود.',
    unpricedBadge: 'بی‌قیمت',
    unpricedBadgeHint: 'برخی درخواست‌های این مدل بدون قیمت در پیکربندی سرو شده‌اند: مصرف ثبت شده ولی چیزی بابتش وصول نشده. نرخ مدل را در pricing اضافه کنید.',
    estimatedBadge: 'تخمینی',
    estimatedBadgeHint: 'بخشی از شمارش توکن‌های این مدل تخمین خودِ دروازه است چون ارائه‌دهنده مصرفی گزارش نکرده بود.',
    audioSec: 'ثانیهٔ صدا',
    imagesUnit: 'تصویر',
    charsUnit: 'کاراکتر',
    exportCsv: 'خروجی CSV',
    exportCsvHint: 'دانلود جدول مصرف هر مدل با هر دو هزینه (کسرشده و هزینهٔ ارائه‌دهنده).',
  },
  en: {
    title: 'Usage',
    subtitle: 'Tokens, requests and cost since the counters were last reset.',
    outTokens: 'Output tokens',
    inTokens: 'Input tokens',
    totalTokens: 'Total tokens',
    providerCost: 'Provider cost',
    providerCostHint: 'What the providers charged the gateway for these requests, at list price. Recorded from this version on.',
    charged: 'Charged to balances',
    requests: 'Requests',
    byProvider: 'Usage by provider',
    byModel: 'Usage by model',
    byKey: 'Usage by API key',
    notFound: 'Nothing found',
    provider: 'Provider',
    model: 'Model',
    tokensTotal: 'Total tokens',
    cost: 'Cost',
    key: 'Key',
    emptyKeys: 'No key usage recorded yet',
    emptyKeysHint: 'Usage stats appear here after your keys are first used.',
    unpricedBadge: 'Unpriced',
    unpricedBadgeHint: 'Some calls on this model were served with no price in the config: usage was recorded but nothing was billed for it. Add the model\'s rate under pricing.',
    estimatedBadge: 'Estimated',
    estimatedBadgeHint: 'Part of this model\'s token counts is the gateway\'s own estimate, because the upstream reported no usage.',
    audioSec: 'Audio seconds',
    imagesUnit: 'Images',
    charsUnit: 'Characters',
    exportCsv: 'Export CSV',
    exportCsvHint: 'Download the per-model usage table with both costs (charged and provider cost).',
  },
};

export default function Usage() {
  const t = useT(T);
  const [byProject, setByProject] = useState({});
  const [byModel, setByModel] = useState({});
  const [byProvider, setByProvider] = useState({});
  // Only an administrator sees the whole deployment, and only for the whole
  // deployment does what the vendors charged the gateway mean anything.
  const [scoped, setScoped] = useState(false);
  const [error, setError] = useState(null);

  const load = () =>
    api
      .usage()
      .then((r) => {
        setByProject(r.by_project || {});
        setByModel(r.by_model || {});
        setByProvider(r.by_provider || {});
        setScoped(Boolean(r.scoped));
      })
      .catch((e) => setError(e.message));

  // Braces, not useEffect(load): load returns its promise, and whatever an
  // effect returns React calls as the cleanup — so leaving this page threw
  // "is not a function" and took the whole console down with it.
  useEffect(() => { load(); }, []);

  const rows = Object.entries(byProject).sort((a, b) => (b[1].requests || 0) - (a[1].requests || 0));
  const modelRows = Object.entries(byModel).sort((a, b) => (b[1].requests || 0) - (a[1].requests || 0));
  const provRows = Object.entries(byProvider).sort((a, b) => (b[1].requests || 0) - (a[1].requests || 0));
  
  const total = rows.reduce(
    (acc, [, v]) => ({
      requests: acc.requests + (v.requests || 0),
      prompt_tokens: acc.prompt_tokens + (v.prompt_tokens || 0),
      completion_tokens: acc.completion_tokens + (v.completion_tokens || 0),
      cost: acc.cost + (v.cost_usd || 0),
      providerCost: acc.providerCost + (v.provider_cost_usd || 0),
    }),
    { requests: 0, prompt_tokens: 0, completion_tokens: 0, cost: 0, providerCost: 0 }
  );

  const totalTokens = total.prompt_tokens + total.completion_tokens;

  // A row's honesty badges: a model nobody priced reads as zero billed unless
  // it is called out, and a guessed token count should never pass for a
  // vendor's number.
  const badges = (v) => (
    <>
      {(v.unpriced_requests > 0) && <span className="badge badge-fail" title={t('unpricedBadgeHint')}>{t('unpricedBadge')}</span>}
      {(v.estimated_requests > 0) && <span className="badge badge-muted" title={t('estimatedBadgeHint')}>{t('estimatedBadge')}</span>}
    </>
  );

  // The physical units a non-token call consumed, shown only when the row
  // actually carries them — a transcribe row speaks in seconds, an image row
  // in images, a speech row in characters.
  const units = (v) => (
    <>
      {(v.audio_seconds > 0) && <span className="mono" title={t('audioSec')}><Icon name="clock" size={12} /> {fmtInt(Math.round(v.audio_seconds))}s</span>}
      {(v.images > 0) && <span className="mono" title={t('imagesUnit')}><Icon name="image" size={12} /> {fmtInt(v.images)}</span>}
      {(v.characters > 0) && <span className="mono" title={t('charsUnit')}><Icon name="type" size={12} /> {fmtInt(v.characters)}</span>}
    </>
  );

  return (
    <Layout title={t('title')} subtitle={t('subtitle')}>
      {error && <div className="card banner-error">{error}</div>}

      {/* The CSV export is admin-only server-side (requireAdmin), so the link
          only shows to the caller who can actually use it — a non-admin's
          scoped view has no deployment-wide table to export. */}
      {!scoped && (
        <div style={{ marginBottom: 12 }}>
          <a className="btn btn-secondary" href="/api/usage/export" download title={t('exportCsvHint')}>
            {t('exportCsv')}
          </a>
        </div>
      )}

      {/* Six totals, as the same KPI tiles the dashboard uses so the two
          screens read as one console, and two to a row on a phone. */}
      <div className="grid-auto kpi-grid stagger">
        {[
          { label: t('outTokens'), value: fmtInt(total.completion_tokens), icon: 'arrowUp', tone: 'ok' },
          { label: t('inTokens'), value: fmtInt(total.prompt_tokens), icon: 'arrowDown' },
          { label: t('totalTokens'), value: fmtInt(totalTokens), icon: 'layers', tone: 'pass' },
          { label: t('charged'), value: usd(total.cost), icon: 'card', tone: 'warn', ltr: true },
          // What a call cost the gateway sat under this label, and it was the
          // charged amount — a plan's markup included — under another name.
          !scoped && { label: t('providerCost'), value: usd(total.providerCost), icon: 'receipt', ltr: true, hint: t('providerCostHint') },
          { label: t('requests'), value: fmtInt(total.requests), icon: 'zap' },
        ].filter(Boolean).map((k) => (
          <div key={k.icon} className="card kpi" title={k.hint}>
            <div className="kpi-label">
              <span className={'kpi-icon' + (k.tone ? ' ' + k.tone : '')}><Icon name={k.icon} size={18} /></span>
              {k.label}
            </div>
            <div className={'kpi-value' + (k.ltr ? ' ltr' : '')}>{k.value}</div>
          </div>
        ))}
      </div>

      {/* Model & Provider Stats */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(min(300px, 100%), 1fr))', gap: 24, marginBottom: 24 }}>
        <div className="card" style={{ padding: 0, display: 'flex', flexDirection: 'column' }}>
          <div style={{ padding: 24, borderBottom: '1px solid var(--ng-border)' }}>
            <h3 style={{ fontSize: 14, margin: 0 }}>{t('byProvider')}</h3>
          </div>
          {provRows.length === 0 ? (
            <div style={{ flex: 1, padding: 32, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center' }}>
              <div style={{ fontSize: 32, marginBottom: 12, color: 'var(--ng-border)' }}><Icon name="inbox" size={32} /></div>
              <div style={{ fontWeight: 700, fontSize: 14, marginBottom: 8 }}>{t('notFound')}</div>
            </div>
          ) : (
            <table className="tbl" style={{ border: 'none', margin: 0 }}>
              <thead><tr><th>{t('provider')}</th><th>{t('requests')}</th><th>{t('tokensTotal')}</th><th>{t('providerCost')}</th><th>{t('cost')}</th></tr></thead>
              <tbody>
                {provRows.map(([name, v]) => (
                  <tr key={name}>
                    <td style={{ fontWeight: 700, color: 'var(--ng-heading)' }} dir="ltr">{name} {badges(v)}</td>
                    <td className="mono">{fmtInt(v.requests || 0)}</td>
                    <td className="mono">{fmtInt((v.prompt_tokens || 0) + (v.completion_tokens || 0))}</td>
                    <td className="mono ltr">{usd(v.provider_cost_usd || 0)}</td>
                    <td className="mono ltr">{usd(v.cost_usd)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>

        <div className="card" style={{ padding: 0, display: 'flex', flexDirection: 'column' }}>
          <div style={{ padding: 24, borderBottom: '1px solid var(--ng-border)' }}>
            <h3 style={{ fontSize: 14, margin: 0 }}>{t('byModel')}</h3>
          </div>
          {modelRows.length === 0 ? (
            <div style={{ flex: 1, padding: 32, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center' }}>
              <div style={{ fontSize: 32, marginBottom: 12, color: 'var(--ng-border)' }}><Icon name="inbox" size={32} /></div>
              <div style={{ fontWeight: 700, fontSize: 14, marginBottom: 8 }}>{t('notFound')}</div>
            </div>
          ) : (
            <table className="tbl" style={{ border: 'none', margin: 0 }}>
              <thead><tr><th>{t('model')}</th><th>{t('requests')}</th><th>{t('tokensTotal')}</th><th></th><th>{t('providerCost')}</th><th>{t('cost')}</th></tr></thead>
              <tbody>
                {modelRows.map(([name, v]) => (
                  <tr key={name}>
                    <td style={{ fontWeight: 700, color: 'var(--ng-heading)' }} dir="ltr">
                      {name} {badges(v)}
                    </td>
                    <td className="mono">{fmtInt(v.requests || 0)}</td>
                    <td className="mono">{fmtInt((v.prompt_tokens || 0) + (v.completion_tokens || 0))}</td>
                    <td style={{ whiteSpace: 'nowrap', display: 'grid', gap: 2 }}>{units(v)}</td>
                    <td className="mono ltr">{usd(v.provider_cost_usd || 0)}</td>
                    <td className="mono ltr">{usd(v.cost_usd)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>

      {/* API Key Usage Table */}
      <div className="card" style={{ padding: 0 }}>
        <div style={{ padding: 24, borderBottom: '1px solid var(--ng-border)' }}>
          <h3 style={{ fontSize: 14, margin: 0 }}>{t('byKey')}</h3>
        </div>
        <table className="tbl" style={{ border: 'none' }}>
          <thead>
            <tr>
              <th>{t('key')}</th>
              <th>{t('outTokens')}</th>
              <th>{t('inTokens')}</th>
              <th>{t('requests')}</th>
              <th>{t('charged')}</th>
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 && (
              <tr>
                <td colSpan={5} style={{ padding: '60px 24px', textAlign: 'center' }}>
                  <div style={{ fontSize: 32, marginBottom: 12, color: 'var(--ng-border)' }}><Icon name="inbox" size={32} /></div>
                  <div style={{ fontWeight: 700, fontSize: 14, marginBottom: 8 }}>{t('emptyKeys')}</div>
                  <p style={{ color: 'var(--ng-muted)', fontSize: 12 }}>{t('emptyKeysHint')}</p>
                </td>
              </tr>
            )}
            {rows.map(([name, v]) => (
              <tr key={name}>
                <td style={{ fontWeight: 700, color: 'var(--ng-heading)' }}>{name}</td>
                <td className="mono">{fmtInt(v.completion_tokens || 0)}</td>
                <td className="mono">{fmtInt(v.prompt_tokens || 0)}</td>
                <td className="mono">{fmtInt(v.requests || 0)}</td>
                <td className="mono ltr">{usd(v.cost_usd)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Layout>
  );
}
