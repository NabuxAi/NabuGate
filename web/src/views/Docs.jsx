import { useState } from 'react';

export default function Docs() {
  const [activeTab, setActiveTab] = useState('intro');
  const isFa = true;

  const tabs = [
    { id: 'intro', title: isFa ? 'معرفی و شروع سریع' : 'Introduction' },
    { id: 'api-reference', title: isFa ? 'مرجع API' : 'API Reference' },
    { id: 'env', title: isFa ? 'متغیرهای محیطی (Env)' : 'Environment Variables' },
    { id: 'cursor', title: isFa ? 'اتصال به Cursor' : 'Cursor Integration' },
    { id: 'cline', title: isFa ? 'اتصال به Cline' : 'Cline Integration' },
    { id: 'claude-code', title: isFa ? 'اتصال به Claude Code' : 'Claude Code' },
    { id: 'sdk', title: isFa ? 'کتابخانه‌های پایتون و نود' : 'Python & Node.js SDK' },
    { id: 'curl', title: isFa ? 'نمونه کدهای cURL' : 'cURL Examples' },
  ];

  return (
    <div style={{ minHeight: '100vh', display: 'flex', flexDirection: 'column', background: 'var(--ng-bg)' }}>
      <header className="jv-header">
        <div className="jv-container" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <a href="/" className="jv-logo">
            <span style={{ background: 'linear-gradient(90deg, #3b82f6, #8b5cf6)', WebkitBackgroundClip: 'text', color: 'transparent', fontWeight: 900, fontSize: 24, letterSpacing: '-1px' }}>
              NabuGate Docs
            </span>
          </a>
          <nav className="jv-nav hidden-mobile">
            <a href="/">{isFa ? 'صفحه اصلی' : 'Home'}</a>
            <a href="/panel/">{isFa ? 'ورود به پنل' : 'Console'}</a>
          </nav>
        </div>
      </header>
      
      <main style={{ flex: 1, display: 'flex', maxWidth: 1200, margin: '0 auto', width: '100%', padding: '40px 20px', gap: '40px' }}>
        {/* Sidebar */}
        <aside style={{ width: '250px', flexShrink: 0, borderRight: isFa ? 'none' : '1px solid var(--ng-border)', borderLeft: isFa ? '1px solid var(--ng-border)' : 'none', paddingRight: isFa ? 0 : 20, paddingLeft: isFa ? 20 : 0 }}>
          <h3 style={{ fontSize: 13, color: 'var(--ng-muted)', marginBottom: 16, textTransform: 'uppercase', letterSpacing: '1px', fontWeight: 700 }}>{isFa ? 'فهرست مستندات' : 'Documentation'}</h3>
          <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'flex', flexDirection: 'column', gap: 6 }}>
            {tabs.map(tab => (
              <li key={tab.id}>
                <button 
                  onClick={() => setActiveTab(tab.id)}
                  style={{
                    width: '100%', textAlign: isFa ? 'right' : 'left', padding: '10px 14px',
                    background: activeTab === tab.id ? 'var(--ng-surface)' : 'transparent',
                    color: activeTab === tab.id ? 'var(--ng-fg)' : 'var(--ng-muted)',
                    border: activeTab === tab.id ? '1px solid var(--ng-border)' : '1px solid transparent', 
                    borderRadius: 8, cursor: 'pointer', fontSize: 14,
                    fontWeight: activeTab === tab.id ? 700 : 500, transition: 'all 0.2s'
                  }}
                >
                  {tab.title}
                </button>
              </li>
            ))}
          </ul>
        </aside>

        {/* Content */}
        <div style={{ flex: 1, color: 'var(--ng-heading)', lineHeight: 1.8 }}>
          {activeTab === 'intro' && (
            <div className="docs-content fade-in">
              <h1 style={{ fontSize: 32, marginBottom: 24, fontWeight: 900, letterSpacing: '-1px' }}>{isFa ? 'شروع سریع' : 'Quickstart'}</h1>
              <p style={{ color: 'var(--ng-muted)', marginBottom: 24, fontSize: 16 }}>
                درگاه NabuGate یک پروکسی هوشمند و کاملاً سازگار با استاندارد OpenAI است. شما می‌توانید در هر نرم‌افزار، کتابخانه یا افزونه‌ای که از OpenAI پشتیبانی می‌کند، NabuGate را جایگزین کنید.
              </p>
              <div style={{ padding: 20, background: 'var(--ng-surface-soft)', border: '1px solid var(--ng-border)', borderRadius: 12, marginBottom: 32 }}>
                <h3 style={{ margin: '0 0 12px 0', fontSize: 16, color: 'var(--ng-fg)' }}>نکات کلیدی اتصال:</h3>
                <ul style={{ margin: 0, paddingInlineStart: 24, color: 'var(--ng-muted)' }}>
                  <li style={{ marginBottom: 8 }}><strong>Base URL:</strong> <code>https://gate.nabuxai.com/v1</code></li>
                  <li style={{ marginBottom: 8 }}><strong>API Key:</strong> کلیدی که از پنل NabuGate ساخته‌اید (شروع با ng_)</li>
                  <li><strong>Model Name:</strong> نام مدل مورد نظر (مثل gpt-4o یا claude-3-5-sonnet)</li>
                </ul>
              </div>
            </div>
          )}

          {activeTab === 'api-reference' && (
            <div className="docs-content fade-in">
              <h1 style={{ fontSize: 32, marginBottom: 24, fontWeight: 900, letterSpacing: '-1px' }}>مرجع API</h1>
              <p style={{ color: 'var(--ng-muted)', marginBottom: 24 }}>اندپوینت‌های زیر به صورت ۱۰۰٪ با استاندارد OpenAI سازگار هستند و ترافیک شما را مستقیماً به پروایدرهای مختلف (Anthropic, Google, Groq و...) ترجمه می‌کنند.</p>
              
              <div style={{ background: 'var(--ng-surface-soft)', border: '1px solid var(--ng-border)', borderRadius: 12, padding: 20, marginBottom: 16 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12 }}>
                  <span style={{ background: 'var(--ng-ok-soft)', color: 'var(--ng-ok-text)', padding: '4px 8px', borderRadius: 6, fontWeight: 700, fontSize: 12, fontFamily: 'monospace' }}>POST</span>
                  <code style={{ fontSize: 16, fontWeight: 600 }}>/v1/chat/completions</code>
                </div>
                <p style={{ margin: 0, color: 'var(--ng-muted)' }}>اصلی‌ترین اندپوینت برای چت و تعامل با مدل‌های متنی و بینایی (Vision). پشتیبانی کامل از Streaming و Tool Calling.</p>
              </div>

              <div style={{ background: 'var(--ng-surface-soft)', border: '1px solid var(--ng-border)', borderRadius: 12, padding: 20, marginBottom: 16 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12 }}>
                  <span style={{ background: 'var(--ng-ok-soft)', color: 'var(--ng-ok-text)', padding: '4px 8px', borderRadius: 6, fontWeight: 700, fontSize: 12, fontFamily: 'monospace' }}>POST</span>
                  <code style={{ fontSize: 16, fontWeight: 600 }}>/v1/embeddings</code>
                </div>
                <p style={{ margin: 0, color: 'var(--ng-muted)' }}>برای تولید Embedding و استفاده در سیستم‌های RAG.</p>
              </div>

              <div style={{ background: 'var(--ng-surface-soft)', border: '1px solid var(--ng-border)', borderRadius: 12, padding: 20, marginBottom: 16 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12 }}>
                  <span style={{ background: 'var(--ng-ok-soft)', color: 'var(--ng-ok-text)', padding: '4px 8px', borderRadius: 6, fontWeight: 700, fontSize: 12, fontFamily: 'monospace' }}>GET</span>
                  <code style={{ fontSize: 16, fontWeight: 600 }}>/v1/models</code>
                </div>
                <p style={{ margin: 0, color: 'var(--ng-muted)' }}>دریافت لیست تمامی مدل‌های فعال و در دسترس در دروازه شما.</p>
              </div>
            </div>
          )}

          {activeTab === 'env' && (
            <div className="docs-content fade-in">
              <h1 style={{ fontSize: 28, marginBottom: 24, fontWeight: 800 }}>متغیرهای محیطی سیستم (Env Vars)</h1>
              <p style={{ marginBottom: 16, color: 'var(--ng-muted)' }}>
                در صورتی که در لینوکس، مک یا فریم‌ورک‌های استاندارد کار می‌کنید، نیازی به تغییر کد نیست. فقط متغیرهای زیر را تنظیم کنید:
              </p>
              <pre dir="ltr" style={{ background: 'var(--ng-surface)', padding: 20, borderRadius: 12, border: '1px solid var(--ng-border)', fontSize: 14, fontFamily: 'monospace', overflowX: 'auto' }}>
export OPENAI_BASE_URL="https://gate.nabuxai.com/v1"
export OPENAI_API_KEY="ng_xxxxxxxxxxxxxxxxxxxx"
              </pre>
            </div>
          )}

          {activeTab === 'cursor' && (
            <div className="docs-content fade-in">
              <h1 style={{ fontSize: 28, marginBottom: 24, fontWeight: 800 }}>اتصال به Cursor</h1>
              <p style={{ marginBottom: 24, color: 'var(--ng-muted)' }}>کرسر (Cursor) به راحتی از NabuGate پشتیبانی می‌کند. مراحل زیر را طی کنید:</p>
              <div style={{ background: 'var(--ng-surface-soft)', padding: 24, borderRadius: 12, border: '1px solid var(--ng-border)' }}>
                <ol style={{ paddingInlineStart: 24, display: 'flex', flexDirection: 'column', gap: 16, margin: 0 }}>
                  <li>تنظیمات (Settings) برنامه Cursor را باز کنید.</li>
                  <li>به بخش <strong>Models</strong> بروید.</li>
                  <li>در قسمت <strong>OpenAI API Key</strong> کلید دریافتی از پنل ما را وارد کنید.</li>
                  <li>گزینه <strong>Override OpenAI Base URL</strong> را فعال کرده و آدرس <code>https://gate.nabuxai.com/v1</code> را بنویسید.</li>
                  <li>حالا می‌توانید به راحتی نام مدل‌هایی مثل <code>claude-3-5-sonnet</code> یا <code>gpt-4o</code> را در چت تایپ و استفاده کنید.</li>
                </ol>
              </div>
            </div>
          )}

          {activeTab === 'cline' && (
            <div className="docs-content fade-in">
              <h1 style={{ fontSize: 28, marginBottom: 24, fontWeight: 800 }}>اتصال به Cline (افزونه VS Code)</h1>
              <p style={{ marginBottom: 24, color: 'var(--ng-muted)' }}>برای تنظیم هوش مصنوعی Cline:</p>
              <div style={{ background: 'var(--ng-surface-soft)', padding: 24, borderRadius: 12, border: '1px solid var(--ng-border)' }}>
                <ol style={{ paddingInlineStart: 24, display: 'flex', flexDirection: 'column', gap: 16, margin: 0 }}>
                  <li>در تنظیمات افزونه Cline، بخش <strong>API Provider</strong> را روی <code>OpenAI Compatible</code> قرار دهید.</li>
                  <li>فیلد <strong>Base URL</strong> را روی <code>https://gate.nabuxai.com/v1</code> تنظیم کنید.</li>
                  <li>کلید دسترسی NabuGate را در فیلد <strong>API Key</strong> قرار دهید.</li>
                  <li>در فیلد Model ID نام مدل مورد نظر (مثلاً <code>claude-3-5-sonnet</code>) را وارد کنید.</li>
                </ol>
              </div>
            </div>
          )}

          {activeTab === 'claude-code' && (
            <div className="docs-content fade-in">
              <h1 style={{ fontSize: 28, marginBottom: 24, fontWeight: 800 }}>اتصال به Claude Code</h1>
              <p style={{ marginBottom: 16, color: 'var(--ng-muted)' }}>
                ابزار رسمی Anthropic برای ترمینال (Claude Code) می‌تواند ترافیک خود را از طریق NabuGate عبور دهد:
              </p>
              <pre dir="ltr" style={{ background: 'var(--ng-surface)', padding: 20, borderRadius: 12, border: '1px solid var(--ng-border)', fontSize: 14, fontFamily: 'monospace', overflowX: 'auto' }}>
# ابتدا متغیرهای محیطی را ست کنید
export OPENAI_BASE_URL="https://gate.nabuxai.com/v1"
export OPENAI_API_KEY="ng_xxxxxxxxxxxxxxxxxxxx"

# سپس کلود کد را با سوییچ openai اجرا کنید
claude -p openai -m claude-3-5-sonnet
              </pre>
            </div>
          )}
          
          {activeTab === 'sdk' && (
            <div className="docs-content fade-in">
              <h1 style={{ fontSize: 28, marginBottom: 24, fontWeight: 800 }}>استفاده در کتابخانه‌های رسمی</h1>
              
              <h3 style={{ fontSize: 18, margin: '32px 0 16px 0' }}>Node.js / Javascript</h3>
              <pre dir="ltr" style={{ background: 'var(--ng-surface)', padding: 20, borderRadius: 12, border: '1px solid var(--ng-border)', fontSize: 14, fontFamily: 'monospace', overflowX: 'auto' }}>
{`import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "https://gate.nabuxai.com/v1",
  apiKey: "ng_xxxxxxxx", // Your NabuGate API Key
});

const response = await client.chat.completions.create({
  model: "claude-3-5-sonnet", // Or any other model
  messages: [{ role: "user", content: "سلام!" }],
});

console.log(response.choices[0].message.content);`}
              </pre>

              <h3 style={{ fontSize: 18, margin: '32px 0 16px 0' }}>Python</h3>
              <pre dir="ltr" style={{ background: 'var(--ng-surface)', padding: 20, borderRadius: 12, border: '1px solid var(--ng-border)', fontSize: 14, fontFamily: 'monospace', overflowX: 'auto' }}>
{`from openai import OpenAI

client = OpenAI(
    base_url="https://gate.nabuxai.com/v1",
    api_key="ng_xxxxxxxx",
)

response = client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "سلام!"}]
)

print(response.choices[0].message.content)`}
              </pre>
            </div>
          )}

          {activeTab === 'curl' && (
            <div className="docs-content fade-in">
              <h1 style={{ fontSize: 28, marginBottom: 24, fontWeight: 800 }}>نمونه کدهای cURL</h1>
              <p style={{ marginBottom: 16, color: 'var(--ng-muted)' }}>تست سریع API مستقیماً از طریق ترمینال:</p>
              
              <pre dir="ltr" style={{ background: 'var(--ng-surface)', padding: 20, borderRadius: 12, border: '1px solid var(--ng-border)', fontSize: 14, fontFamily: 'monospace', overflowX: 'auto' }}>
{`curl https://gate.nabuxai.com/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer ng_xxxxxxxxxxxxxxx" \\
  -d '{
    "model": "claude-3-5-sonnet",
    "messages": [
      {
        "role": "user",
        "content": "سلام، تو کی هستی؟"
      }
    ]
  }'`}
              </pre>
            </div>
          )}
        </div>
      </main>
    </div>
  );
}
