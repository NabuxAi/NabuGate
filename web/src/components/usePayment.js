import { useCallback, useEffect, useRef, useState } from 'react';
import * as api from '../api.js';
import { localError } from './PaymentNotices.jsx';

export const MIN_USD = 1;
export const MAX_USD = 5000;

/**
 * Starting a payment and finishing one, shared by every screen that offers a
 * top-up or shows payment history.
 *
 * Finishing is the half that is easy to get wrong. The gateway returns the
 * payer to us with whatever query string it likes, and none of it is evidence
 * — so the server asks the gateway what actually happened. Refreshing that
 * page is ordinary, so settling is safe to repeat and credits once.
 *
 * `error` is the Error from api.js (with `code`), never a bare message: the
 * screens translate it with <PayError>.
 */
export function usePayment(onSettled) {
  const [busy, setBusy] = useState(false);
  const [checking, setChecking] = useState(false);
  const [error, setError] = useState(null);
  const [settled, setSettled] = useState(null);
  const cb = useRef(onSettled);
  cb.current = onSettled;

  const settle = useCallback(async () => {
    const res = await api.settleMyPayments();
    setSettled(res);
    // Anything that moved — credited, a plan bought, an invoice turned failed —
    // changes what the screen shows, so the screen reloads in place.
    if (cb.current && (res?.credited || res?.subscribed || res?.payments?.length)) cb.current(res);
    return res;
  }, []);

  useEffect(() => {
    // Settle on EVERY load of a screen that offers payment, not only when the
    // gateway's query is present: a gateway that confirms later — NowPayments
    // waits for chain confirmations — would otherwise leave the invoice pending
    // forever. Asking is cheap: the server returns at once when nothing is pending.
    const params = new URLSearchParams(window.location.search);
    if (params.has('status') || params.has('gateway')) {
      // Cosmetic only: keep the gateway's query out of a URL somebody might copy.
      const clean = new URL(window.location.href);
      clean.search = '';
      window.history.replaceState(null, '', clean.toString());
    }

    let cancelled = false;
    api
      .settleMyPayments()
      .then((res) => {
        if (cancelled) return;
        setSettled(res);
        if (cb.current && (res?.credited || res?.subscribed || res?.payments?.length)) cb.current(res);
      })
      .catch((e) => {
        // 501 is "this deployment has no gateway", which the screens already
        // say in their own words; repeating it on every visit made an ordinary
        // deployment look broken.
        if (!cancelled && e.status !== 501 && !e.unauthenticated) setError(e);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  // "Check status": the same settle, on demand. Returns the result so the
  // screen can say "still waiting" instead of nothing.
  async function recheck() {
    setChecking(true);
    setError(null);
    try {
      return await settle();
    } catch (e) {
      if (e.status !== 501) setError(e);
      return null;
    } finally {
      setChecking(false);
    }
  }

  // { amount } tops up; { plan } pays exactly what that plan still needs and
  // buys it once the money lands (the server ignores amount then).
  async function pay({ amount, gateway, plan } = {}) {
    setError(null);
    if (!plan) {
      const n = Number(amount);
      if (!(n >= MIN_USD && n <= MAX_USD)) {
        setError(localError('amount_out_of_range', { min_usd: MIN_USD, max_usd: MAX_USD }));
        return;
      }
    }
    setBusy(true);
    try {
      const res = await api.rechargeMe({ amount, gateway, plan });
      if (!res?.checkout_url) throw localError('no_checkout');
      // The gateway's own page. Leaving the panel here is the point: the card
      // details are entered at the bank, never in this app.
      window.location.assign(res.checkout_url);
    } catch (e) {
      setError(e);
      setBusy(false);
    }
  }

  return { pay, recheck, busy, checking, error, settled, setError };
}
