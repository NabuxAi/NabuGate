import { useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { fmtInt, fmtDigits, usd, useT } from '../i18n/index.jsx';
import Icon from '../components/Icon.jsx';

const T = {
  fa: {
    title: 'مصرف',
    subtitle: 'تحلیل مصرف توکن، درخواست‌ها و هزینه‌ها در بازه‌ی انتخابی.',
    last30: '۳۰ روز اخیر',
    lastWeek: 'هفته اخیر',
    today: 'امروز',
    outTokens: 'توکن خروجی',
    inTokens: 'توکن ورودی',
    totalTokens: 'کل توکن',
    providerCost: 'هزینه ارائه‌دهنده',
    estCost: 'هزینه تخمینی',
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
  },
  en: {
    title: 'Usage',
    subtitle: 'Token, request and cost analytics for the selected period.',
    last30: 'Last 30 days',
    lastWeek: 'Last week',
    today: 'Today',
    outTokens: 'Output tokens',
    inTokens: 'Input tokens',
    totalTokens: 'Total tokens',
    providerCost: 'Provider cost',
    estCost: 'Estimated cost',
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
  },
};

export default function Usage() {
  const t = useT(T);
  const [byProject, setByProject] = useState({});
  const [byModel, setByModel] = useState({});
  const [byProvider, setByProvider] = useState({});
  const [error, setError] = useState(null);

  const load = () =>
    api
      .usage()
      .then((r) => {
        setByProject(r.by_project || {});
        setByModel(r.by_model || {});
        setByProvider(r.by_provider || {});
      })
      .catch((e) => setError(e.message));

  useEffect(load, []);

  const rows = Object.entries(byProject).sort((a, b) => (b[1].requests || 0) - (a[1].requests || 0));
  const modelRows = Object.entries(byModel).sort((a, b) => (b[1].requests || 0) - (a[1].requests || 0));
  const provRows = Object.entries(byProvider).sort((a, b) => (b[1].requests || 0) - (a[1].requests || 0));
  
  const total = rows.reduce(
    (acc, [, v]) => ({
      requests: acc.requests + (v.requests || 0),
      prompt_tokens: acc.prompt_tokens + (v.prompt_tokens || 0),
      completion_tokens: acc.completion_tokens + (v.completion_tokens || 0),
      cost: acc.cost + (v.cost_usd || 0),
    }),
    { requests: 0, prompt_tokens: 0, completion_tokens: 0, cost: 0 }
  );

  const totalTokens = total.prompt_tokens + total.completion_tokens;

  return (
    <Layout
      title={t('title')}
      subtitle={t('subtitle')}
      actions={
        <select className="input" style={{ width: 'auto' }} aria-label={t('title')}>
          <option>{t('last30')}</option>
          <option>{t('lastWeek')}</option>
          <option>{t('today')}</option>
        </select>
      }
    >
      {error && <div className="card banner-error">{error}</div>}

      {/* Six totals, as the same KPI tiles the dashboard uses so the two
          screens read as one console, and two to a row on a phone. */}
      <div className="grid-auto kpi-grid stagger">
        {[
          { label: t('outTokens'), value: fmtInt(total.completion_tokens), icon: 'arrowUp', tone: 'ok' },
          { label: t('inTokens'), value: fmtInt(total.prompt_tokens), icon: 'arrowDown' },
          { label: t('totalTokens'), value: fmtInt(totalTokens), icon: 'layers', tone: 'pass' },
          { label: t('providerCost'), value: '$ ' + fmtDigits(total.cost.toFixed(3)), icon: 'receipt', ltr: true },
          // This multiplied the dollar figure by a rate written into the
          // source as "dummy exchange rate for UI". A made-up number rendered
          // in تومان beside real ones reads as a real one.
          { label: t('estCost'), value: usd(total.cost), icon: 'card', tone: 'warn', ltr: true },
          { label: t('requests'), value: fmtInt(total.requests), icon: 'zap' },
        ].map((k) => (
          <div key={k.icon} className="card kpi">
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
              <thead><tr><th>{t('provider')}</th><th>{t('tokensTotal')}</th><th>{t('cost')}</th></tr></thead>
              <tbody>
                {provRows.map(([name, v]) => (
                  <tr key={name}>
                    <td style={{ fontWeight: 700, color: 'var(--ng-heading)' }} dir="ltr">{name}</td>
                    <td className="mono">{fmtInt((v.prompt_tokens || 0) + (v.completion_tokens || 0))}</td>
                    <td className="mono ltr">{fmtDigits('$' + (v.cost_usd || 0).toFixed(4))}</td>
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
              <thead><tr><th>{t('model')}</th><th>{t('tokensTotal')}</th><th>{t('cost')}</th></tr></thead>
              <tbody>
                {modelRows.map(([name, v]) => (
                  <tr key={name}>
                    <td style={{ fontWeight: 700, color: 'var(--ng-heading)' }} dir="ltr">{name}</td>
                    <td className="mono">{fmtInt((v.prompt_tokens || 0) + (v.completion_tokens || 0))}</td>
                    <td className="mono ltr">{fmtDigits('$' + (v.cost_usd || 0).toFixed(4))}</td>
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
              <th>{t('providerCost')}</th>
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
                <td className="mono ltr">{fmtDigits('$' + (v.cost_usd || 0).toFixed(4))}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Layout>
  );
}
