import Layout from '../components/Layout.jsx';

export default function Integration() {
  const origin = typeof window !== 'undefined' ? window.location.origin : 'https://gate.nabuxai.com';
  
  return (
    <Layout title="اتصال و کدها" subtitle="راهنمای اتصال به NabuGate در ابزارها و زبان‌های مختلف">
      <div className="card" style={{ marginBottom: 24, padding: 24 }}>
        <h3 style={{ marginTop: 0, marginBottom: 16 }}>Claude Code</h3>
        <p className="muted" style={{ fontSize: 13, marginBottom: 12 }}>
          برای استفاده از NabuGate درون محیط خط فرمان کلود (Claude Code) دستورات زیر را وارد کنید.
          از آنجا که NabuGate رابط OpenAI را شبیه‌سازی می‌کند، می‌توانید به شکل زیر کلود را متصل کنید:
        </p>
        <pre className="code-block" style={{ direction: 'ltr', textAlign: 'left', background: '#0b1020', padding: 16, borderRadius: 8, overflowX: 'auto', border: '1px solid #1c2742', fontSize: 13, color: '#e6ecff' }}>
          <code>
{`claude config set api_base ${origin}/v1
claude config set api_key YOUR_NABUGATE_TOKEN
claude config set model nabu-fast`}
          </code>
        </pre>
      </div>

      <div className="card" style={{ marginBottom: 24, padding: 24 }}>
        <h3 style={{ marginTop: 0, marginBottom: 16 }}>Cursor & Codex IDE</h3>
        <p className="muted" style={{ fontSize: 13, marginBottom: 12 }}>
          در تنظیمات ویرایشگر Cursor یا ابزارهای مشابه (مثل Continue.dev)، به بخش Models بروید و این مقادیر را تنظیم کنید:
        </p>
        <div style={{ background: '#0b1020', padding: '16px', borderRadius: 8, border: '1px solid #1c2742', fontSize: 13 }}>
          <div style={{ marginBottom: 8 }}><strong>OpenAI API Base:</strong> <code style={{ color: '#66b2ff' }}>{origin}/v1</code></div>
          <div style={{ marginBottom: 8 }}><strong>API Key:</strong> <code style={{ color: '#66b2ff' }}>YOUR_NABUGATE_TOKEN</code></div>
          <div><strong>Models:</strong> <code style={{ color: '#66b2ff' }}>nabu-fast, nabu-reasoning, ...</code></div>
        </div>
      </div>

      <div className="card" style={{ marginBottom: 24, padding: 24 }}>
        <h3 style={{ marginTop: 0, marginBottom: 16 }}>Python (OpenAI SDK)</h3>
        <p className="muted" style={{ fontSize: 13, marginBottom: 12 }}>
          کتابخانه رسمی OpenAI در پایتون با NabuGate کاملاً سازگار است. فقط کافیست <code>base_url</code> را تغییر دهید:
        </p>
        <pre className="code-block" style={{ direction: 'ltr', textAlign: 'left', background: '#0b1020', padding: 16, borderRadius: 8, overflowX: 'auto', border: '1px solid #1c2742', fontSize: 13, color: '#e6ecff' }}>
          <code>
{`from openai import OpenAI

client = OpenAI(
    base_url="${origin}/v1",
    api_key="YOUR_NABUGATE_TOKEN"
)

response = client.chat.completions.create(
    model="nabu-fast",
    messages=[{"role": "user", "content": "سلام!"}]
)
print(response.choices[0].message.content)`}
          </code>
        </pre>
      </div>

      <div className="card" style={{ padding: 24 }}>
        <h3 style={{ marginTop: 0, marginBottom: 16 }}>cURL</h3>
        <pre className="code-block" style={{ direction: 'ltr', textAlign: 'left', background: '#0b1020', padding: 16, borderRadius: 8, overflowX: 'auto', border: '1px solid #1c2742', fontSize: 13, color: '#e6ecff' }}>
          <code>
{`curl ${origin}/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer YOUR_NABUGATE_TOKEN" \\
  -d '{
    "model": "nabu-fast",
    "messages": [{"role": "user", "content": "سلام"}]
  }'`}
          </code>
        </pre>
      </div>
    </Layout>
  );
}
