import { useEffect, useState } from 'react';
import * as api from '../api.js';

/**
 * Starting a payment and finishing one, shared by the two screens that offer
 * a top-up.
 *
 * Finishing is the half that is easy to get wrong. The gateway returns the
 * payer to us with whatever query string it likes, and none of it is evidence
 * — so the only thing read from the URL is the invoice number, and the server
 * asks the gateway what actually happened. Refreshing that page is ordinary,
 * so settling is safe to repeat and credits once.
 */
export function usePayment(onSettled) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [settled, setSettled] = useState(null);

  useEffect(() => {
    // Settle on EVERY load of a screen that offers payment, not only when the
    // gateway's query is present.
    //
    // This used to require ?status= or ?gateway= and then strip the query
    // immediately, which gave each payment exactly one settlement attempt: the
    // single page load on return from the bank. A gateway that confirms later —
    // NowPayments waits for chain confirmations — or one blip in the bridge left
    // the invoice pending forever, and the banner telling the payer to reopen
    // the page could not work, because reopening it carries no query.
    //
    // Asking is cheap: the server returns immediately when nothing is pending.
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
        if (res?.credited && onSettled) onSettled();
      })
      .catch((e) => {
        if (!cancelled) setError(e.message);
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function pay(amount, gateway) {
    setBusy(true);
    setError(null);
    try {
      const res = await api.rechargeMe(amount, gateway);
      if (!res?.checkout_url) {
        throw new Error('درگاه پرداخت آدرسی برای ادامه نداد.');
      }
      // The gateway's own page. Leaving the panel here is the point: the card
      // details are entered at the bank, never in this app.
      window.location.assign(res.checkout_url);
    } catch (e) {
      setError(e.message);
      setBusy(false);
    }
  }

  return { pay, busy, error, settled, setError };
}
