import { navGroups } from '../data/mock.js';
import { useTheme } from '../useTheme.js';

export default function Sidebar({ current, onNavigate, effectivelyAdmin, isPanel }) {
  const { theme, toggleTheme } = useTheme();

  return (
    <aside className="sidebar">
      <div className="brand">
        <div className="brand-mark" aria-hidden="true">✨</div>
        <div style={{ minWidth: 0, display: 'flex', flexDirection: 'column', gap: 4 }}>
          <div className="brand-name">NabuGate</div>
          <span style={{ fontSize: 10, background: effectivelyAdmin ? 'var(--ng-danger-bg)' : 'var(--ng-success-bg)', color: effectivelyAdmin ? 'var(--ng-danger-text)' : 'var(--ng-success-text)', padding: '2px 6px', borderRadius: 10, width: 'fit-content' }}>
            {effectivelyAdmin ? 'مدیریت کل' : 'پنل کاربری'}
          </span>
        </div>
      </div>

      <nav className="nav">
        {navGroups.map((group, i) => {
          if (group.adminOnly && !effectivelyAdmin) return null;
          return (
            <div key={i} className="nav-group">
              <div className="nav-group-title">{group.title}</div>
              {group.items.map((item) => {
                return (
                  <button
                    key={item.id}
                    type="button"
                    className={'nav-item' + (current === item.id ? ' active' : '')}
                    aria-current={current === item.id ? 'page' : undefined}
                    onClick={() => onNavigate(item.id)}
                  >
                    <span className="ic" aria-hidden="true">
                      {item.icon}
                    </span>
                    {item.label}
                  </button>
                );
              })}
            </div>
          );
        })}
      </nav>

      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', padding: '0 24px 16px', color: 'var(--ng-muted)' }}>
        <button onClick={toggleTheme} className="btn" style={{ flex: 1, justifyContent: 'center', background: 'var(--ng-bg-hover)', border: '1px solid var(--ng-border)' }}>
          {theme === 'dark' ? '☀️ روز' : '🌙 شب'}
        </button>
      </div>

      <div className="svc">
        <span className="dot dot-ok dot-ok-ring" aria-hidden="true" />
        <div style={{ flex: 1, minWidth: 0 }}>
          <div className="svc-name">سرویس سالم است</div>
          <div className="svc-meta ltr">/healthz · 200 OK</div>
        </div>
      </div>
    </aside>
  );
}
