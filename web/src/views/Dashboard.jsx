import Layout from '../components/Layout.jsx';
import { stats, providers, usageByProject, health, faDigits, faInt } from '../data/mock.js';

function ProviderTile({ p }) {
  return (
    <div className={'prov' + (p.on ? '' : ' off')}>
      <div style={{ minWidth: 0 }}>
        <div className="name">
          <strong>{p.name}</strong>
          <span className={'ptag' + (p.tagKind === 'default' ? ' default' : p.tagKind === 'pass' ? ' pass' : '')}>
            {p.tag}
          </span>
        </div>
        <div className={'url' + (p.urlLtr ? ' ltr' : '')} dir={p.urlLtr || /[a-z]/.test(p.url) ? 'ltr' : undefined}>
          {p.url}
        </div>
      </div>
      <span className={'dot ' + (p.on ? 'dot-ok' : 'dot-idle')} aria-hidden="true" />
    </div>
  );
}

export default function Dashboard() {
  const active = providers.filter((p) => p.on).length;
  return (
    <Layout
      title="داشبورد"
      subtitle="نمای کلی دروازه، پرووایدرها و مصرف"
      actions={
        <>
          <span className="badge badge-ok">آنلاین</span>
          <span className="pill ltr">v1 · OpenAI-compatible</span>
        </>
      }
    >
      <div className="stat-grid">
        {stats.map((s) => (
          <div className="stat" key={s.label}>
            <div className="label">{s.label}</div>
            <div className={'value' + (s.tone ? ' ' + s.tone : '')} dir={s.ltr ? 'ltr' : undefined}>
              {s.value}
            </div>
          </div>
        ))}
      </div>

      <div className="grid grid-2-wide">
        <div className="card">
          <div className="card-head">
            <h3>پرووایدرها</h3>
            <span className="pill">
              {active} فعال · {providers.length - active} خاموش
            </span>
          </div>
          <div className="prov-grid">
            {providers.map((p) => (
              <ProviderTile key={p.name} p={p} />
            ))}
          </div>
        </div>

        <div className="col">
          <div className="card">
            <h3 style={{ marginBottom: 12 }}>مصرف بر اساس پروژه</h3>
            <div className="rows">
              {usageByProject.map((u, i) => (
                <div key={u.project}>
                  {i > 0 && <div className="hr" style={{ marginBottom: 10 }} />}
                  <div className="row">
                    <div>
                      <div className="k">{u.project}</div>
                      <div className="k-sub">{faInt(u.requests)} درخواست</div>
                    </div>
                    <strong className="v ltr">{faDigits(u.cost)}</strong>
                  </div>
                </div>
              ))}
            </div>
          </div>

          <div className="card">
            <h3 style={{ marginBottom: 12 }}>سلامت سرویس</h3>
            <div className="kv">
              {health.map((h) => (
                <div className="line" key={h.k}>
                  <span>{h.k}</span>
                  <strong className={h.warn ? 'warn' : ''} dir={h.ltr ? 'ltr' : undefined}>
                    {faDigits(h.v)}
                  </strong>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </Layout>
  );
}
