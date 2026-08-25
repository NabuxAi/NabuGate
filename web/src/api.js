/*
 * Live console API.
 */

const BASE = '/api';

async function req(path, options = {}) {
  // Prevent aggressive browser caching for GET requests
  const url = (options.method && options.method !== 'GET') ? BASE + path : BASE + path + (path.includes('?') ? '&' : '?') + '_t=' + Date.now();
  
  const res = await fetch(url, {
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

export const signup = (username, password) =>
  req("/signup", { method: "POST", body: JSON.stringify({ username, password }) });

export const login = (username, password) =>
  req('/login', { method: 'POST', body: JSON.stringify({ username, password }) });

export const logout = () => req('/logout', { method: 'POST' });

// ---- tokens -----------------------------------------------------------------

export const listTokens = () => req('/tokens');

export const createToken = ({ name, allow, rateLimit, allowedOrigins, providers }) =>
  req('/tokens', {
    method: 'POST',
    body: JSON.stringify({
      name,
      allow,
      rate_limit: rateLimit,
      allowed_origins: allowedOrigins,
      providers,
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

// ---- admin accounts ---------------------------------------------------------

export const listAdmins = () => req('/admins');

export const createAdmin = (username, password) =>
  req('/admins', { method: 'POST', body: JSON.stringify({ username, password }) });

// ---- sub-agents & flows ----------------------------------------------------

export const listAgents = () => req('/agents');

export const saveAgent = (agent) =>
  req('/agents', {
    method: 'POST',
    body: JSON.stringify(agent),
  });

export const deleteAgent = (name) =>
  req(`/agents/${encodeURIComponent(name)}`, { method: 'DELETE' });

export const testAgent = (name, message) =>
  req(`/agents/${encodeURIComponent(name)}/test`, {
    method: 'POST',
    body: JSON.stringify({ message }),
  });

export const listFlows = () => req('/flows');

export const saveFlow = (flow) =>
  req('/flows', {
    method: 'POST',
    body: JSON.stringify(flow),
  });

export const deleteFlow = (name) =>
  req(`/flows/${encodeURIComponent(name)}`, { method: 'DELETE' });

export const testFlow = (name, message) =>
  req(`/flows/${encodeURIComponent(name)}/test`, {
    method: 'POST',
    body: JSON.stringify({ message }),
  });

export const patchToken = (name, allowedOrigins, providers) =>
  req(`/tokens/${encodeURIComponent(name)}`, {
    method: 'PATCH',
    body: JSON.stringify({ allowed_origins: allowedOrigins, providers }),
  });

export const getMe = () => req('/me');
export const rechargeMe = (amount) => req('/me/recharge', { method: 'POST', body: JSON.stringify({ amount: Number(amount) }) });
