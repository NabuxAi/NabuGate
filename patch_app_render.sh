sed -i '' '/const render = VIEWS\[view\] || VIEWS.dashboard;/d' web/src/App.jsx
cat << 'INNER' > patch_render.js
  let allowed = ['dashboard', 'integration', 'tokens', 'usage', 'logs'];
  if (session.is_admin) allowed = Object.keys(VIEWS);
  const safeView = allowed.includes(view) ? view : 'dashboard';
  const render = VIEWS[safeView] || VIEWS.dashboard;
INNER
sed -i '' -e '/<div className="app">/r patch_render.js' -e '/<div className="app">/d' web/src/App.jsx
sed -i '' 's/{render()}/<div className="app">\n      <Sidebar current={safeView} onNavigate={navigate} isAdmin={session.is_admin} \/>\n      {render()}\n    <\/div>/g' web/src/App.jsx
