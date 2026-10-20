import { useEffect, useId, useRef, useState } from 'react';
import Icon from '../components/Icon.jsx';
import { useI18n, useT } from '../i18n/index.jsx';

const SEEN_KEY = 'nabugate_intro_dismissed';

// Files rendered from video/ and copied into the site bundle. Paths are
// relative so the Iran mirror serves them from its own host.
const MEDIA = {
  fa: { mp4: '/intro/nabugate-fa.mp4', webm: '/intro/nabugate-fa.webm', poster: '/intro/nabugate-fa.jpg' },
  en: { mp4: '/intro/nabugate-en.mp4', webm: '/intro/nabugate-en.webm', poster: '/intro/nabugate-en.jpg' },
  ar: { mp4: '/intro/nabugate-ar.mp4', webm: '/intro/nabugate-ar.webm', poster: '/intro/nabugate-ar.jpg' },
};

const T = {
  fa: {
    watch: 'ویدیوی معرفی',
    title: 'معرفی کوتاه',
    close: 'بستن ویدیو',
  },
  en: {
    watch: 'Watch the intro',
    title: 'A short intro',
    close: 'Close video',
  },
  ar: {
    watch: 'شاهد المقدمة',
    title: 'مقدمة قصيرة',
    close: 'إغلاق الفيديو',
  },
};

function wasDismissed() {
  try { return localStorage.getItem(SEEN_KEY) === '1'; } catch { return false; }
}
function rememberDismissed() {
  try { localStorage.setItem(SEEN_KEY, '1'); } catch { /* private mode: this visit only */ }
}

export function useIntro() {
  const [open, setOpen] = useState(false);

  useEffect(() => {
    if (wasDismissed()) return undefined;
    const id = window.setTimeout(() => setOpen(true), 700);
    return () => window.clearTimeout(id);
  }, []);

  const close = () => {
    rememberDismissed();
    setOpen(false);
  };
  const replay = () => setOpen(true);
  return { open, close, replay };
}

export function IntroDialog({ open, onClose }) {
  const t = useT(T);
  const { lang } = useI18n();
  const titleId = useId();
  const closeRef = useRef(null);
  const media = MEDIA[lang] || MEDIA.fa;
  const reduce = typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  useEffect(() => {
    if (!open) return undefined;
    const prev = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    closeRef.current?.focus();
    const onKey = (e) => { if (e.key === 'Escape') onClose(); };
    window.addEventListener('keydown', onKey);
    return () => {
      document.body.style.overflow = prev;
      window.removeEventListener('keydown', onKey);
    };
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div className="lp-intro-backdrop" onClick={onClose}>
      <div
        className="lp-intro-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="lp-intro-bar">
          <h2 id={titleId}>{t('title')}</h2>
          <button ref={closeRef} type="button" className="lp-intro-close" onClick={onClose} aria-label={t('close')}>
            <Icon name="x" size={18} />
          </button>
        </div>
        <video
          key={lang}
          className="lp-intro-video"
          controls
          playsInline
          muted
          autoPlay={!reduce}
          preload="none"
          poster={media.poster}
        >
          <source src={media.webm} type="video/webm" />
          <source src={media.mp4} type="video/mp4" />
        </video>
      </div>
    </div>
  );
}

export function IntroButton({ onClick, className = '' }) {
  const t = useT(T);
  return (
    <button type="button" className={'lp-btn lp-btn-outline lp-btn-lg ' + className} onClick={onClick}>
      <Icon name="play" size={18} />{t('watch')}
    </button>
  );
}
