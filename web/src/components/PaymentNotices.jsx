import { usd, fmtInt, fmtDate, useT } from '../i18n/index.jsx';
import Icon from './Icon.jsx';

/*
 * What the payment screens say, shared by Plans, Account and Payments.
 *
 * The server answers every refusal with an English sentence and a `code`. The
 * sentence is a fallback for API callers; a Persian payer reading "payment
 * bridge refused" learns nothing. So the screen speaks by code and keeps the
 * raw sentence only behind a "details" toggle, for support.
 */

const T = {
  fa: {
    payments_disabled: 'پرداخت آنلاین در این استقرار فعال نیست. برای شارژ دستی با پشتیبانی تماس بگیرید.',
    amount_out_of_range: 'مبلغ باید بین {min} و {max} باشد.',
    gateway_refused: 'درگاه پرداخت ساختِ فاکتور را نپذیرفت. چیزی از حسابتان کم نشده؛ چند دقیقه بعد دوباره تلاش کنید یا درگاه دیگری انتخاب کنید.',
    bridge_unreachable: 'سرویس پرداخت الان در دسترس نیست. چیزی از حسابتان کم نشده؛ چند دقیقه بعد دوباره تلاش کنید.',
    insufficient_balance: 'موجودی شما ({balance}) برای این اشتراک ({price}) کافی نیست؛ {shortfall} کم دارید.',
    plan_switch_needs_confirm: 'اشتراکِ فعالِ «{name}» هنوز {days} روز دیگر اعتبار دارد و با تعویض از بین می‌رود.',
    no_such_plan: 'این اشتراک دیگر وجود ندارد. صفحه را دوباره باز کنید تا فهرستِ به‌روز را ببینید.',
    plan_unavailable: 'این اشتراک موقتاً در دسترس نیست. کمی بعد دوباره تلاش کنید.',
    no_checkout: 'درگاه پرداخت آدرسی برای ادامه نداد. چند دقیقه بعد دوباره تلاش کنید.',
    generic: 'درخواست انجام نشد{status}. کمی بعد دوباره تلاش کنید.',
    httpStatus: ' (کد {code})',
    details: 'جزئیات',
    sep: '، ',

    credited: 'پرداخت تأیید شد و موجودی اضافه شد.',
    creditedBalance: 'پرداخت تأیید شد و موجودی اضافه شد. موجودی فعلی: {balance}',
    subscribed: 'اشتراکِ «{name}» فعال شد و تا {date} اعتبار دارد.',
    stillPending: (v) => <><strong>{v.n} پرداخت هنوز منتظرِ تأییدِ درگاه است.</strong> این چند دقیقه طبیعی است؛ هر بار که این صفحه باز شود دوباره از درگاه پرسیده می‌شود و به‌محض تأیید، موجودی یک‌بار اضافه می‌شود.</>,
    failed: (v) => <><strong>{v.n} پرداخت ناموفق بود یا لغو شد</strong> و چیزی به موجودی اضافه نشد. اگر مبلغ از کارتتان کم شده، بانک ظرف ۷۲ ساعت آن را برمی‌گرداند.</>,
    underpaid: (v) => <><strong>مبلغِ پرداخت‌شده کمتر از مبلغِ فاکتور بود</strong> و برای همین خودکار به موجودی اضافه نشد. شناسهٔ فاکتور {v.ids} را برای پشتیبانی بفرستید تا بررسی شود.</>,

    disabled: () => <><strong>درگاه پرداخت در این استقرار فعال نیست.</strong> دکمه‌های پرداخت کار نمی‌کنند تا مدیر متغیرهای <code dir="ltr">NABUPAY_URL</code> و <code dir="ltr">NABUPAY_SECRET</code> را تنظیم کند. برای شارژ دستی با پشتیبانی تماس بگیرید.</>,
    gateway: 'درگاه پرداخت',
    chooseLater: 'در صفحهٔ پرداخت انتخاب می‌کنم',
    bankRate: 'بانک معادلِ تومانیِ این مبلغ را با نرخ روز کم می‌کند؛ همین مبلغِ دلاری به کیف‌پولتان اضافه می‌شود.',
  },
  en: {
    payments_disabled: 'Online payment is not enabled on this deployment. Contact support for a manual top-up.',
    amount_out_of_range: 'The amount must be between {min} and {max}.',
    gateway_refused: 'The payment gateway refused to create the invoice. Nothing was charged; try again in a few minutes or pick a different gateway.',
    bridge_unreachable: 'The payment service is unreachable right now. Nothing was charged; try again in a few minutes.',
    insufficient_balance: 'Your balance ({balance}) does not cover this plan ({price}); you are {shortfall} short.',
    plan_switch_needs_confirm: 'Your active plan “{name}” still has {days} days left, which switching would forfeit.',
    no_such_plan: 'This plan no longer exists. Reload the page to see the current plans.',
    plan_unavailable: 'This plan is temporarily unavailable. Try again later.',
    no_checkout: 'The payment gateway did not return a checkout URL. Try again in a few minutes.',
    generic: 'The request failed{status}. Try again in a moment.',
    httpStatus: ' (code {code})',
    details: 'Details',
    sep: ', ',

    credited: 'Payment confirmed and your balance has been credited.',
    creditedBalance: 'Payment confirmed and your balance has been credited. Current balance: {balance}',
    subscribed: 'Plan “{name}” is active until {date}.',
    stillPending: (v) => <><strong>{v.n} payment(s) still awaiting the gateway’s confirmation.</strong> A few minutes is normal; the gateway is asked again every time this page opens, and your balance is credited exactly once when it confirms.</>,
    failed: (v) => <><strong>{v.n} payment(s) failed or were cancelled</strong>, so nothing was added to your balance. If money left your card, the bank returns it within 72 hours.</>,
    underpaid: (v) => <><strong>Less was paid than the invoice asked for</strong>, so it was not credited automatically. Send invoice {v.ids} to support to have it reviewed.</>,

    disabled: () => <><strong>The payment gateway is not enabled on this deployment.</strong> Payment buttons won’t work until an admin sets <code dir="ltr">NABUPAY_URL</code> and <code dir="ltr">NABUPAY_SECRET</code>. Contact support for a manual top-up.</>,
    gateway: 'Payment gateway',
    chooseLater: 'Let me choose at the payment page',
    bankRate: 'The bank charges the toman equivalent at the day’s rate; this dollar amount is what lands in your wallet.',
  },
};

