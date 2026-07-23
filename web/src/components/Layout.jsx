export default function Layout({ title, subtitle, actions, children }) {
  return (
    <div className="main">
      <header className="topbar">
        <div>
          <h2>{title}</h2>
          {subtitle && <p className="sub">{subtitle}</p>}
        </div>
        {actions && <div className="topbar-actions">{actions}</div>}
      </header>
      <div className="content">{children}</div>
    </div>
  );
}
