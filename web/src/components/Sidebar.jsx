import { navGroups } from '../data/mock.js';

export default function Sidebar({ current, onNavigate, isAdmin }) {
  return (
    <aside className="sidebar">
      <div className="brand">
        <div className="brand-mark" aria-hidden="true">✨</div>
        <div style={{ minWidth: 0 }}>
          <div className="brand-name">Console Jarvis</div>
        </div>
      </div>

      <nav className="nav">
        {navGroups.map((group, i) => {
          if (group.adminOnly && !isAdmin) return null;
          return (
            <div key={i} className="nav-group">
              <div className="nav-group-title">{group.title}</div>
              {group.items.map((item) => {
                // If you want some items to be hidden from non-admins, you can filter them here if needed.
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
