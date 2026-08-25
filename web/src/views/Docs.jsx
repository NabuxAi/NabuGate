import React from 'react';
import '../styles/landing.css';

export default function Docs({ lang = 'fa' }) {
  const isFa = lang === 'fa';
  
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
            <a href="/">{isFa ? 'صفحه اصلی' : 'Home'}</a>
            <a href="/plans">{isFa ? 'پلن‌ها' : 'Pricing'}</a>
            <a href="/models">{isFa ? 'مدل‌ها' : 'Models'}</a>
            <a href="/docs" className="active">{isFa ? 'مستندات' : 'Docs'}</a>
          </nav>
          <div className="jv-actions">
            <a href="/panel/login" className="jv-btn jv-btn-primary">
              {isFa ? 'شروع استفاده' : 'Get Started'}
              <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M7 17V7h10"></path><path d="M17 17 7 7"></path></svg>
            </a>
          </div>
        </div>
      </header>
      
      <main style={{ flex: 1, padding: '40px 20px', background: '#0a0e17' }}>
        <div style={{ maxWidth: 800, margin: '0 auto', color: 'rgba(255, 255, 255, 0.8)', lineHeight: 1.8 }}>
          <h1 style={{ fontSize: 32, marginBottom: 24, fontWeight: 'bold', color: '#fff' }}>مستندات و آموزش اتصال</h1>
          <p style={{ color: 'rgba(255, 255, 255, 0.6)', marginBottom: 40, fontSize: 16 }}>
            درگاه NabuGate کاملاً با استاندارد OpenAI سازگار است. برای استفاده از آن کافی‌ست آدرس پایه (Base URL) و کلید دسترسی (API Key) خود را در هر ابزاری که از OpenAI پشتیبانی می‌کند وارد کنید.
          </p>

          <section style={{ marginBottom: 40 }} id="env">
            <h2 style={{ fontSize: 24, marginBottom: 16, paddingBottom: 8, borderBottom: '1px solid rgba(255,255,255,0.1)', color: '#fff' }}>متغیرهای محیطی سیستم</h2>
            <p style={{ marginBottom: 16 }}>
              ساده‌ترین راه برای تنظیم سراسری در اکثر ابزارها و SDK ها:
            </p>
            <pre dir="ltr" style={{ background: 'rgba(255,255,255,0.03)', padding: 16, borderRadius: 8, border: '1px solid rgba(255,255,255,0.1)', fontSize: 14, color: '#fff' }}>
export OPENAI_BASE_URL="https://gate.nabuxai.com/v1"
export OPENAI_API_KEY="ng_xxxxxxxxxxxxxxxxxxxx"
            </pre>
          </section>

          <section style={{ marginBottom: 40 }} id="cursor">
            <h2 style={{ fontSize: 24, marginBottom: 16, paddingBottom: 8, borderBottom: '1px solid rgba(255,255,255,0.1)', color: '#fff' }}>اتصال به Cursor</h2>
            <ol style={{ paddingInlineStart: 24, display: 'flex', flexDirection: 'column', gap: 12 }}>
              <li>تنظیمات (Settings) Cursor را باز کنید.</li>
              <li>به بخش <strong>Models</strong> بروید.</li>
              <li>در قسمت <strong>OpenAI API Key</strong> کلید دریافتی خود را وارد کنید.</li>
              <li>گزینه <strong>Override OpenAI Base URL</strong> را فعال کنید و آدرس <code>https://gate.nabuxai.com/v1</code> را وارد نمایید.</li>
              <li>حالا می‌توانید نام مدل‌های دلخواه خود (مثل <code>gpt-4o</code> یا <code>claude-3-5-sonnet</code>) را تایپ کرده و استفاده کنید.</li>
            </ol>
          </section>

          <section style={{ marginBottom: 40 }} id="claude-code">
            <h2 style={{ fontSize: 24, marginBottom: 16, paddingBottom: 8, borderBottom: '1px solid rgba(255,255,255,0.1)', color: '#fff' }}>اتصال به Claude Code</h2>
            <p style={{ marginBottom: 16 }}>
              در محیط ترمینال کافی‌ست از سویچ‌های خط فرمان استفاده کنید یا تنظیمات سراسری انجام دهید:
            </p>
            <pre dir="ltr" style={{ background: 'rgba(255,255,255,0.03)', padding: 16, borderRadius: 8, border: '1px solid rgba(255,255,255,0.1)', fontSize: 14, color: '#fff' }}>
claude -p openai -m claude-3-5-sonnet
            </pre>
            <p style={{ marginTop: 16 }}>
              دقت کنید متغیرهای محیطی که در بالا اشاره شد باید در ترمینال شما در دسترس باشند.
            </p>
          </section>
          
          <section style={{ marginBottom: 40 }} id="cline">
            <h2 style={{ fontSize: 24, marginBottom: 16, paddingBottom: 8, borderBottom: '1px solid rgba(255,255,255,0.1)', color: '#fff' }}>اتصال به Cline (افزونه VS Code)</h2>
            <ol style={{ paddingInlineStart: 24, display: 'flex', flexDirection: 'column', gap: 12 }}>
              <li>در تنظیمات افزونه Cline، بخش <strong>API Provider</strong> را روی <code>OpenAI Compatible</code> قرار دهید.</li>
              <li>فیلد <strong>Base URL</strong> را روی <code>https://gate.nabuxai.com/v1</code> تنظیم کنید.</li>
              <li>کلید دسترسی را در فیلد <strong>API Key</strong> قرار دهید.</li>
              <li>نام مدل (Model ID) را دقیقاً مطابق با لیست مدل‌های ما وارد کنید.</li>
            </ol>
          </section>

          <section style={{ marginBottom: 40 }} id="sdk">
            <h2 style={{ fontSize: 24, marginBottom: 16, paddingBottom: 8, borderBottom: '1px solid rgba(255,255,255,0.1)', color: '#fff' }}>استفاده در کتابخانه رسمی Python / Node</h2>
            <pre dir="ltr" style={{ background: 'rgba(255,255,255,0.03)', padding: 16, borderRadius: 8, border: '1px solid rgba(255,255,255,0.1)', fontSize: 14, color: '#fff' }}>
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
          </section>
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