const KNOWN = new Set([
  'payments_disabled', 'amount_out_of_range', 'gateway_refused', 'bridge_unreachable',
  'insufficient_balance', 'plan_switch_needs_confirm', 'no_such_plan', 'plan_unavailable', 'no_checkout',
]);

/* A fresh key per click: the server charges once however often it is sent. */
export function newIdempotencyKey() {
  try {
    if (window.crypto?.randomUUID) return window.crypto.randomUUID();
  } catch { /* insecure context */ }
  const rnd = () => Math.random().toString(16).slice(2, 10);
  return `${Date.now().toString(16)}-${rnd()}-${rnd()}-${rnd()}`;
}

/* An error the client raises itself, shaped like one from the server. */
export function localError(code, details = {}) {
  const e = new Error(code);
  e.code = code;
  e.details = details;
  e.local = true;
  return e;
}

function sentence(t, err) {
  const d = err.details || {};
  switch (err.code) {
    case 'amount_out_of_range':
      return t('amount_out_of_range', { min: usd(d.min_usd ?? 1), max: usd(d.max_usd ?? 5000) });
    case 'insufficient_balance':
      return t('insufficient_balance', {
        balance: usd(d.balance_usd),
        price: usd(d.price_usd),
        shortfall: usd(d.shortfall_usd),
      });
    case 'plan_switch_needs_confirm':
      return t('plan_switch_needs_confirm', { name: d.current_name || d.current_plan || '', days: fmtInt(d.days_left) });
    default:
      if (KNOWN.has(err.code)) return t(err.code);
      return t('generic', { status: err.status ? t('httpStatus', { code: fmtInt(err.status) }) : '' });
  }
}

