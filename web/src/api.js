/*
 * Live console API.
 *
 * The console used to render entirely from src/data/mock.js, so every number it
 * showed was invented. These call the gateway for real.
 *
 * Authentication is a session cookie set by /admin/api/login, not a gateway
 * key: the console must never hold one, because a key in a browser is a key
 * anyone with the URL can read.
 */

const BASE = '/admin/api';

async function req(path, options = {}) {
  const res = await fetch(BASE + path, {
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
    ...options,
  });

  if (res.status === 401) {
    const err = new Error('unauthenticated');
    err.unauthenticated = true;
    throw err;
  }
  if (!res.ok) {
    let message = res.statusText;
    try {
      const body = await res.json();
      message = body?.error?.message || body?.error || message;
    } catch {
      /* non-JSON error body */
    }
    throw new Error(message);
  }
  return res.status === 204 ? null : res.json();
}

// ---- session ----------------------------------------------------------------

export const status = () => req('/status');

export const setup = (username, password) =>
  req('/setup', { method: 'POST', body: JSON.stringify({ username, password }) });

export const login = (username, password) =>
  req('/login', { method: 'POST', body: JSON.stringify({ username, password }) });

export const logout = () => req('/logout', { method: 'POST' });

// ---- tokens -----------------------------------------------------------------

export const listTokens = () => req('/tokens');

export const createToken = ({ name, allow, rateLimit, allowedOrigins }) =>
  req('/tokens', {
    method: 'POST',
    body: JSON.stringify({
      name,
      allow,
      rate_limit: rateLimit,
      allowed_origins: allowedOrigins,
    }),
  });

export const deleteToken = (name) =>
  req(`/tokens/${encodeURIComponent(name)}`, { method: 'DELETE' });

export const setTokenDisabled = (name, disabled) =>
  req(`/tokens/${encodeURIComponent(name)}`, {
    method: 'PATCH',
    body: JSON.stringify({ disabled }),
  });

export const setTokenOrigins = (name, allowedOrigins) =>
  req(`/tokens/${encodeURIComponent(name)}`, {
    method: 'PATCH',
    body: JSON.stringify({ allowed_origins: allowedOrigins }),
  });

export const overview = () => req('/overview');

// ---- usage ------------------------------------------------------------------

export const usage = () => req('/usage');

export const resetUsage = (project) =>
  req(`/usage/reset${project ? `?project=${encodeURIComponent(project)}` : ''}`, {
    method: 'POST',
  });
