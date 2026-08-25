import { navigate } from "../nav.js";
import { useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';

export default function Plans() {
  const [loading, setLoading] = useState(false);
  const [gatewayOpen, setGatewayOpen] = useState(false);
  const [selectedPlan, setSelectedPlan] = useState(null);
  const [selectedGateway, setSelectedGateway] = useState('zarinpal');
  const [successMsg, setSuccessMsg] = useState('');

  const plans = [
    { id: 'starter', name: 'بسته پایه', amount: 100000, label: '۱۰۰,۰۰۰ تومان', tokens: 'پایه' },
    { id: 'pro', name: 'بسته حرفه‌ای', amount: 500000, label: '۵۰۰,۰۰۰ تومان', tokens: 'اقتصادی', popular: true },
    { id: 'ultra', name: 'بسته نامحدود', amount: 2000000, label: '۲,۰۰۰,۰۰۰ تومان', tokens: 'تجاری' },
  ];

  const gateways = [
    { id: 'zarinpal', name: 'زرین‌پال', icon: '💳' },
    { id: 'payir', name: 'شبکه پرداخت پی', icon: '🏦' },
    { id: 'nextpay', name: 'نکست‌پی', icon: '🚀' },
    { id: 'mellat', name: 'درگاه ملت', icon: '🏛️' },
  ];

  const handleBuy = (plan) => {
    setSelectedPlan(plan);
    setGatewayOpen(true);
    setSuccessMsg('');
  };

  const handlePay = async () => {
    setLoading(true);
    try {
      // Simulate NabuPay gateway processing time
      await new Promise(r => setTimeout(r, 1500));
      await api.rechargeMe(selectedPlan.amount);
      setSuccessMsg(`پرداخت از طریق درگاه ${gateways.find(g => g.id === selectedGateway).name} با موفقیت انجام شد. مبلغ ${selectedPlan.amount.toLocaleString()} تومان افزوده شد.`);
      setGatewayOpen(false);
      // Refresh after a bit
      setTimeout(() => navigate('dashboard'), 2500);
    } catch (e) {
      alert('خطا در پرداخت: ' + e.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Layout title="پلن‌ها" subtitle="خرید و ارتقای اشتراک">
      {successMsg && (
        <div className="card" style={{ padding: 16, marginBottom: 24, background: 'var(--ng-ok-soft)', color: 'var(--ng-ok-text)', border: '1px solid var(--ng-ok)' }}>
          {successMsg}
        </div>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', gap: 24, padding: '16px 0' }}>
        {plans.map(plan => (
          <div key={plan.id} className="card" style={{ padding: 32, display: 'flex', flexDirection: 'column', position: 'relative', border: plan.popular ? '1px solid var(--ng-fg)' : undefined }}>
            {plan.popular && (
              <div style={{ position: 'absolute', top: -12, left: '50%', transform: 'translateX(-50%)', background: 'var(--ng-fg)', color: 'var(--ng-bg)', padding: '4px 12px', borderRadius: 12, fontSize: 12, fontWeight: 'bold' }}>
                پیشنهاد ویژه
              </div>
            )}
            <h3 style={{ fontSize: 20, marginBottom: 8, fontWeight: 700 }}>{plan.name}</h3>
            <div style={{ fontSize: 32, fontWeight: 800, margin: '16px 0', color: 'var(--ng-fg)' }}>
              {plan.label}
            </div>
            <ul style={{ listStyle: 'none', padding: 0, margin: '0 0 32px 0', color: 'var(--ng-muted)', flex: 1 }}>
              <li style={{ marginBottom: 12 }}>✓ مدل پرداخت بر اساس مصرف</li>
              <li style={{ marginBottom: 12 }}>✓ دسترسی به تمامی مدل‌ها</li>
              <li style={{ marginBottom: 12 }}>✓ رده مصرف: {plan.tokens}</li>
            </ul>
            <button 
              className={`btn ${plan.popular ? 'btn-primary' : ''}`} 
              style={!plan.popular ? { border: '1px solid var(--ng-border)', background: 'transparent', color: 'var(--ng-fg)' } : {}}
              onClick={() => handleBuy(plan)}
            >
              انتخاب و پرداخت
            </button>
          </div>
        ))}
      </div>

      {/* Simulated NabuPay Gateway */}
      {gatewayOpen && (
        <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.8)', backdropFilter: 'blur(4px)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000 }}>
          <div className="card" style={{ width: 450, maxWidth: '95%', padding: 0, overflow: 'hidden' }}>
            <div style={{ background: '#0e121b', padding: '20px', borderBottom: '1px solid var(--ng-border)', textAlign: 'center' }}>
              <div style={{ fontSize: 24, fontWeight: 800, color: '#3b82f6', letterSpacing: '-1px' }}>NabuPay</div>
              <div style={{ fontSize: 12, color: 'var(--ng-muted)', marginTop: 4 }}>انتخاب درگاه پرداخت</div>
            </div>
            <div style={{ padding: '24px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
                <span style={{ color: 'var(--ng-muted)' }}>مبلغ قابل پرداخت:</span>
                <span style={{ fontWeight: 800, fontSize: 18, color: 'var(--ng-fg)' }}>{selectedPlan.amount.toLocaleString()} <span style={{ fontSize: 12, fontWeight: 400 }}>تومان</span></span>
              </div>
              
              <div style={{ marginBottom: 24 }}>
                <p style={{ color: 'var(--ng-muted)', fontSize: 14, marginBottom: 12 }}>درگاه پرداخت مورد نظر خود را انتخاب کنید:</p>
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                  {gateways.map(g => (
                    <button
                      key={g.id}
                      onClick={() => setSelectedGateway(g.id)}
                      style={{
                        padding: '12px',
                        background: selectedGateway === g.id ? 'var(--ng-fg)' : 'transparent',
                        color: selectedGateway === g.id ? 'var(--ng-bg)' : 'var(--ng-heading)',
                        border: selectedGateway === g.id ? '1px solid var(--ng-fg)' : '1px solid var(--ng-border)',
                        borderRadius: 8,
                        cursor: 'pointer',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        gap: 8,
                        fontWeight: selectedGateway === g.id ? 'bold' : 'normal',
                        transition: 'all 0.2s'
                      }}
                    >
                      <span>{g.icon}</span>
                      {g.name}
                    </button>
                  ))}
                </div>
              </div>
              
              <button 
                onClick={handlePay} 
                className="btn btn-primary" 
                style={{ width: '100%', padding: '12px', fontSize: 16, display: 'flex', justifyContent: 'center', alignItems: 'center' }}
                disabled={loading}
              >
                {loading ? 'در حال انتقال به درگاه...' : 'تایید و انتقال به درگاه بانکی'}
              </button>
              <button 
                onClick={() => setGatewayOpen(false)} 
                className="btn" 
                style={{ width: '100%', padding: '12px', marginTop: 12, background: 'transparent', color: 'var(--ng-muted)' }}
                disabled={loading}
              >
                انصراف
              </button>
            </div>
          </div>
        </div>
      )}
    </Layout>
  );
}
