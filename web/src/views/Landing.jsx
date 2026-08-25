import React, { useState, useEffect } from 'react';
import * as api from '../api.js';
import '../styles/landing.css';

export default function Landing({ lang = 'fa' }) {
  const isFa = lang === 'fa';
  const [models, setModels] = useState([]);
  const [ping, setPing] = useState(true);

  useEffect(() => {
    api.publicModels().then(names => setModels(names || [])).catch(() => {});
    
    // Simulate ping dot blinking
    const iv = setInterval(() => setPing(p => !p), 1000);
    return () => clearInterval(iv);
  }, []);

  const getModelColor = (name) => {
    const l = name.toLowerCase();
    if (l.includes('gpt')) return { bg: 'rgba(16, 185, 129, 0.16)', border: 'rgba(16, 185, 129, 0.4)' };
    if (l.includes('claude')) return { bg: 'rgba(249, 115, 22, 0.16)', border: 'rgba(249, 115, 22, 0.4)' };
    if (l.includes('gemini')) return { bg: 'rgba(59, 130, 246, 0.16)', border: 'rgba(59, 130, 246, 0.4)' };
    if (l.includes('deepseek')) return { bg: 'rgba(99, 102, 241, 0.16)', border: 'rgba(99, 102, 241, 0.4)' };
    return { bg: 'rgba(255, 255, 255, 0.1)', border: 'rgba(255, 255, 255, 0.2)' };
  };

  const getModelIcon = (name) => {
    const l = name.toLowerCase();
    if (l.includes('gpt')) return <svg viewBox="0 0 24 24" width="16" height="16" xmlns="http://www.w3.org/2000/svg"><g><circle cx="12" cy="12" r="2.4" fill="#fff"></circle><circle cx="12" cy="4.5" r="1.8" fill="#fff" fillOpacity="0.9"></circle><circle cx="5.5" cy="16" r="1.8" fill="#fff" fillOpacity="0.9"></circle><circle cx="18.5" cy="16" r="1.8" fill="#fff" fillOpacity="0.9"></circle><path d="M12 6.3v3.3M10.4 13.4 7 15M13.6 13.4 17 15" stroke="#fff" strokeWidth="1.6" strokeLinecap="round" fill="none"></path></g></svg>;
    if (l.includes('claude')) return <svg viewBox="0 0 24 24" width="16" height="16" xmlns="http://www.w3.org/2000/svg"><g stroke="#fff" strokeWidth="1.6" strokeLinecap="round" fill="none"><path d="M12 3v18M5 6.5l14 11M19 6.5l-14 11"></path></g></svg>;
    if (l.includes('gemini')) return <svg viewBox="0 0 24 24" width="16" height="16" xmlns="http://www.w3.org/2000/svg"><g><path d="M12 3c.4 4.6 1.4 5.6 6 6-4.6.4-5.6 1.4-6 6-.4-4.6-1.4-5.6-6-6 4.6-.4 5.6-1.4 6-6Z" fill="#fff"></path><path d="M18.5 14.5c.2 2 .6 2.4 2.5 2.6-1.9.2-2.3.6-2.5 2.5-.2-1.9-.6-2.3-2.5-2.5 1.9-.2 2.3-.6 2.5-2.6Z" fill="#fff" fillOpacity="0.8"></path></g></svg>;
    return <svg viewBox="0 0 24 24" width="16" height="16" xmlns="http://www.w3.org/2000/svg"><g stroke="#fff" strokeWidth="1.6" strokeLinecap="round" fill="none"><circle cx="12" cy="12" r="6.5"></circle><path d="M5.5 12a6.5 6.5 0 0 0 11 4.6" strokeOpacity="0.5"></path><circle cx="17.4" cy="9" r="1.7" fill="#fff" stroke="none"></circle></g></svg>;
  };

  const displayModels = models.length > 0 ? models.slice(0, 4) : ['gpt-4o', 'claude-3-5-sonnet', 'gemini-1.5-pro', 'deepseek-coder'];

  return (
    <div className={`landing-body ${isFa ? 'rtl' : 'ltr'}`} dir={isFa ? 'rtl' : 'ltr'}>
      <header className="jv-header">
        <div className="jv-container">
          <a href="/" className="jv-logo">
            <svg viewBox="0 0 40 40" width="36" height="36" role="img" aria-label="NabuGate" fill="none" xmlns="http://www.w3.org/2000/svg">
              <rect x="1" y="1" width="38" height="38" rx="11" fill="url(#jv-bg)"></rect>
              <rect x="1" y="1" width="38" height="38" rx="11" stroke="url(#jv-stroke)" strokeWidth="1.2"></rect>
              <circle cx="13" cy="13" r="2.6" fill="#fff" fillOpacity="0.92"></circle>
              <circle cx="13" cy="27" r="2.6" fill="#fff" fillOpacity="0.92"></circle>
              <circle cx="28" cy="20" r="3.4" fill="#fff"></circle>
              <path d="M15.3 14.1 L25.2 18.7 M15.3 25.9 L25.2 21.3" stroke="#fff" strokeOpacity="0.85" strokeWidth="1.5" strokeLinecap="round"></path>
              <defs>
                <linearGradient id="jv-bg" x1="0" y1="0" x2="40" y2="40" gradientUnits="userSpaceOnUse">
                  <stop stopColor="hsl(213 94% 62%)"></stop>
                  <stop offset="1" stopColor="hsl(262 83% 67%)"></stop>
                </linearGradient>
                <linearGradient id="jv-stroke" x1="0" y1="0" x2="40" y2="40" gradientUnits="userSpaceOnUse">
                  <stop stopColor="#fff" stopOpacity="0.5"></stop>
                  <stop offset="1" stopColor="#fff" stopOpacity="0.1"></stop>
                </linearGradient>
              </defs>
            </svg>
            <span>
              <strong>{isFa ? 'نبوگیت' : 'NabuGate'}</strong>
              <small>NabuGate</small>
            </span>
          </a>
          <nav className="jv-nav hidden-mobile">
            <a href="/" className="active">{isFa ? 'صفحه اصلی' : 'Home'}</a>
            <a href="#/plans">{isFa ? 'پلن‌ها' : 'Pricing'}</a>
            <a href="#/models">{isFa ? 'مدل‌ها' : 'Models'}</a>
            <a href="#/docs">{isFa ? 'مستندات' : 'Docs'}</a>
          </nav>
          <div className="jv-actions">
            <a href="#/login" className="jv-btn jv-btn-ghost">
              <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4"></path><polyline points="10 17 15 12 10 7"></polyline><line x1="15" x2="3" y1="12" y2="12"></line></svg>
              {isFa ? 'ورود به کنسول' : 'Login'}
            </a>
            <a href="#/login" className="jv-btn jv-btn-primary">
              {isFa ? 'شروع استفاده' : 'Get Started'}
              <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M7 17V7h10"></path><path d="M17 17 7 7"></path></svg>
            </a>
          </div>
        </div>
      </header>

      <main style={{ flex: 1 }}>
        <section className="jv-hero">
          <div className="jv-bg-grid"></div>
          <div className="jv-container jv-hero-content">
            <div className="jv-hero-text">
              <span className="jv-badge">
                <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" style={{ color: '#8b5cf6' }}><path d="M9.937 15.5A2 2 0 0 0 8.5 14.063l-6.135-1.582a.5.5 0 0 1 0-.962L8.5 9.936A2 2 0 0 0 9.937 8.5l1.582-6.135a.5.5 0 0 1 .963 0L14.063 8.5A2 2 0 0 0 15.5 9.937l6.135 1.581a.5.5 0 0 1 0 .964L15.5 14.063a2 2 0 0 0-1.437 1.437l-1.582 6.135a.5.5 0 0 1-.963 0z"></path><path d="M20 3v4"></path><path d="M22 5h-4"></path><path d="M4 17v2"></path><path d="M5 18H3"></path></svg>
                {isFa ? 'درگاه چندارائه‌دهندهٔ هوش مصنوعی' : 'Multi-provider AI Gateway'}
              </span>
              <h1 className="jv-title">
                {isFa ? 'یک حساب، یک API،' : 'One Account, One API,'}<br/>
                {isFa ? 'دسترسی به ' : 'Access to '} <span className="jv-text-gradient">{isFa ? 'چندین مدل' : 'Multiple Models'}</span> {isFa ? 'هوش مصنوعی' : 'AI'}
              </h1>
              <p className="jv-desc">
                {isFa 
                  ? 'نبوگیت دسترسی به GPT، Claude، Gemini، Groq و DeepSeek را از طریق یک درگاه یکپارچهٔ سازگار با OpenAI فراهم می‌کند. اشتراک، کلید API و مصرف خود را از یک کنسول واحد مدیریت کنید.'
                  : 'NabuGate provides access to GPT, Claude, Gemini, Groq, and DeepSeek via a single OpenAI-compatible gateway. Manage your subscription, API keys, and usage from one console.'}
              </p>
              <div className="jv-hero-btns">
                <a href="#/login" className="jv-btn jv-btn-primary">
                  {isFa ? 'شروع استفاده' : 'Get Started'}
                  <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M7 17V7h10"></path><path d="M17 17 7 7"></path></svg>
                </a>
                <a href="#/plans" className="jv-btn jv-btn-outline">{isFa ? 'مشاهده پلن‌ها' : 'View Plans'}</a>
              </div>
              <div className="jv-tools">
                <span>{isFa ? 'سازگار با ابزارهای محبوب توسعه‌دهندگان:' : 'Compatible with popular developer tools:'}</span>
                <div className="jv-tools-list">
                  <span className="jv-tool-tag" dir="ltr">Claude Code</span>
                  <span className="jv-tool-tag" dir="ltr">Cursor</span>
                  <span className="jv-tool-tag" dir="ltr">Codex</span>
                  <span className="jv-tool-tag" dir="ltr">Cline</span>
                  <span className="jv-tool-tag" dir="ltr">Roo Code</span>
                  <span className="jv-tool-tag" dir="ltr">OpenAI SDK</span>
                </div>
              </div>
            </div>
            
            <div className="jv-mockup-wrapper">
              <div className="jv-mockup">
                <div className="jv-mockup-glow"></div>
                <div className="jv-mockup-header">
                  <div className="jv-dots">
                    <span className="jv-dot-red"></span>
                    <span className="jv-dot-yellow"></span>
                    <span className="jv-dot-green"></span>
                  </div>
                  <span className="jv-endpoint" dir="ltr">https://gate.nabuxai.com/v1</span>
                  <div className="jv-status">
                    <div className="jv-ping"><span className="jv-ping-circle" style={{ opacity: ping ? 1 : 0.2 }}></span></div>
                    {isFa ? 'آنلاین' : 'Online'}
                  </div>
                </div>
                
                <div className="jv-gateway-box">
                  <div className="jv-gb-title">
                    <strong>{isFa ? 'درگاه نبوگیت' : 'NabuGate Gateway'}</strong>
                    <small>{isFa ? 'مسیریابی هوشمند درخواست‌ها' : 'Smart Request Routing'}</small>
                  </div>
                  <span className="jv-bearer" dir="ltr">Bearer ng_••••</span>
                </div>
                
                <div className="jv-models-grid">
                  {displayModels.map((m, i) => (
                    <div className="jv-model-card" key={i}>
                      <div className="jv-model-icon" style={{ background: getModelColor(m).bg, borderColor: getModelColor(m).border }}>
                        {getModelIcon(m)}
                      </div>
                      <div className="jv-model-info" style={{ overflow: 'hidden' }}>
                        <strong dir="ltr" style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{m}</strong>
                        <small>{isFa ? 'فعال' : 'Active'}</small>
                      </div>
                    </div>
                  ))}
                </div>
                
                <div className="jv-code-block">
                  <pre dir="ltr">
<span style={{ color: '#8b5cf6' }}>await</span> client.chat.completions.create(&#123;
  model: <span style={{ color: '#10b981' }}>"{displayModels[0] || 'gpt-4o'}"</span>,
  messages,
&#125;)
                  </pre>
                </div>
              </div>
            </div>
          </div>
        </section>

        <section className="jv-section bg-alt" id="how-it-works">
          <div className="jv-container">
            <div className="jv-section-header">
              <span className="jv-badge">{isFa ? 'چطور کار می‌کند' : 'How it works'}</span>
              <h2>{isFa ? 'در سه گام ساده شروع کنید' : 'Start in 3 easy steps'}</h2>
              <p>{isFa ? 'از ثبت‌نام تا اولین درخواست API، مسیری روشن و بدون پیچیدگی.' : 'From signup to your first API request, a clear and simple path.'}</p>
            </div>
            
            <div className="jv-steps">
              <div className="jv-step">
                <span className="jv-step-num">۱</span>
                <div className="jv-step-icon">
                  <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"></path><circle cx="9" cy="7" r="4"></circle><line x1="19" x2="19" y1="8" y2="14"></line><line x1="22" x2="16" y1="11" y2="11"></line></svg>
                </div>
                <h3>{isFa ? 'ثبت‌نام' : 'Sign Up'}</h3>
                <p>{isFa ? 'در کنسول نبوگیت ثبت‌نام کنید و حساب خود را در چند ثانیه بسازید.' : 'Register in NabuGate console and create your account in seconds.'}</p>
              </div>
              <div className="jv-step">
                <span className="jv-step-num">۲</span>
                <div className="jv-step-icon">
                  <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><rect width="20" height="14" x="2" y="5" rx="2"></rect><line x1="2" x2="22" y1="10" y2="10"></line></svg>
                </div>
                <h3>{isFa ? 'خرید پلن' : 'Buy a Plan'}</h3>
                <p>{isFa ? 'پلن یا بستهٔ توکن متناسب با نیاز خود را انتخاب و از طریق درگاه پرداخت تهیه کنید.' : 'Choose a plan or token package that fits your needs.'}</p>
              </div>
              <div className="jv-step">
                <span className="jv-step-num">۳</span>
                <div className="jv-step-icon">
                  <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M2.586 17.414A2 2 0 0 0 2 18.828V21a1 1 0 0 0 1 1h3a1 1 0 0 0 1-1v-1a1 1 0 0 1 1-1h1a1 1 0 0 0 1-1v-1a1 1 0 0 1 1-1h.172a2 2 0 0 0 1.414-.586l.814-.814a6.5 6.5 0 1 0-4-4z"></path><circle cx="16.5" cy="7.5" r=".5" fill="currentColor"></circle></svg>
                </div>
                <h3>{isFa ? 'دریافت کلید و اتصال' : 'Get Key & Connect'}</h3>
                <p>{isFa ? 'یک کلید API بسازید، آدرس پایهٔ درگاه را تنظیم کنید و ابزارهای خود را وصل کنید.' : 'Create an API key, set the base URL, and connect your tools.'}</p>
              </div>
            </div>
          </div>
        </section>
        
        <section className="jv-section" id="developer-tools">
          <div className="jv-container">
            <div className="jv-section-header">
              <span className="jv-badge">{isFa ? 'ابزارهای توسعه‌دهندگان' : 'Developer Tools'}</span>
              <h2>{isFa ? 'با ابزارهایی که هر روز استفاده می‌کنید کار می‌کند' : 'Works with the tools you use every day'}</h2>
              <p>{isFa ? 'درگاه با استاندارد OpenAI سازگار است؛ کافی است آدرس پایه و کلید را تنظیم کنید.' : 'The gateway is fully compatible with OpenAI standards. Just set your base URL and API key.'}</p>
            </div>
            
            <div className="jv-dev-setup">
              <div className="jv-dev-header">
                <span dir="ltr">bash · متغیرهای محیطی</span>
              </div>
              <div className="jv-dev-body">
                <pre dir="ltr">
export OPENAI_BASE_URL="https://gate.nabuxai.com/v1"
export OPENAI_API_KEY="ng_xxxxxxxxxxxxxxxxxxxx"
                </pre>
              </div>
            </div>
            
            <div className="jv-tools-grid">
              {['Claude Code', 'Codex', 'VS Code', 'Cursor', 'Cline', 'Roo Code', 'OpenAI SDK'].map(t => (
                <a href="#/docs" className="jv-tool-card" key={t}>
                  <div className="jv-tool-icon">
                    <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polyline points="4 17 10 11 4 5"></polyline><line x1="12" x2="20" y1="19" y2="19"></line></svg>
                  </div>
                  <strong dir="ltr">{t}</strong>
                  <small>{isFa ? 'سازگار کامل' : 'Fully Compatible'}</small>
                </a>
              ))}
            </div>
          </div>
        </section>
      </main>

      <footer className="jv-footer">
        <div className="jv-container">
          <p>&copy; {new Date().getFullYear()} NabuGate. {isFa ? 'تمامی حقوق محفوظ است.' : 'All rights reserved.'}</p>
        </div>
      </footer>
    </div>
  );
}
