import { navigate } from '../nav.js';
import { useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { usd } from '../data/mock.js';

export default function Plans() {
  const [loading, setLoading] = useState(false);
  const [selected, setSelected] = useState(null);
  const [successMsg, setSuccessMsg] = useState('');
  const [error, setError] = useState(null);

  // Amounts are dollars, because /api/me/recharge adds this number straight to
  // a balance the gateway spends in USD. These were 100000, 500000 and 2000000
  // labelled تومان — so buying the "۱۰۰٬۰۰۰ تومان" package credited $100,000.
  const plans = [
    { id: 'starter', name: 'بستهٔ پایه', amount: 10 },
    { id: 'pro', name: 'بستهٔ حرفه‌ای', amount: 50, popular: true },
    { id: 'ultra', name: 'بستهٔ تجاری', amount: 200 },
  ];

  const buy = async (plan) => {
    setSelected(plan);
    setLoading(true);
    setError(null);
    setSuccessMsg('');
    try {
      await api.rechargeMe(plan.amount);
      setSuccessMsg(`موجودی ${usd(plan.amount)} افزایش یافت.`);
      setTimeout(() => navigate('account'), 2000);
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
      setSelected(null);
    }
  };

  return (
    <Layout title="خرید و شارژ حساب" subtitle="افزایش موجودی برای استفاده از مدل‌ها.">
      {error && <div className="card banner-error">{error}</div>}
      {successMsg && <div className="card banner-ok">{successMsg}</div>}

      {/* This screen used to open a modal branded "NabuPay — درگاه امن یکپارچه
          نبوکس", ask you to pick between سامان, ملت, پاسارگاد and زرین‌پال,
          wait 1.5 seconds, and then report that payment through the chosen
          bank had succeeded. No gateway is connected and no bank was involved;
          the balance was simply incremented. A convincing receipt for a
          payment that did not happen is worse than no screen at all. */}
      <div className="card" style={{ marginBottom: 24, padding: 16 }}>
        <strong>درگاه پرداخت هنوز وصل نیست.</strong>
        <p className="muted" style={{ margin: '8px 0 0', fontSize: 13, lineHeight: 1.7 }}>
          این دکمه‌ها موجودی را بدون هیچ تراکنشِ بانکی بالا می‌برند و فقط برای
          راه‌اندازی و آزمایش‌اند.
        </p>
      </div>

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
              disabled={loading}
            >
              {loading && selected?.id === plan.id ? 'در حال شارژ…' : `شارژ ${usd(plan.amount)}`}
            </button>
          </div>
        ))}
      </div>
    </Layout>
  );
}
