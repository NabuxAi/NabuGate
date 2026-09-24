import { useEffect, useState } from 'react';
import * as api from '../api.js';
import { navGroups, NAV_T } from '../nav.js';
import { useT, usd } from '../i18n/index.jsx';
import Icon from './Icon.jsx';
import { LangSwitch, Logo, ThemeToggle } from './Brand.jsx';

const T = {
  fa: {
    admin: 'مدیریت کل',
    panel: 'پنل کاربری',
    healthy: 'سرویس سالم است',
    unhealthy: 'دروازه پاسخ نمی‌دهد',
    checking: 'در حال بررسی…',
    balance: 'موجودی',
    logout: 'خروج',
    close: 'بستن منو',
    language: 'زبان',
  },
  en: {
    admin: 'Administrator',
    panel: 'User console',
    healthy: 'All systems normal',
    unhealthy: 'Gateway not responding',
    checking: 'Checking…',
    balance: 'Balance',
    logout: 'Sign out',
    close: 'Close menu',
    language: 'Language',
  },
};

// The health row used to be a hard-coded "200 OK". It now asks /healthz, so a
// console left open while the gateway goes down says so.
function useHealth() {
  const [state, setState] = useState('checking');
  useEffect(() => {
    let alive = true;
    const check = () =>
      fetch('/healthz', { cache: 'no-store' })
        .then((r) => alive && setState(r.ok ? 'ok' : 'down'))
        .catch(() => alive && setState('down'));
    check();
    const iv = setInterval(check, 60000);
    return () => { alive = false; clearInterval(iv); };
  }, []);
  return state;
}

export default function Sidebar({ current, onNavigate, effectivelyAdmin }) {
  const t = useT(T);
  const tNav = useT(NAV_T);
  const health = useHealth();
  const [me, setMe] = useState(null);

  useEffect(() => {
    api.getMe().then(setMe).catch(() => setMe({}));
  }, []);

  const closeDrawer = () => document.querySelector('.app')?.classList.remove('nav-open');

  async function logout() {
    try { await api.logout(); } catch { /* the session may already be gone */ }
    window.location.reload();
  }

  const email = me?.email || me?.name || '';
  const initial = (email || '?').trim().charAt(0).toUpperCase();

  return (
    <aside className="sidebar">
      <div className="sb-brand">
        <Logo size={38} />
        <div className="sb-brand-text">
          <div className="brand-name">NabuGate</div>
          <span className={'sb-role' + (effectivelyAdmin ? ' is-admin' : '')}>
            {effectivelyAdmin ? t('admin') : t('panel')}
          </span>
        </div>
        <button type="button" className="icon-btn sb-close" onClick={closeDrawer} aria-label={t('close')}>
          <Icon name="x" size={17} />
        </button>
      </div>

      <nav className="nav">
        {navGroups.map((group) => {
          if (group.adminOnly && !effectivelyAdmin) return null;
          return (
            <div key={group.key} className="nav-group">
              <div className="nav-group-title">{tNav('group.' + group.key)}</div>
              {group.items.map((item) => (
                <button
                  key={item.id}
                  type="button"
                  className={'nav-item' + (current === item.id ? ' active' : '')}
                  aria-current={current === item.id ? 'page' : undefined}
                  onClick={() => {
                    // On a phone the sidebar is a drawer; a tap on an item
                    // should take you to the page, not leave the drawer open.
                    closeDrawer();
                    onNavigate(item.id);
                  }}
                >
                  <span className="ic" aria-hidden="true"><Icon name={item.icon} size={18} /></span>
                  <span className="nav-label">{tNav(item.id)}</span>
                </button>
              ))}
            </div>
          );
        })}
      </nav>

      <div className="sb-foot">
        <div className="sb-user">
          <button type="button" className="sb-user-main" onClick={() => { closeDrawer(); onNavigate('profile'); }}>
            <span className="sb-avatar" aria-hidden="true">{initial}</span>
            <span className="sb-user-text">
              <span className="sb-email" dir="ltr">{me === null ? '…' : email || '—'}</span>
              <span className="sb-balance">{t('balance')}: <b className="ltr">{usd(me?.balance)}</b></span>
            </span>
          </button>
          <button type="button" className="icon-btn" onClick={logout} aria-label={t('logout')} title={t('logout')}>
            <Icon name="logout" size={17} />
          </button>
        </div>

        <div className="sb-controls">
          <LangSwitch size="sm" />
          <ThemeToggle />
        </div>

        <div className={'svc svc-' + health}>
          <span className={'dot ' + (health === 'ok' ? 'dot-ok dot-ok-ring' : health === 'down' ? 'dot-down' : 'dot-idle')} aria-hidden="true" />
          <div style={{ flex: 1, minWidth: 0 }}>
            <div className="svc-name">{t(health === 'ok' ? 'healthy' : health === 'down' ? 'unhealthy' : 'checking')}</div>
            <div className="svc-meta ltr">/healthz{health === 'ok' ? ' · 200 OK' : ''}</div>
          </div>
        </div>
      </div>
    </aside>
  );
}
