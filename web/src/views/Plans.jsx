import { navigate } from "../nav.js";
import { useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';

export default function Plans() {
  const [loading, setLoading] = useState(false);
  const [gatewayOpen, setGatewayOpen] = useState(false);
  const [selectedPlan, setSelectedPlan] = useState(null);
  const [successMsg, setSuccessMsg] = useState('');
  const [selectedBank, setSelectedBank] = useState('saman');

  const plans = [
    { id: 'starter', name: 'بسته پایه', amount: 100000, label: '۱۰۰,۰۰۰ تومان', tokens: 'پایه' },
    { id: 'pro', name: 'بسته حرفه‌ای', amount: 500000, label: '۵۰۰,۰۰۰ تومان', tokens: 'اقتصادی', popular: true },
    { id: 'ultra', name: 'بسته نامحدود', amount: 2000000, label: '۲,۰۰۰,۰۰۰ تومان', tokens: 'تجاری' },
  ];

  const banks = [
    { id: 'saman', name: 'سامان' },
    { id: 'mellat', name: 'ملت' },
    { id: 'pasargad', name: 'پاسارگاد' },
    { id: 'zarinpal', name: 'زرین‌پال' },
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
      const bankName = banks.find(b => b.id === selectedBank)?.name;
      setSuccessMsg(`پرداخت از طریق درگاه یکپارچه NabuPay (درگاه ${bankName}) با موفقیت انجام شد. مبلغ ${selectedPlan.amount.toLocaleString('fa-IR')} تومان افزوده شد.`);
      setGatewayOpen(false);
      // Refresh after a bit
      setTimeout(() => navigate('dashboard'), 3000);
    } catch (e) {
      alert('خطا در پرداخت: ' + e.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Layout title="خرید و شارژ حساب" subtitle="افزایش موجودی برای استفاده از هوش مصنوعی">
      {successMsg && (
        <div className="card" style={{ padding: 16, marginBottom: 24, background: 'var(--ng-ok-soft)', color: 'var(--ng-ok-text)', border: '1px solid var(--ng-ok)' }}>
          {successMsg}
        </div>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', gap: 24, padding: '16px 0' }}>
        {plans.map(plan => (
          <div key={plan.id} className="card" style={{ padding: 32, display: 'flex', flexDirection: 'column', position: 'relative', border: plan.popular ? '2px solid var(--ng-fg)' : undefined }}>
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
              <li style={{ marginBottom: 12 }}>✓ مدل پرداخت Pay-as-you-go</li>
              <li style={{ marginBottom: 12 }}>✓ دسترسی به تمامی مدل‌ها (GPT, Claude, Gemini)</li>
              <li style={{ marginBottom: 12 }}>✓ اولویت پردازش: {plan.tokens}</li>
            </ul>
            <button 
              className={`btn ${plan.popular ? 'btn-primary' : ''}`} 
              style={!plan.popular ? { border: '1px solid var(--ng-border)', background: 'transparent', color: 'var(--ng-fg)' } : {}}
              onClick={() => handleBuy(plan)}
            >
              شارژ حساب
            </button>
          </div>
        ))}
      </div>

      {/* Simulated NabuPay Gateway */}
      {gatewayOpen && (
        <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.85)', backdropFilter: 'blur(8px)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000 }}>
          <div className="card" style={{ width: 450, maxWidth: '95%', padding: 0, overflow: 'hidden', border: '1px solid var(--ng-border)' }}>
            
            <div style={{ background: 'var(--ng-surface-soft)', padding: '24px 24px 16px 24px', textAlign: 'center', borderBottom: '1px solid var(--ng-border)' }}>
              <div style={{ fontSize: 36, marginBottom: 12 }}>💳</div>
              <div style={{ fontSize: 24, fontWeight: 800, color: 'var(--ng-heading)', letterSpacing: '-0.5px' }}>NabuPay</div>
              <div style={{ fontSize: 13, color: 'var(--ng-muted)', marginTop: 4 }}>درگاه امن یکپارچه نبوکس</div>
            </div>
            
            <div style={{ padding: '24px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16, paddingBottom: 16, borderBottom: '1px dashed var(--ng-border)' }}>
                <span style={{ color: 'var(--ng-muted)' }}>مبلغ قابل پرداخت:</span>
                <span style={{ fontWeight: 800, fontSize: 18, color: 'var(--ng-fg)' }}>{selectedPlan.amount.toLocaleString('fa-IR')} <span style={{ fontSize: 12, fontWeight: 400 }}>تومان</span></span>
              </div>
              
              <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 24 }}>
                <span style={{ color: 'var(--ng-muted)' }}>پذیرنده:</span>
                <span style={{ fontWeight: 700, fontSize: 14 }}>پلتفرم NabuGate</span>
              </div>
              
              <div style={{ marginBottom: 24 }}>
                <div style={{ fontSize: 13, color: 'var(--ng-muted)', marginBottom: 12 }}>انتخاب درگاه پرداخت:</div>
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                  {banks.map(b => (
                    <button 
                      key={b.id}
                      onClick={() => setSelectedBank(b.id)}
                      style={{ 
                        padding: '12px', borderRadius: 8, cursor: 'pointer', fontSize: 14, fontWeight: 600,
                        border: selectedBank === b.id ? '2px solid var(--ng-fg)' : '1px solid var(--ng-border)',
                        background: selectedBank === b.id ? 'var(--ng-surface-soft)' : 'transparent',
                        color: 'var(--ng-heading)'
                      }}
                    >
                      {b.name}
                    </button>
                  ))}
                </div>
              </div>
              
              <button 
                onClick={handlePay} 
                disabled={loading}
                className="btn btn-primary" 
                style={{ width: '100%', padding: '14px', fontSize: 16, fontWeight: 700 }}
              >
                {loading ? 'در حال انتقال...' : 'پرداخت و انتقال به درگاه بانکی'}
              </button>
              
              <button 
                onClick={() => setGatewayOpen(false)} 
                disabled={loading}
                className="btn" 
                style={{ width: '100%', marginTop: 12, background: 'transparent', color: 'var(--ng-muted)', border: 'none' }}
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
