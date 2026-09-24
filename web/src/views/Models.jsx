import { Fragment, useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { SkeletonCards } from '../components/Skeleton.jsx';
import Icon from '../components/Icon.jsx';
import { fmtInt, useT } from '../i18n/index.jsx';

const T = {
  fa: {
    title: 'مدل‌ها و آلیاس‌ها',
    subtitle: 'کاتالوگ مدل‌های فعال و در دسترس در NabuGate',
    aliasesTitle: 'مدل‌های پایه (Aliases)',
    aliasesIntro: 'نبوگیت به جای دسترسی مستقیم به پروایدرها، مدل‌ها را در قالب «آلیاس» (Alias) ارائه می‌کند تا در صورت قطعی هر سرویس دهنده، به صورت خودکار مدل جایگزین (Fallback) استفاده شود. شما در کد خود این نام‌ها را صدا می‌زنید.',
    noAliases: 'هیچ aliasی تعریف نشده است.',
    activeProviders: '{n} ارائه‌دهنده فعال',
    connected: 'متصل',
    routing: 'مسیردهی مدل‌ها:',
    agentsTitle: 'عامل‌های هوشمند (Sub-agents)',
    agentsIntro: 'ساب‌اجنت‌ها (Sub-agents) پرامپت‌ها و پارامترهای از پیش تعریف شده‌ای هستند که روی یکی از آلیاس‌ها سوار می‌شوند. با صدا زدن نام ساب‌اجنت به عنوان Model، شما نیازی به تنظیم پرامپت سیستم در سمت کلاینت نخواهید داشت.',
    noAgents: 'ساب‌اجنتی یافت نشد.',
    agentReady: 'ساب‌اجنت آماده',
    agentNote: 'سیستم پرامپت و پارامترهای پیش‌فرض این اجنت در بک‌اند دروازه (Gateway) نگهداری می‌شود.',
  },
  en: {
    title: 'Models & aliases',
    subtitle: 'The models active and available on NabuGate',
    aliasesTitle: 'Base models (aliases)',
    aliasesIntro: 'Rather than exposing providers directly, NabuGate offers models as aliases, so that when a provider goes down a fallback model takes over automatically. These are the names you call from your code.',
    noAliases: 'No aliases are defined.',
    activeProviders: '{n} active providers',
    connected: 'Connected',
    routing: 'Model routing:',
    agentsTitle: 'Sub-agents',
    agentsIntro: 'Sub-agents are predefined prompts and parameters layered on top of one of the aliases. Pass a sub-agent’s name as the model and you don’t need to set a system prompt on the client.',
    noAgents: 'No sub-agents found.',
    agentReady: 'Sub-agent ready',
    agentNote: 'This agent’s system prompt and default parameters are kept in the gateway’s backend.',
  },
};

export default function Models() {
  const t = useT(T);
  const [data, setData] = useState(null);
  const [error, setError] = useState(null);

  useEffect(() => {
    api.overview().then(setData).catch((e) => setError(e.message));
  }, []);

  const aliases = data?.aliases || [];
  const agents = data?.agents || [];
  
  const getModelColor = (name) => {
    const l = name.toLowerCase();
    if (l.includes('gpt')) return { bg: 'rgba(16, 185, 129, 0.1)', color: '#10b981', border: 'rgba(16, 185, 129, 0.3)' };
    if (l.includes('claude')) return { bg: 'rgba(249, 115, 22, 0.1)', color: '#f97316', border: 'rgba(249, 115, 22, 0.3)' };
    if (l.includes('gemini')) return { bg: 'rgba(59, 130, 246, 0.1)', color: '#3b82f6', border: 'rgba(59, 130, 246, 0.3)' };
    if (l.includes('deepseek')) return { bg: 'rgba(99, 102, 241, 0.1)', color: '#6366f1', border: 'rgba(99, 102, 241, 0.3)' };
    return { bg: 'var(--ng-accent-soft)', color: 'var(--ng-accent)', border: 'var(--ng-accent-border)' };
  };

  const getModelIcon = (name) => {
    const l = name.toLowerCase();
    if (l.includes('gpt')) return <svg viewBox="0 0 24 24" width="20" height="20" xmlns="http://www.w3.org/2000/svg"><g><circle cx="12" cy="12" r="2.4" fill="currentColor"></circle><circle cx="12" cy="4.5" r="1.8" fill="currentColor"></circle><circle cx="5.5" cy="16" r="1.8" fill="currentColor"></circle><circle cx="18.5" cy="16" r="1.8" fill="currentColor"></circle><path d="M12 6.3v3.3M10.4 13.4 7 15M13.6 13.4 17 15" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" fill="none"></path></g></svg>;
    if (l.includes('claude')) return <svg viewBox="0 0 24 24" width="20" height="20" xmlns="http://www.w3.org/2000/svg"><g stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" fill="none"><path d="M12 3v18M5 6.5l14 11M19 6.5l-14 11"></path></g></svg>;
    if (l.includes('gemini')) return <svg viewBox="0 0 24 24" width="20" height="20" xmlns="http://www.w3.org/2000/svg"><g><path d="M12 3c.4 4.6 1.4 5.6 6 6-4.6.4-5.6 1.4-6 6-.4-4.6-1.4-5.6-6-6 4.6-.4 5.6-1.4 6-6Z" fill="currentColor"></path><path d="M18.5 14.5c.2 2 .6 2.4 2.5 2.6-1.9.2-2.3.6-2.5 2.5-.2-1.9-.6-2.3-2.5-2.5 1.9-.2 2.3-.6 2.5-2.6Z" fill="currentColor" fillOpacity="0.8"></path></g></svg>;
    return <svg viewBox="0 0 24 24" width="20" height="20" xmlns="http://www.w3.org/2000/svg"><g stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" fill="none"><circle cx="12" cy="12" r="6.5"></circle><path d="M5.5 12a6.5 6.5 0 0 0 11 4.6" strokeOpacity="0.5"></path><circle cx="17.4" cy="9" r="1.7" fill="currentColor" stroke="none"></circle></g></svg>;
  };

  return (
    <Layout title={t('title')} subtitle={t('subtitle')}>
      {error && <div className="card banner-error">{error}</div>}

      <div style={{ marginBottom: 32 }}>
        <h3 style={{ fontSize: 18, marginBottom: 8, color: 'var(--ng-heading)' }}>{t('aliasesTitle')}</h3>
        <p style={{ color: 'var(--ng-muted)', fontSize: 13, marginBottom: 20 }}>{t('aliasesIntro')}</p>

        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(min(280px, 100%), 1fr))', gap: 16 }}>
          {data === null && <div style={{ gridColumn: '1 / -1' }}><SkeletonCards n={6} h={110} /></div>}
          {data !== null && aliases.length === 0 && <div className="card" style={{ padding: 24, textAlign: 'center', color: 'var(--ng-muted)', gridColumn: '1 / -1' }}>{t('noAliases')}</div>}
          {aliases.map((a) => {
            const style = getModelColor(a.id);
            return (
              <div key={a.id} className="card" style={{ padding: 20, display: 'flex', flexDirection: 'column', gap: 16, border: `1px solid ${style.border}` }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                  <div style={{ width: 40, height: 40, borderRadius: 10, background: style.bg, color: style.color, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                    {getModelIcon(a.id)}
                  </div>
                  <div style={{ flex: 1, overflow: 'hidden' }}>
                    <div style={{ fontWeight: 700, fontSize: 16, color: 'var(--ng-heading)', whiteSpace: 'nowrap', textOverflow: 'ellipsis', overflow: 'hidden' }} dir="ltr">{a.id}</div>
                    <div style={{ fontSize: 12, color: style.color, marginTop: 4 }}>
                      {a.targets ? t('activeProviders', { n: fmtInt(a.targets.length) }) : t('connected')}
                    </div>
                  </div>
                </div>
                {a.targets && a.targets.length > 0 && (
                  <div style={{ fontSize: 12, color: 'var(--ng-muted)', background: 'var(--ng-surface)', padding: '8px 12px', borderRadius: 6, display: 'flex', flexDirection: 'column', gap: 4 }}>
                    <span style={{ fontSize: 11, fontWeight: 'bold' }}>{t('routing')}</span>
                    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                      {/* The arrow sits between the chips, not inside the LTR
                          model id, so it points at the next hop in either
                          direction. */}
                      {a.targets.map((tg, i) => (
                        <Fragment key={i}>
                          <span dir="ltr" style={{ fontSize: 11 }}>{tg.model || 'default'}</span>
                          {i < a.targets.length - 1 && (
                            <Icon name="arrow" size={11} className="flip-rtl" style={{ alignSelf: 'center' }} />
                          )}
                        </Fragment>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            );
          })}
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
    </Layout>
  );
}
