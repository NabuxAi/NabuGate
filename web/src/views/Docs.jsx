import React, { useState } from 'react';
import '../styles/landing.css';

export default function Docs({ lang = 'fa' }) {
  const isFa = lang === 'fa';
  const [activeTab, setActiveTab] = useState('intro');

  const tabs = [
    { id: 'intro', title: isFa ? 'معرفی' : 'Introduction' },
    { id: 'env', title: isFa ? 'متغیرهای محیطی' : 'Environment Variables' },
    { id: 'cursor', title: isFa ? 'اتصال به Cursor' : 'Cursor Integration' },
    { id: 'claude-code', title: isFa ? 'اتصال به Claude Code' : 'Claude Code Integration' },
    { id: 'cline', title: isFa ? 'اتصال به Cline' : 'Cline Integration' },
    { id: 'sdk', title: isFa ? 'کتابخانه رسمی (SDK)' : 'Official SDKs' },
  ];

  return (
    <div className={`landing-body ${isFa ? 'rtl' : 'ltr'}`} dir={isFa ? 'rtl' : 'ltr'}>
      <header className="jv-header">
        <div className="jv-container" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', height: '4rem' }}>
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
            <a href="/">{isFa ? 'صفحه اصلی' : 'Home'}</a>
            <a href="/plans">{isFa ? 'پلن‌ها' : 'Pricing'}</a>
            <a href="/models">{isFa ? 'مدل‌ها' : 'Models'}</a>
            <a href="/docs" className="active">{isFa ? 'مستندات' : 'Docs'}</a>
          </nav>
          <div className="jv-actions">
            <a href="/panel/login" className="jv-btn jv-btn-primary">
              {isFa ? 'شروع استفاده' : 'Get Started'}
            </a>
          </div>
        </div>
      </header>
      
      <main style={{ flex: 1, display: 'flex', maxWidth: 1200, margin: '0 auto', width: '100%', padding: '40px 20px', gap: '40px' }}>
        {/* Sidebar */}
        <aside style={{ width: '250px', flexShrink: 0, borderRight: isFa ? 'none' : '1px solid rgba(255,255,255,0.1)', borderLeft: isFa ? '1px solid rgba(255,255,255,0.1)' : 'none', paddingRight: isFa ? 0 : 20, paddingLeft: isFa ? 20 : 0 }}>
          <h3 style={{ fontSize: 14, color: '#9ca3af', marginBottom: 16, textTransform: 'uppercase', letterSpacing: '1px' }}>{isFa ? 'فهرست مستندات' : 'Documentation'}</h3>
          <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'flex', flexDirection: 'column', gap: 8 }}>
            {tabs.map(tab => (
              <li key={tab.id}>
                <button 
                  onClick={() => setActiveTab(tab.id)}
                  style={{
                    width: '100%', textAlign: isFa ? 'right' : 'left', padding: '8px 12px',
                    background: activeTab === tab.id ? 'rgba(59, 130, 246, 0.1)' : 'transparent',
                    color: activeTab === tab.id ? '#60a5fa' : '#d1d5db',
                    border: 'none', borderRadius: 6, cursor: 'pointer', fontSize: 14,
                    fontWeight: activeTab === tab.id ? 600 : 400, transition: 'all 0.2s'
                  }}
                >
                  {tab.title}
                </button>
              </li>
            ))}
          </ul>
        </aside>

        {/* Content */}
        <div style={{ flex: 1, color: 'rgba(255, 255, 255, 0.8)', lineHeight: 1.8 }}>
          {activeTab === 'intro' && (
            <div className="docs-content fade-in">
              <h1 style={{ fontSize: 32, marginBottom: 24, fontWeight: 'bold', color: '#fff' }}>{isFa ? 'مستندات و آموزش اتصال' : 'Documentation & Integration'}</h1>
              <p style={{ color: 'rgba(255, 255, 255, 0.6)', marginBottom: 24, fontSize: 16 }}>
                درگاه NabuGate کاملاً با استاندارد OpenAI سازگار است. برای استفاده از آن کافی‌ست آدرس پایه (Base URL) و کلید دسترسی (API Key) خود را در هر ابزاری که از OpenAI پشتیبانی می‌کند وارد کنید.
              </p>
              <div style={{ padding: 16, background: 'rgba(59, 130, 246, 0.1)', border: '1px solid rgba(59, 130, 246, 0.2)', borderRadius: 8, color: '#93c5fd' }}>
                <strong>نکته مهم:</strong> برای اتصال به سرویس‌های ما، شما هیچ نیازی به تغییر کد یا نصب پکیج‌های جدید ندارید. فقط کافیست Endpoint مربوط به OpenAI را در کد خود تغییر دهید.
              </div>
            </div>
          )}

          {activeTab === 'env' && (
            <div className="docs-content fade-in">
              <h1 style={{ fontSize: 28, marginBottom: 24, color: '#fff' }}>متغیرهای محیطی سیستم (Environment Variables)</h1>
              <p style={{ marginBottom: 16 }}>
                ساده‌ترین راه برای تنظیم سراسری در اکثر ابزارها و SDK ها، استفاده از متغیرهای محیطی است:
              </p>
              <pre dir="ltr" style={{ background: 'rgba(0,0,0,0.3)', padding: 16, borderRadius: 8, border: '1px solid rgba(255,255,255,0.1)', fontSize: 14, color: '#fff' }}>
export OPENAI_BASE_URL="https://gate.nabuxai.com/v1"
export OPENAI_API_KEY="ng_xxxxxxxxxxxxxxxxxxxx"
              </pre>
            </div>
          )}

          {activeTab === 'cursor' && (
            <div className="docs-content fade-in">
              <h1 style={{ fontSize: 28, marginBottom: 24, color: '#fff' }}>اتصال به Cursor</h1>
              <ol style={{ paddingInlineStart: 24, display: 'flex', flexDirection: 'column', gap: 12 }}>
                <li>تنظیمات (Settings) Cursor را باز کنید.</li>
                <li>به بخش <strong>Models</strong> بروید.</li>
                <li>در قسمت <strong>OpenAI API Key</strong> کلید دریافتی خود را وارد کنید.</li>
                <li>گزینه <strong>Override OpenAI Base URL</strong> را فعال کنید و آدرس <code>https://gate.nabuxai.com/v1</code> را وارد نمایید.</li>
                <li>حالا می‌توانید نام مدل‌های دلخواه خود (مثل <code>gpt-4o</code> یا <code>claude-3-5-sonnet</code>) را تایپ کرده و استفاده کنید.</li>
              </ol>
            </div>
          )}

          {activeTab === 'claude-code' && (
            <div className="docs-content fade-in">
              <h1 style={{ fontSize: 28, marginBottom: 24, color: '#fff' }}>اتصال به Claude Code</h1>
              <p style={{ marginBottom: 16 }}>
                در محیط ترمینال کافی‌ست از سویچ‌های خط فرمان استفاده کنید یا تنظیمات سراسری انجام دهید:
              </p>
              <pre dir="ltr" style={{ background: 'rgba(0,0,0,0.3)', padding: 16, borderRadius: 8, border: '1px solid rgba(255,255,255,0.1)', fontSize: 14, color: '#fff' }}>
claude -p openai -m claude-3-5-sonnet
              </pre>
              <p style={{ marginTop: 16 }}>
                دقت کنید متغیرهای محیطی که در بالا اشاره شد باید در ترمینال شما در دسترس باشند.
              </p>
            </div>
          )}
          
          {activeTab === 'cline' && (
            <div className="docs-content fade-in">
              <h1 style={{ fontSize: 28, marginBottom: 24, color: '#fff' }}>اتصال به Cline (افزونه VS Code)</h1>
              <ol style={{ paddingInlineStart: 24, display: 'flex', flexDirection: 'column', gap: 12 }}>
                <li>در تنظیمات افزونه Cline، بخش <strong>API Provider</strong> را روی <code>OpenAI Compatible</code> قرار دهید.</li>
                <li>فیلد <strong>Base URL</strong> را روی <code>https://gate.nabuxai.com/v1</code> تنظیم کنید.</li>
                <li>کلید دسترسی را در فیلد <strong>API Key</strong> قرار دهید.</li>
                <li>نام مدل (Model ID) را دقیقاً مطابق با لیست مدل‌های ما وارد کنید.</li>
              </ol>
            </div>
          )}

          {activeTab === 'sdk' && (
            <div className="docs-content fade-in">
              <h1 style={{ fontSize: 28, marginBottom: 24, color: '#fff' }}>استفاده در کتابخانه رسمی Python / Node</h1>
              <pre dir="ltr" style={{ background: 'rgba(0,0,0,0.3)', padding: 16, borderRadius: 8, border: '1px solid rgba(255,255,255,0.1)', fontSize: 14, color: '#fff' }}>
{`// Javascript / Node.js
import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "https://gate.nabuxai.com/v1",
  apiKey: "ng_xxxxxxxx",
});

const response = await client.chat.completions.create({
  model: "claude-3-5-sonnet",
  messages: [{ role: "user", content: "سلام!" }],
});`}
              </pre>
            </div>
          )}
        </div>
      </main>
      
      <footer className="jv-footer">
        <div className="jv-container">
          <p>&copy; {new Date().getFullYear()} NabuGate. {isFa ? 'تمامی حقوق محفوظ است.' : 'All rights reserved.'}</p>
        </div>
      </footer>
    </div>
  );
}
