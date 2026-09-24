import { useState } from 'react';
import { useT } from '../i18n/index.jsx';
import Icon from './Icon.jsx';

const T = {
  fa: { copy: 'کپی', copied: 'کپی شد' },
  en: { copy: 'Copy', copied: 'Copied' },
};

/* A left-to-right code sample with a copy button that appears on hover (and
   stays visible on touch screens, which have no hover). The button is pinned to
   the code box rather than the whole block, so a label above the code does not
   push it onto the box's border. */
export default function CodeBlock({ code, label, style }) {
  const t = useT(T);
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
        <div className="code-label" style={{ fontSize: 11, color: 'var(--ng-muted)', marginBottom: 6, fontFamily: 'var(--ng-mono)' }} dir="ltr">
          {label}
        </div>
      )}
      <div className="code-box">
        <pre className="code" dir="ltr"><code>{code}</code></pre>
        <button type="button" className={'copy' + (done ? ' done' : '')} onClick={copy} aria-label={t('copy')}>
          <Icon name={done ? 'check' : 'copy'} size={13} />
          {done ? t('copied') : t('copy')}
        </button>
      </div>
    </div>
  );
}
