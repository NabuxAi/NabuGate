import React from 'react';

export default function Landing({ lang = 'fa' }) {
  const isFa = lang === 'fa';
  
  return (
    <div className={`landing-page ${isFa ? 'rtl' : 'ltr'}`} dir={isFa ? 'rtl' : 'ltr'}>
      {/* Background Glow Effects */}
      <div className="landing-bg-glow glow-1"></div>
      <div className="landing-bg-glow glow-2"></div>

      <header className="landing-header glass">
        <div className="logo">
          <div className="logo-icon">✨</div>
          <span style={{ fontWeight: 800 }}>NabuGate</span>
        </div>
        <nav>
          <a href="#/pricing" className="nav-link">{isFa ? 'قیمت‌گذاری' : 'Pricing'}</a>
          <a href="#/docs" className="nav-link">{isFa ? 'مستندات' : 'Docs'}</a>
          <a href="#/dashboard" className="btn-primary-outline">{isFa ? 'ورود به کنسول' : 'Go to Console'}</a>
        </nav>
      </header>
      
      <main className="landing-main">
        <section className="hero">
          <div className="hero-badge">
            <span className="pulse-dot"></span>
            {isFa ? 'نسخه ۲.۰ منتشر شد' : 'Version 2.0 is live'}
          </div>
          <h1>
            {isFa ? 'دروازهٔ هوشمند' : 'The Smart Gateway'}
            <br />
            <span className="text-gradient">{isFa ? 'به دنیای هوش مصنوعی' : 'To the AI World'}</span>
          </h1>
          <p className="hero-subtitle">
            {isFa 
              ? 'با یکپارچه‌سازی بی‌نظیر، تنها با یک کلید API به تمامی مدل‌های پیشرفته (OpenAI, Anthropic, Gemini, Groq) متصل شوید و هزینه‌ها را بهینه‌سازی کنید.' 
              : 'Seamlessly connect to all advanced models (OpenAI, Anthropic, Gemini, Groq) with a single API key and optimize your costs.'}
          </p>
          <div className="hero-actions">
            <a href="#/dashboard" className="btn-primary-glow">
              {isFa ? 'ساخت کلید API رایگان' : 'Create Free API Key'}
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" style={{ marginLeft: isFa ? 0 : 8, marginRight: isFa ? 8 : 0 }}>
                <path d={isFa ? "M19 12H5M12 19L5 12L12 5" : "M5 12H19M12 5L19 12L12 19"} stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/>
              </svg>
            </a>
            <a href="#/docs" className="btn-secondary-glass">
              {isFa ? 'مستندات و راهنما' : 'Documentation & Guide'}
            </a>
          </div>
          
          <div className="hero-mockup glass">
            <div className="mockup-header">
              <span className="dot bg-red"></span>
              <span className="dot bg-yellow"></span>
              <span className="dot bg-green"></span>
              <div className="mockup-title">bash</div>
            </div>
            <pre><code><span className="dim">$</span> export OPENAI_API_BASE="https://gate.nabuxai.com/v1"
<span className="dim">$</span> export OPENAI_API_KEY="ng-********************"
<span className="dim">$</span> claude
<span className="highlight">Claude Code is now connected through NabuGate!</span></code></pre>
          </div>
        </section>

        <section className="features-grid">
          <div className="feature-card glass">
            <div className="feature-icon bg-blue">💳</div>
            <h3>{isFa ? 'پرداخت منعطف (دلاری و تومنی)' : 'Flexible Payment (USD & Toman)'}</h3>
            <p>{isFa ? 'پشتیبانی یکپارچه از پرداخت ریالی از طریق NabuPay و پرداخت ارزی برای کاربران بین‌المللی.' : 'Seamlessly pay with NabuPay for Toman or use USD.'}</p>
          </div>
          <div className="feature-card glass">
            <div className="feature-icon bg-purple">⚡</div>
            <h3>{isFa ? 'مسیردهی هوشمند (Fallback)' : 'Smart Routing (Fallback)'}</h3>
            <p>{isFa ? 'آلیاس‌سازی مدل‌ها و سوئیچ خودکار بین پروایدرها در صورت قطعی، تضمین آپتایم ۱۰۰٪.' : 'Model aliasing and automatic Fallback routing for zero downtime.'}</p>
          </div>
          <div className="feature-card glass">
            <div className="feature-icon bg-green">📊</div>
            <h3>{isFa ? 'شفافیت کامل مصرف' : 'Full Usage Transparency'}</h3>
            <p>{isFa ? 'داشبوردی حرفه‌ای برای مدیریت دقیق هزینه‌ها و مصرف توکن به تفکیک پروژه، کلید و اعضای تیم.' : 'Accurate cost and token usage tracking per project and user.'}</p>
          </div>
        </section>

        <section className="faq-section">
          <h2 className="section-title">{isFa ? 'سوالات متداول' : 'FAQ'}</h2>
          
          <div className="faq-item glass">
            <details>
              <summary>
                {isFa ? 'آیا با Claude Code کار می‌کند؟' : 'Does it work with Claude Code?'}
                <span className="icon-plus">+</span>
              </summary>
              <div className="faq-content">
                {isFa ? 'بله. کافی است متغیرهای محیطی سازگار با OpenAI (آدرس پایه درگاه و کلید API) را تنظیم کنید تا Claude Code درخواست‌ها را از طریق درگاه ارسال کند.' : 'Yes. Just set the OpenAI-compatible environment variables.'}
              </div>
            </details>
          </div>
          
          <div className="faq-item glass">
            <details>
              <summary>
                {isFa ? 'پلن‌های توکنی چه هستند؟' : 'What are token plans?'}
                <span className="icon-plus">+</span>
              </summary>
              <div className="faq-content">
                {isFa ? 'هر پلن مقدار مشخصی توکن و مدت اعتبار دارد. مصرف هر درخواست از موجودی توکن شما کم می‌شود و در کنسول قابل پیگیری است.' : 'Each plan gives you a certain amount of tokens.'}
              </div>
            </details>
          </div>
        </section>
      </main>

      <footer className="landing-footer glass">
        <p>&copy; {new Date().getFullYear()} NabuGate. {isFa ? 'تمامی حقوق محفوظ است.' : 'All rights reserved.'}</p>
        <div className="lang-switcher">
          <a href="/">English</a> <span className="dim">|</span> <a href="/fa">فارسی</a>
        </div>
      </footer>
    </div>
  );
}
