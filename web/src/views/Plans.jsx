import { navigate } from '../nav.js';
import { useState } from 'react';
import Layout from '../components/Layout.jsx';
import { usd } from '../data/mock.js';
import { usePayment } from '../components/usePayment.js';

export default function Plans() {
  const [selected, setSelected] = useState(null);
  const payment = usePayment(() => navigate('account'));

  // Amounts are dollars, because /api/me/recharge adds this number straight to
  // a balance the gateway spends in USD. These were 100000, 500000 and 2000000
  // labelled تومان — so buying the "۱۰۰٬۰۰۰ تومان" package credited $100,000.
  const plans = [
    { id: 'starter', name: 'بستهٔ پایه', amount: 10 },
    { id: 'pro', name: 'بستهٔ حرفه‌ای', amount: 50, popular: true },
    { id: 'ultra', name: 'بستهٔ تجاری', amount: 200 },
  ];

  const buy = (plan) => {
    setSelected(plan);
    payment.pay(plan.amount);
  };

  return (
    <Layout title="خرید و شارژ حساب" subtitle="افزایش موجودی برای استفاده از مدل‌ها.">
      {payment.error && <div className="card banner-error">{payment.error}</div>}
      {payment.settled?.credited && <div className="card banner-ok">پرداخت تأیید شد و موجودی اضافه شد.</div>}

      {/* This screen used to fake the whole thing: a modal branded "NabuPay",
          a choice of four real banks, a 1.5-second wait and then a report that
          payment had succeeded — with no gateway involved and the balance
          simply incremented. It now hands off to the real gateway through the
          NabuPay bridge, and the money is credited only once that gateway
          confirms it. */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(260px, 1fr))', gap: 24, padding: '16px 0' }}>
        {plans.map((plan) => (
          <div
            key={plan.id}
            className="card"
            style={{
              padding: 32, display: 'flex', flexDirection: 'column', position: 'relative',
              border: plan.popular ? '2px solid var(--ng-fg)' : undefined,
            }}
          >
            {plan.popular && (
              <div style={{ position: 'absolute', top: -12, left: '50%', transform: 'translateX(-50%)', background: 'var(--ng-fg)', color: 'var(--ng-bg)', padding: '4px 12px', borderRadius: 12, fontSize: 12, fontWeight: 'bold' }}>
                پیشنهاد ویژه
              </div>
            )}
            <h3 style={{ fontSize: 20, marginBottom: 8, fontWeight: 700 }}>{plan.name}</h3>
            <div style={{ fontSize: 32, fontWeight: 800, margin: '16px 0', color: 'var(--ng-fg)' }} dir="ltr">
              {usd(plan.amount)}
            </div>
            <ul style={{ listStyle: 'none', padding: 0, margin: '0 0 32px 0', color: 'var(--ng-muted)', flex: 1, fontSize: 14, lineHeight: 1.9 }}>
              {/* No processing-priority tiers exist: the router picks a target
                  by alias and fallback order, not by what somebody paid. */}
              <li>✓ پرداخت به‌ازای مصرف (pay-as-you-go)</li>
              <li>✓ دسترسی به همان مدل‌هایی که کلیدتان اجازه دارد</li>
              <li>✓ موجودی منقضی نمی‌شود</li>
            </ul>
            <button
              className={`btn ${plan.popular ? 'btn-primary' : ''}`}
              style={!plan.popular ? { border: '1px solid var(--ng-border)', background: 'transparent', color: 'var(--ng-fg)' } : {}}
              onClick={() => buy(plan)}
              disabled={payment.busy}
            >
              {payment.busy && selected?.id === plan.id
                ? 'در حال انتقال به درگاه…'
                : `پرداخت ${usd(plan.amount)}`}
            </button>
          </div>
        ))}
      </div>
    </Layout>
  );
}
