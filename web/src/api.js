/*
 * Live console API.
 */

const BASE = '/api';

async function reqPublic(path) {
  const res = await fetch(path);
  if (!res.ok) return [];
  return res.json();
}

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
    // Nothing read this flag, so an expired session left every card on the
    // page showing the word "unauthenticated" and no way back to the sign-in
    // form short of reloading by hand. The app listens for this and re-checks
    // the session, which renders the sign-in screen.
    window.dispatchEvent(new CustomEvent('nabu:unauthenticated'));
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
    const err = new Error(message);
    err.status = res.status;
    throw err;
  }
  return res.status === 204 ? null : res.json();
}

// ---- session ----------------------------------------------------------------

export const status = () => req('/status');
export const statusNabu = () => req("/nabu/status");
export const publicModels = () => reqPublic("/api/public/models");



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
// Starts a real payment: answers with an invoice number and the gateway's
// checkout URL, not with a new balance. Nothing is credited until the payer
// comes back and settleMyPayment confirms with the gateway.
export const rechargeMe = (amount, gateway) =>
  req('/me/recharge', {
    method: 'POST',
    body: JSON.stringify({ amount: Number(amount), gateway }),
  });

// Finishes whatever this account left pending at the gateway. Takes no
// invoice number: the server knows which invoices this caller started, and
// nothing the gateway put in the return URL is worth reading.
export const settleMyPayments = () => req('/me/payments/settle', { method: 'POST' });

// Both of these called get()/post(), which this module has never defined, and
// doubled the /api prefix that req() already adds. Every admin user screen
// threw a ReferenceError before it could render a row.
export const listUsers = () => req('/users');

export const adminRechargeUser = (email, amount) =>
  req('/users/recharge', {
    method: 'POST',
    body: JSON.stringify({ email, amount: Number(amount) }),
  });

// ---- the signed-in account -------------------------------------------------

export const myUsage = () => req('/me/usage');

export const changeMyPassword = (current, next) =>
  req('/me/password', {
    method: 'POST',
    body: JSON.stringify({ current, new: next }),
  });

export const recentRequests = () => req('/requests');
