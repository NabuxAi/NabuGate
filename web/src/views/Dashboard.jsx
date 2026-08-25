import { navigate } from "../nav.js";
import { useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';

export default function Dashboard() {
  const [data, setData] = useState(null);
  const [tokens, setTokens] = useState([]);
  const [error, setError] = useState(null);
  const [user, setUser] = useState(null);
  const [showOnboarding, setShowOnboarding] = useState(false);
  const [onboardingStep, setOnboardingStep] = useState(1);

  const load = () => {
    api.overview().then(setData).catch((e) => setError(e.message));
    api.listTokens().then((r) => setTokens(r.tokens || [])).catch(() => {});
    api.getMe().then(setUser).catch(() => {});
    
    if (!localStorage.getItem('nabugate_onboarding_done')) {
      setShowOnboarding(true);
    }
  };

  useEffect(load, []);

  const closeOnboarding = () => {
    localStorage.setItem('nabugate_onboarding_done', 'true');
    setShowOnboarding(false);
  };

  const usage = data?.usage || {};
  const rows = Object.entries(usage).sort((a, b) => (b[1].requests || 0) - (a[1].requests || 0));
  const total = rows.reduce(
    (a, [, v]) => ({
      requests: a.requests + (v.requests || 0),
      tokens: a.tokens + (v.prompt_tokens || 0) + (v.completion_tokens || 0),
      cost: a.cost + (v.cost_usd || 0),
    }),
    { requests: 0, tokens: 0, cost: 0 }
  );

  const activeKeys = tokens.length;
  const balance = user ? user.balance || 0 : 0;
  const username = user?.name || user?.email?.split('@')[0] || "کاربر";

  return (
    <Layout title="داشبورد" subtitle="">
      {error && <div className="card banner-error">{error}</div>}

      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 32 }}>
        <div>
          <h2 style={{ fontSize: 24, fontWeight: 800, margin: 0, color: 'var(--ng-heading)' }}>سلام، {username} 👋</h2>
          <p style={{ color: 'var(--ng-muted)', fontSize: 13, marginTop: 4 }}>نمای کلی حساب، اشتراک و مصرف شما.</p>
        </div>
        <div style={{ display: 'flex', gap: 12 }}>
          <button className="btn" style={{ background: 'transparent', border: '1px solid var(--ng-border)', color: 'var(--ng-heading)' }} onClick={() => navigate('plans')}>
            + خرید پلن / شارژ
          </button>
          <button className="btn btn-primary" onClick={() => navigate('tokens')}>
            🔑 ساخت کلید
          </button>
        </div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', gap: 20, marginBottom: 24 }}>
        <div className="card" style={{ padding: 24, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', textAlign: 'center', minHeight: 220, gridColumn: 'span 2' }}>
          <h3 style={{ fontSize: 16, margin: '0 0 16px 0', alignSelf: 'flex-start', borderBottom: '1px solid var(--ng-border)', paddingBottom: 12, width: '100%', textAlign: 'right' }}>موجودی حساب</h3>
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center' }}>
            <div style={{ fontSize: 40, fontWeight: 800, marginBottom: 8, color: 'var(--ng-fg)' }}>
              {balance.toLocaleString('fa-IR')} <span style={{ fontSize: 16, fontWeight: 400, color: 'var(--ng-muted)' }}>تومان</span>
            </div>
            <p style={{ fontSize: 13, color: 'var(--ng-muted)', marginBottom: 20 }}>از طریق NabuPay می‌توانید حساب خود را شارژ کنید یا پلن تهیه کنید.</p>
            <button onClick={() => navigate('plans')} className="btn btn-primary">شارژ موجودی (NabuPay)</button>
          </div>
        </div>

        <div className="card" style={{ padding: 24, display: 'flex', flexDirection: 'column' }}>
          <h3 style={{ fontSize: 16, margin: '0 0 20px 0', borderBottom: '1px solid var(--ng-border)', paddingBottom: 12 }}>وضعیت حساب</h3>
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 16 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <span style={{ fontSize: 13, color: 'var(--ng-muted)' }}>ایمیل</span>
              <span style={{ fontSize: 13, fontWeight: 700 }} dir="ltr">{user?.email || "..."}</span>
            </div>
          </div>
          <button style={{ marginTop: 20, background: 'transparent', border: '1px solid var(--ng-border)', color: 'var(--ng-heading)', padding: '10px', borderRadius: 8, fontSize: 13, cursor: 'pointer', width: '100%' }} onClick={() => navigate('profile')}>
            تنظیمات
          </button>
        </div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: 20, marginBottom: 24 }}>
        <div className="card" style={{ padding: 24, display: 'flex', flexDirection: 'column', alignItems: 'center', textAlign: 'center' }}>
          <div style={{ color: 'var(--ng-muted)', fontSize: 13, marginBottom: 8 }}>مصرف ۳۰ روز اخیر</div>
          <div style={{ fontSize: 24, fontWeight: 800 }}>{Number(total.tokens).toLocaleString('fa-IR')} <span style={{ fontSize: 12, color: 'var(--ng-muted)', fontWeight: 400 }}>توکن</span></div>
        </div>
        <div className="card" style={{ padding: 24, display: 'flex', flexDirection: 'column', alignItems: 'center', textAlign: 'center' }}>
          <div style={{ color: 'var(--ng-muted)', fontSize: 13, marginBottom: 8 }}>درخواست‌ها (۳۰ روز)</div>
          <div style={{ fontSize: 24, fontWeight: 800 }}>{Number(total.requests).toLocaleString('fa-IR')}</div>
        </div>
        <div className="card" style={{ padding: 24, display: 'flex', flexDirection: 'column', alignItems: 'center', textAlign: 'center' }}>
          <div style={{ color: 'var(--ng-muted)', fontSize: 13, marginBottom: 8 }}>کلیدهای فعال</div>
          <div style={{ fontSize: 24, fontWeight: 800 }}>{Number(activeKeys).toLocaleString('fa-IR')}</div>
        </div>
      </div>

      {showOnboarding && (
        <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.85)', backdropFilter: 'blur(8px)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 9999 }}>
          <div className="card" style={{ width: 500, maxWidth: '95%', padding: 0, overflow: 'hidden', position: 'relative' }}>
            <div style={{ height: 6, background: 'linear-gradient(to right, #3b82f6, #8b5cf6)' }}></div>
            
            <div style={{ padding: '32px 32px 16px 32px' }}>
              {onboardingStep === 1 && (
                <div className="fade-in">
                  <div style={{ display: 'flex', justifyContent: 'center', marginBottom: 24, fontSize: 48 }}>👋</div>
                  <h2 style={{ fontSize: 24, fontWeight: 800, textAlign: 'center', marginBottom: 12 }}>به کنسول NabuGate خوش آمدید!</h2>
                  <p style={{ color: 'var(--ng-muted)', lineHeight: 1.8, textAlign: 'center' }}>
                    اینجا درگاه مرکزی هوش مصنوعی شماست. می‌توانید به راحتی به تمام مدل‌های هوش مصنوعی (OpenAI, Anthropic, Gemini و ...) با یک کلید دسترسی داشته باشید.
                  </p>
                </div>
              )}

              {onboardingStep === 2 && (
                <div className="fade-in">
                  <div style={{ display: 'flex', justifyContent: 'center', marginBottom: 24, fontSize: 48 }}>🔑</div>
                  <h2 style={{ fontSize: 24, fontWeight: 800, textAlign: 'center', marginBottom: 12 }}>ساخت کلید دسترسی (API Key)</h2>
                  <p style={{ color: 'var(--ng-muted)', lineHeight: 1.8, textAlign: 'center' }}>
                    برای شروع، باید به بخش <strong>«کلیدها»</strong> بروید و یک API Key بسازید. این کلید رو تو هر ابزاری مثل Cursor یا Cline وارد کنید کار می‌کنه!
                  </p>
                </div>
              )}

              {onboardingStep === 3 && (
                <div className="fade-in">
                  <div style={{ display: 'flex', justifyContent: 'center', marginBottom: 24, fontSize: 48 }}>💳</div>
                  <h2 style={{ fontSize: 24, fontWeight: 800, textAlign: 'center', marginBottom: 12 }}>شارژ حساب با NabuPay</h2>
                  <p style={{ color: 'var(--ng-muted)', lineHeight: 1.8, textAlign: 'center' }}>
                    سیستم پرداخت به صورت Pay-as-you-go (پرداخت به ازای مصرف) کار می‌کنه. با استفاده از <strong>NabuPay</strong> می‌تونی حسابتو شارژ کنی و فقط به اندازه توکن‌هایی که مصرف می‌کنی هزینه بدی.
                  </p>
                </div>
              )}
            </div>

            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '16px 32px 32px 32px' }}>
              <div style={{ display: 'flex', gap: 6 }}>
                {[1, 2, 3].map(step => (
                  <div key={step} style={{ width: 8, height: 8, borderRadius: '50%', background: step === onboardingStep ? 'var(--ng-fg)' : 'var(--ng-border)' }} />
                ))}
              </div>
              <div style={{ display: 'flex', gap: 12 }}>
                <button onClick={closeOnboarding} className="btn" style={{ background: 'transparent', color: 'var(--ng-muted)', border: 'none' }}>
                  رد کردن
                </button>
                {onboardingStep < 3 ? (
                  <button onClick={() => setOnboardingStep(s => s + 1)} className="btn btn-primary">
                    مرحله بعد
                  </button>
                ) : (
                  <button onClick={closeOnboarding} className="btn btn-primary">
                    شروع کنید!
                  </button>
                )}
              </div>
            </div>
          </div>
        </div>
      )}
    </Layout>
  );
}
