import { useEffect, useMemo, useState } from 'react';
import Layout from '../components/Layout.jsx';
import CodeBlock from '../components/CodeBlock.jsx';
import * as api from '../api.js';
import { useI18n, useT } from '../i18n/index.jsx';
import { gatewayBase } from '../origin.js';

/*
 * Templates & skills.
 *
 * Templates are ready tasks with blanks to fill (POST /v1/templates/{name}/run);
 * skills are instruction bundles any chat request can switch on with
 * "nabu_skills". Everyone sees both and gets the exact request for their own
 * key; an admin can also run a template here as a preview.
 */
const T = {
  fa: {
    title: 'تمپلیت‌ها و اسکیل‌ها',
    subtitle: 'کارهای آماده با چند خانهٔ پرکردنی، و مهارت‌هایی که روی هر درخواست چت روشن می‌شوند',
    tabTemplates: 'تمپلیت‌ها ({n})',
    tabSkills: 'اسکیل‌ها ({n})',
    all: 'همه',
    search: 'جستجو…',
    model: 'مدل',
    skills: 'اسکیل‌ها',
    run: 'اجرای آزمایشی',
    running: 'در حال اجرا…',
    required: 'الزامی',
    output: 'خروجی',
    via: 'از {provider} · {model}',
    apiCall: 'فراخوانی با API (با کلید خودتان)',
    chatUse: 'استفاده در هر درخواست چت',
    pick: 'یک تمپلیت انتخاب کنید.',
    adminOnly: 'اجرای آزمایشی در کنسول فقط برای مدیر است؛ با کلید API خودتان همین درخواست را بفرستید.',
    json: 'خروجی JSON',
    empty: 'هنوز تمپلیتی بارگذاری نشده است.',
    cat: { marketing: 'بازاریابی', support: 'پشتیبانی', language: 'زبان', engineering: 'برنامه‌نویسی', business: 'کسب‌وکار', data: 'داده', writing: 'نوشتن', style: 'سبک', safety: 'ایمنی' },
  },
  en: {
    title: 'Templates & skills',
    subtitle: 'Ready tasks with a few blanks to fill, and skills you can switch on for any chat request',
    tabTemplates: 'Templates ({n})',
    tabSkills: 'Skills ({n})',
    all: 'All',
    search: 'Search…',
    model: 'Model',
    skills: 'Skills',
    run: 'Test run',
    running: 'Running…',
    required: 'required',
    output: 'Output',
    via: 'via {provider} · {model}',
    apiCall: 'Call it from the API (with your own key)',
    chatUse: 'Use it on any chat request',
    pick: 'Pick a template.',
    adminOnly: 'Test runs in the console are admin-only; send this same request with your own API key.',
    json: 'JSON output',
    empty: 'No templates loaded yet.',
    cat: { marketing: 'Marketing', support: 'Support', language: 'Language', engineering: 'Engineering', business: 'Business', data: 'Data', writing: 'Writing', style: 'Style', safety: 'Safety' },
  },
};

const pick = (text, lang) => (lang === 'fa' ? text?.fa || text?.en : text?.en || text?.fa) || '';

