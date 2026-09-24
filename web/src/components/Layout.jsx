import { useEffect, useRef } from 'react';
import Icon from './Icon.jsx';
import { useI18n } from '../i18n/index.jsx';

export default function Layout({ title, subtitle, actions, children }) {
  const main = useRef(null);
  const { lang } = useI18n();
  const openNav = () => document.querySelector('.app')?.classList.add('nav-open');

  // The topbar is a floating material; its edge only appears once content has
  // actually scrolled beneath it. Passive listener, class toggle only, so the
  // scroll thread never waits on this.
  useEffect(() => {
    const el = main.current;
    if (!el) return undefined;
    const onScroll = () => el.classList.toggle('scrolled', window.scrollY > 4);
    onScroll();
    window.addEventListener('scroll', onScroll, { passive: true });
    return () => window.removeEventListener('scroll', onScroll);
  }, []);

  return (
    <div className="main" ref={main}>
      <header className="topbar">
        <div className="topbar-title">
          <button type="button" className="menu-btn" onClick={openNav} aria-label={lang === 'fa' ? 'منو' : 'Menu'}>
            <Icon name="menu" size={19} />
          </button>
          <div style={{ minWidth: 0 }}>
            <h2>{title}</h2>
            {subtitle && <p className="sub">{subtitle}</p>}
          </div>
        </div>
        {actions && <div className="topbar-actions">{actions}</div>}
      </header>
      <div className="content">{children}</div>
    </div>
  );
}
