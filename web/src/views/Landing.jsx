import React from 'react';

export default function Landing({ lang = 'en' }) {
  const isFa = lang === 'fa';
  
  return (
    <div className={`landing-page ${isFa ? 'rtl' : 'ltr'}`} dir={isFa ? 'rtl' : 'ltr'}>
      <header className="landing-header">
        <div className="logo">NabuGate</div>
        <nav>
          <a href="#/pricing">{isFa ? 'قیمت‌گذاری' : 'Pricing'}</a>
          <a href="#/docs">{isFa ? 'مستندات' : 'Docs'}</a>
          <a href="#/dashboard" className="btn-primary">{isFa ? 'ورود به کنسول' : 'Go to Console'}</a>
        </nav>
      </header>
      
      <main className="landing-main">
        <section className="hero">
          <h1>{isFa ? 'دروازهٔ مرکزی هوش مصنوعی' : 'The Central AI Gateway'}</h1>
          <p>{isFa ? 'تنها با یک کلید API به تمامی مدل‌های پیشرفته (OpenAI, Anthropic, Gemini, Groq) متصل شوید.' : 'Connect to all advanced models (OpenAI, Anthropic, Gemini, Groq) with a single API key.'}</p>
          <div className="hero-actions">
            <a href="#/dashboard" className="btn-primary">{isFa ? 'ساخت کلید API' : 'Create API Key'}</a>
            <a href="#/docs" className="btn-secondary">{isFa ? 'سازگار با Claude Code' : 'Compatible with Claude Code'}</a>
          </div>
        </section>

        <section className="features">
          <div className="feature">
            <h3>{isFa ? 'پرداخت آسان (دلاری و تومنی)' : 'Easy Payment (USD & Toman)'}</h3>
            <p>{isFa ? 'پشتیبانی یکپارچه از پرداخت ریالی از طریق NabuPay و پرداخت ارزی.' : 'Seamlessly pay with NabuPay for Toman or use USD.'}</p>
          </div>
          <div className="feature">
            <h3>{isFa ? 'مسیردهی هوشمند' : 'Smart Routing'}</h3>
            <p>{isFa ? 'آلیاس‌سازی مدل‌ها و تعیین Fallback برای جلوگیری از قطعی.' : 'Model aliasing and Fallback routing for zero downtime.'}</p>
          </div>
          <div className="feature">
            <h3>{isFa ? 'شفافیت مصرف' : 'Usage Transparency'}</h3>
            <p>{isFa ? 'مدیریت دقیق هزینه‌ها و مصرف توکن به تفکیک پروژه و کاربر.' : 'Accurate cost and token usage tracking per project and user.'}</p>
          </div>
        </section>
      </main>

      <footer className="landing-footer">
        <p>&copy; {new Date().getFullYear()} NabuGate. {isFa ? 'تمامی حقوق محفوظ است.' : 'All rights reserved.'}</p>
        <div className="lang-switcher">
          <a href="/">English</a> | <a href="/fa">فارسی</a>
        </div>
      </footer>
    </div>
  );
}
