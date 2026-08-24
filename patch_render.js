  let allowed = ['dashboard', 'integration', 'tokens', 'usage', 'logs'];
  if (session.is_admin) allowed = Object.keys(VIEWS);
  const safeView = allowed.includes(view) ? view : 'dashboard';
  const render = VIEWS[safeView] || VIEWS.dashboard;
