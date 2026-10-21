/*
 * The origin the page was actually served from.
 *
 * API calls in api.js are already relative. Code samples must be too, in the
 * sense that they copy *this* host: a visitor on the Iran mirror
 * (https://gate.nabuxai.ir) has to copy https://gate.nabuxai.ir/v1, because
 * their browser is not supposed to talk to gate.nabuxai.com at all.
 * Nothing here falls back to a hard-coded host. When there is no browser
 * (a test render), samples use a relative /v1.
 */

export function siteOrigin() {
  if (typeof window === 'undefined') return '';
  const { origin, protocol } = window.location;
  if (!origin || origin === 'null' || (protocol !== 'https:' && protocol !== 'http:')) return '';
  return origin;
}

export function gatewayBase() {
  const origin = siteOrigin();
  return origin ? `${origin}/v1` : '/v1';
}