export default function Templates({ admin }) {
  const t = useT(T);
  const { lang } = useI18n();
  const L = T[lang] || T.fa;
  const [data, setData] = useState({ templates: [], skills: [] });
  const [error, setError] = useState('');
  const [tab, setTab] = useState('templates');
  const [cat, setCat] = useState('');
  const [q, setQ] = useState('');
  const [sel, setSel] = useState(null);
  const [vars, setVars] = useState({});
  const [out, setOut] = useState(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api.listPresets().then((d) => {
      setData({ templates: d.templates || [], skills: d.skills || [] });
      if ((d.templates || []).length) choose(d.templates[0]);
    }).catch((e) => setError(e.message));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function choose(tpl) {
    setSel(tpl);
    setOut(null);
    const v = {};
    for (const x of tpl.variables || []) v[x.name] = x.default || '';
    setVars(v);
  }

  const items = tab === 'templates' ? data.templates : data.skills;
  const cats = useMemo(() => [...new Set(items.map((x) => x.category).filter(Boolean))], [items]);
  const shown = items.filter((x) => (!cat || x.category === cat) &&
    (!q || `${x.name} ${pick(x.title, lang)} ${pick(x.description, lang)}`.toLowerCase().includes(q.toLowerCase())));

  async function run() {
    if (!sel) return;
    setBusy(true);
    setOut(null);
    try {
      setOut(await api.testTemplate(sel.name, { variables: vars }));
    } catch (e) {
      setOut({ error: e.message });
    } finally {
      setBusy(false);
    }
  }

  const base = gatewayBase();
  const filled = Object.fromEntries(Object.entries(vars).filter(([, v]) => v !== ''));
  const curl = sel ? `curl ${base}/templates/${sel.name}/run \\
  -H "Authorization: Bearer $NABU_KEY" \\
  -H "Content-Type: application/json" \\
  -d '${JSON.stringify({ variables: filled }, null, 0).replace(/'/g, "'\\''")}'` : '';
  const skillCurl = (name) => `curl ${base}/chat/completions \\
  -H "Authorization: Bearer $NABU_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"nabu-smart","nabu_skills":["${name}"],"messages":[{"role":"user","content":"..."}]}'`;

  const missing = sel ? (sel.variables || []).some((x) => x.required && !String(vars[x.name] || '').trim()) : true;

  return (
    <Layout title={t('title')} subtitle={t('subtitle')}>
      {error && <div className="alert alert-error">{error}</div>}
      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: 16 }}>
        <button type="button" className={`btn ${tab === 'templates' ? 'btn-primary' : 'btn-secondary'}`} onClick={() => { setTab('templates'); setCat(''); }}>
          {t('tabTemplates', { n: data.templates.length })}
        </button>
        <button type="button" className={`btn ${tab === 'skills' ? 'btn-primary' : 'btn-secondary'}`} onClick={() => { setTab('skills'); setCat(''); }}>
          {t('tabSkills', { n: data.skills.length })}
        </button>
        <input className="input" style={{ maxWidth: 240 }} placeholder={t('search')} value={q} onChange={(e) => setQ(e.target.value)} />
      </div>
      <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginBottom: 16 }}>
        <button type="button" className={`btn btn-ghost${!cat ? ' active' : ''}`} onClick={() => setCat('')}>{t('all')}</button>
        {cats.map((c) => (
          <button key={c} type="button" className={`btn btn-ghost${cat === c ? ' active' : ''}`} onClick={() => setCat(c)}>{L.cat[c] || c}</button>
        ))}
      </div>

      {tab === 'skills' ? (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))', gap: 14 }}>
          {shown.map((s) => (
            <div key={s.name} className="card">
              <div className="card-head"><h3 className="card-title">{pick(s.title, lang)}</h3></div>
              <p className="card-sub" style={{ lineHeight: 1.6 }}>{pick(s.description, lang)}</p>
              <p className="card-sub"><code dir="ltr">{s.name}</code>{s.response_format === 'json_object' && <> · {t('json')}</>}</p>
              <CodeBlock label={t('chatUse')} code={skillCurl(s.name)} />
            </div>
          ))}
        </div>
      ) : !data.templates.length ? (
        <div className="card"><p className="card-sub">{t('empty')}</p></div>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'minmax(220px, 320px) 1fr', gap: 16, alignItems: 'start' }} className="tpl-grid">
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {shown.map((tpl) => (
              <button key={tpl.name} type="button" onClick={() => choose(tpl)}
                className="card" style={{ textAlign: 'start', cursor: 'pointer', outline: sel?.name === tpl.name ? '2px solid var(--accent, #6366f1)' : 'none' }}>
                <strong>{pick(tpl.title, lang)}</strong>
                <span className="card-sub" style={{ display: 'block', marginTop: 4, lineHeight: 1.5 }}>{pick(tpl.description, lang)}</span>
              </button>
            ))}
          </div>
          {!sel ? <div className="card"><p className="card-sub">{t('pick')}</p></div> : (
            <div className="card" style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              <div>
                <h3 className="card-title">{pick(sel.title, lang)}</h3>
                <p className="card-sub">{t('model')}: <code dir="ltr">{sel.model}</code>
                  {sel.skills?.length ? <> · {t('skills')}: <code dir="ltr">{sel.skills.join(', ')}</code></> : null}</p>
              </div>
              {(sel.variables || []).map((v) => (
                <label key={v.name} style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                  <span>{pick(v.label, lang)}{v.required && <small className="card-sub"> ({t('required')})</small>}</span>
                  {v.options?.length ? (
                    <select className="input" value={vars[v.name] || ''} onChange={(e) => setVars({ ...vars, [v.name]: e.target.value })}>
                      {!v.required && <option value="">—</option>}
                      {v.options.map((o) => <option key={o} value={o}>{o}</option>)}
                    </select>
                  ) : v.multiline ? (
                    <textarea className="input" rows={5} dir="auto" value={vars[v.name] || ''} placeholder={pick(v.placeholder, lang)}
                      onChange={(e) => setVars({ ...vars, [v.name]: e.target.value })} />
                  ) : (
                    <input className="input" dir="auto" value={vars[v.name] || ''} placeholder={pick(v.placeholder, lang)}
                      onChange={(e) => setVars({ ...vars, [v.name]: e.target.value })} />
                  )}
                </label>
              ))}
              {admin ? (
                <div><button type="button" className="btn btn-primary" disabled={busy || missing} onClick={run}>{busy ? t('running') : t('run')}</button></div>
              ) : (
                <p className="card-sub">{t('adminOnly')}</p>
              )}
              {out && (
                <div>
                  <strong>{t('output')}</strong>
                  {out.error ? <div className="alert alert-error">{out.error}</div> : (
                    <>
                      <pre dir="auto" style={{ whiteSpace: 'pre-wrap', lineHeight: 1.7 }}>{out.content}</pre>
                      <small className="card-sub" dir="ltr">{t('via', { provider: out.provider, model: out.model })}</small>
                    </>
                  )}
                </div>
              )}
              <CodeBlock label={t('apiCall')} code={curl} />
            </div>
          )}
        </div>
      )}
    </Layout>
  );
}
