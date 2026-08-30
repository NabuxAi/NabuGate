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
    // Only worth asking when the payer has plausibly just come back from a
    // gateway. The bridge appends its own query to the return URL, so its
    // presence is the signal — its contents are not read, because they come
    // from the payer's browser and prove nothing.
    const params = new URLSearchParams(window.location.search);
    if (!params.has('status') && !params.has('gateway')) return;

    // Cleared before anything else, so a refresh does not re-enter this and a
    // copied URL carries no payment query.
    const clean = new URL(window.location.href);
    clean.search = '';
    window.history.replaceState(null, '', clean.toString());

    api
      .settleMyPayments()
      .then((res) => {
        setSettled(res);
        if (res?.credited && onSettled) onSettled();
      })
      .catch((e) => setError(e.message));
    // Once, on mount: the query is gone by the time anything could re-run it.
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
