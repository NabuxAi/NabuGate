import { useId } from 'react';
import Icon from './Icon.jsx';
import { LANGS, useI18n } from '../i18n/index.jsx';
import { useTheme } from '../useTheme.js';

/* The NabuGate mark: three routes converging on one gate. */
export function Logo({ size = 36 }) {
  // Gradient ids must be unique per instance, or a second logo on the page
  // (header + footer) paints with whichever definition the browser found first.
  const id = useId().replace(/:/g, '');
  return (
    <svg viewBox="0 0 40 40" width={size} height={size} role="img" aria-label="NabuGate" fill="none" className="logo-mark">
      <rect x="1" y="1" width="38" height="38" rx="11" fill={`url(#${id}bg)`} />
      <rect x="1" y="1" width="38" height="38" rx="11" stroke={`url(#${id}st)`} strokeWidth="1.2" />
      <circle cx="13" cy="13" r="2.6" fill="#fff" fillOpacity="0.92" />
      <circle cx="13" cy="27" r="2.6" fill="#fff" fillOpacity="0.92" />
      <circle cx="28" cy="20" r="3.4" fill="#fff" />
      <path d="M15.3 14.1 25.2 18.7M15.3 25.9 25.2 21.3" stroke="#fff" strokeOpacity="0.85" strokeWidth="1.5" strokeLinecap="round" />
      <defs>
        <linearGradient id={`${id}bg`} x1="0" y1="0" x2="40" y2="40" gradientUnits="userSpaceOnUse">
          <stop stopColor="#4f8cff" />
          <stop offset="0.55" stopColor="#6d5dfc" />
          <stop offset="1" stopColor="#a855f7" />
        </linearGradient>
        <linearGradient id={`${id}st`} x1="0" y1="0" x2="40" y2="40" gradientUnits="userSpaceOnUse">
          <stop stopColor="#fff" stopOpacity="0.55" />
          <stop offset="1" stopColor="#fff" stopOpacity="0.08" />
        </linearGradient>
      </defs>
    </svg>
  );
}

/* A two-way segmented switch; with more languages it still lays out as pills. */
export function LangSwitch({ className = '', size }) {
  const { lang, setLang } = useI18n();
  return (
    <div className={'seg ' + (size === 'sm' ? 'seg-sm ' : '') + className} role="group" aria-label="Language / زبان">
      {Object.values(LANGS).map((l) => (
        <button
          key={l.code}
          type="button"
          lang={l.code}
          className={'seg-btn' + (lang === l.code ? ' active' : '')}
          aria-pressed={lang === l.code}
          title={l.name}
          onClick={() => setLang(l.code)}
        >
          {l.short}
        </button>
      ))}
    </div>
  );
}

export function ThemeToggle({ className = '' }) {
  const { theme, toggleTheme } = useTheme();
  const { lang } = useI18n();
  const label = theme === 'dark'
    ? (lang === 'fa' ? 'حالت روز' : 'Light mode')
    : (lang === 'fa' ? 'حالت شب' : 'Dark mode');
  return (
    <button type="button" className={'icon-btn ' + className} onClick={toggleTheme} aria-label={label} title={label}>
      <Icon name={theme === 'dark' ? 'sun' : 'moon'} size={17} />
    </button>
  );
}
