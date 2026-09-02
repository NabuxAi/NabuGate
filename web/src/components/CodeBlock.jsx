import { useState } from 'react';

/* A left-to-right code sample with a copy button that appears on hover. */
export default function CodeBlock({ code, label, style }) {
  const [done, setDone] = useState(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(code);
      setDone(true);
      setTimeout(() => setDone(false), 1600);
    } catch {
      /* clipboard blocked: the text is still selectable */
    }
  }

  return (
    <div className="code-wrap" style={style}>
      {label && (
        <div style={{ fontSize: 11, color: 'var(--ng-muted)', marginBottom: 6, fontFamily: 'var(--ng-mono)' }} dir="ltr">
          {label}
        </div>
      )}
      <pre className="code" dir="ltr"><code>{code}</code></pre>
      <button type="button" className={'copy' + (done ? ' done' : '')} onClick={copy} aria-label="کپی">
        {done ? '✓ کپی شد' : 'کپی'}
      </button>
    </div>
  );
}
