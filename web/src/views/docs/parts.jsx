import { useState } from 'react';
import CodeBlock from '../../components/CodeBlock.jsx';
import Icon from '../../components/Icon.jsx';
import { fmtInt, useI18n } from '../../i18n/index.jsx';

/* Building blocks shared by the Persian and English documentation. */

export function H1({ children }) {
  return <h1 className="docs-h1">{children}</h1>;
}
export function Lead({ children }) {
  return <p className="docs-lead">{children}</p>;
}
export function H2({ children }) {
  return <h2 className="docs-h2">{children}</h2>;
}

export function Steps({ items }) {
  return (
    <ol className="docs-steps stagger">
      {items.map((t, i) => (
        <li key={i} className="step">
          <span className="num">{fmtInt(i + 1)}</span>
          <div>{t}</div>
        </li>
      ))}
    </ol>
  );
}

// The content modules were written with emoji markers; they are mapped to the
// line icons here so both languages render the same glyphs on every platform.
const CALLOUT_ICON = { '🔒': 'lock', '✓': 'check', '⚠️': 'alert', '💡': 'sparkles' };

export function Callout({ kind, icon, children }) {
  const name = CALLOUT_ICON[icon] || (kind === 'warn' ? 'alert' : kind === 'ok' ? 'check' : 'info');
  return (
    <div className={'callout ' + (kind || '')} style={{ margin: '18px 0' }}>
      <span className="ci"><Icon name={name} size={18} /></span>
      <div>{children}</div>
    </div>
  );
}

export function Faq({ q, children }) {
  return <details className="faq"><summary>{q}</summary><div className="faq-body">{children}</div></details>;
}

export function EnvTabs({ BASE, KEY }) {
  const { lang } = useI18n();
  const [os, setOs] = useState('mac');
  const bash = `export OPENAI_BASE_URL="${BASE}"\nexport OPENAI_API_KEY="${KEY}"`;
  const ps = `$env:OPENAI_BASE_URL="${BASE}"\n$env:OPENAI_API_KEY="${KEY}"`;
  return (
    <div>
      <div className="seg" style={{ marginBottom: 10 }} role="tablist" aria-label={lang === 'fa' ? 'سیستم‌عامل' : 'Operating system'}>
        {[['mac', 'macOS / Linux'], ['win', 'Windows (PowerShell)']].map(([id, l]) => (
          <button key={id} type="button" role="tab" aria-selected={os === id} className={'seg-btn' + (os === id ? ' active' : '')} onClick={() => setOs(id)}>{l}</button>
        ))}
      </div>
      <CodeBlock code={os === 'win' ? ps : bash} label={os === 'win' ? 'PowerShell' : 'bash · zsh'} />
    </div>
  );
}
