export default function EmptyState({ icon = '◌', title, hint, action }) {
  return (
    <div className="empty fade-in">
      <div className="empty-icon" aria-hidden="true">{icon}</div>
      {title && <h4>{title}</h4>}
      {hint && <p>{hint}</p>}
      {action && <div style={{ marginTop: 8 }}>{action}</div>}
    </div>
  );
}
