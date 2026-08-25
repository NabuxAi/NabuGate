export function navigate(id) {
  const isPanel = window.location.pathname.startsWith('/panel');
  const isAdminPath = window.location.pathname.startsWith('/admin');
  const basePath = isAdminPath ? '/admin' : (isPanel ? '/panel' : '');
  const newUrl = `${basePath}/${id}`;
  window.history.pushState(null, '', newUrl);
  window.dispatchEvent(new PopStateEvent('popstate'));
}