/*
 * A refusal in the reader's language. `error` may be an Error from api.js or a
 * string the screen already translated. `action` is an optional button that
 * fixes the problem in place (e.g. "pay the difference").
 */
export function PayError({ error, action }) {
  const t = useT(T);
  if (!error) return null;
  if (typeof error === 'string') {
    return <div className="card banner-error"><Icon name="alert" size={18} /><div className="pay-msg">{error}</div></div>;
  }
  const raw = !error.local && error.message && error.message !== error.code ? error.message : '';
  return (
    <div className="card banner-error" role="alert">
      <Icon name="alert" size={18} />
      <div className="pay-msg">
        <div>{sentence(t, error)}</div>
        {raw && (
          <details className="err-details">
            <summary>{t('details')}</summary>
            <span dir="auto">{raw}</span>
          </details>
        )}
        {action && <div className="pay-action">{action}</div>}
      </div>
    </div>
  );
}

/*
 * What a settle round found: credited, a plan bought on arrival, still waiting
 * (information, not an error), failed or cancelled, or paid short.
 */
export function SettleBanners({ result, hidePending = false }) {
  const t = useT(T);
  if (!result) return null;
  const list = Array.isArray(result.payments) ? result.payments : [];
  const pending = list.filter((p) => p.status === 'pending');
  const failedList = list.filter((p) => p.status === 'failed' || p.status === 'expired');
  const failedN = Math.max(Number(result.failed) || 0, failedList.length);
  const underpaid = list.filter((p) => p.status === 'underpaid');
  const ids = (ps) => ps.map((p, i) => (
    <span key={p.invoice || i}>{i > 0 && t('sep')}<code dir="ltr">{p.invoice}</code></span>
  ));

  return (
    <>
      {result.credited ? (
        <div className="card banner-ok" role="status">
          <Icon name="check" size={18} />
          <div className="pay-msg">
            {result.balance != null ? t('creditedBalance', { balance: usd(result.balance) }) : t('credited')}
          </div>
        </div>
      ) : null}
      {result.subscribed && (
        <div className="card banner-ok" role="status">
          <Icon name="key" size={18} />
          <div className="pay-msg">
            {t('subscribed', { name: result.subscribed.name || result.subscribed.plan_id, date: fmtDate(result.subscribed.expires_at) })}
          </div>
        </div>
      )}
      {pending.length > 0 && !hidePending && (
        <div className="callout" role="status">
          <span className="ci"><Icon name="clock" size={18} /></span>
          <div>{t('stillPending', { n: fmtInt(pending.length) })}</div>
        </div>
      )}
      {failedN > 0 && (
        <div className="card banner-warn" role="status">
          <Icon name="ban" size={18} />
          <div className="pay-msg">{t('failed', { n: fmtInt(failedN) })}</div>
        </div>
      )}
      {underpaid.length > 0 && (
        <div className="card banner-error" role="alert">
          <Icon name="alert" size={18} />
          <div className="pay-msg">{t('underpaid', { ids: ids(underpaid) })}</div>
        </div>
      )}
    </>
  );
}

export function PaymentsDisabled() {
  const t = useT(T);
  return (
    <div className="callout warn">
      <span className="ci"><Icon name="pause" size={18} /></span>
      <div>{t('disabled')}</div>
    </div>
  );
}

/*
 * Optional: only shown when the deployment lists its gateways. The default is
 * NabuPay's own page, where the payer picks one that is working today.
 */
export function GatewayPicker({ gateways, value, onChange }) {
  const t = useT(T);
  if (!Array.isArray(gateways) || gateways.length === 0) return null;
  return (
    <label className="gw-pick">
      <span className="muted">{t('gateway')}</span>
      <select className="input" value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">{t('chooseLater')}</option>
        {gateways.map((g) => (
          <option key={g.slug} value={g.slug}>{g.label || g.slug}</option>
        ))}
      </select>
    </label>
  );
}

export function BankRateNote() {
  const t = useT(T);
  return <p className="muted pay-note">{t('bankRate')}</p>;
}
