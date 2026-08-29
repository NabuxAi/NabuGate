import Layout from '../components/Layout.jsx';
import { useState, useEffect } from 'react';
import * as api from '../api.js';

export default function Integration() {
  const origin = typeof window !== 'undefined' ? window.location.origin : 'https://gate.nabuxai.com';
  const [tokens, setTokens] = useState([]);
  const [aliases, setAliases] = useState([]);

  useEffect(() => {
    // listTokens resolves to { tokens: [...] }, not an array. Unwrapping it as
    // one left `tokens` an object, so tokens.length was undefined, the page
    // always claimed you had no keys, and every code sample below carried a
    // placeholder instead of anything real.
    api.listTokens().then((r) => setTokens(r.tokens || [])).catch(() => {});
    api.overview().then((d) => setAliases(d.aliases || [])).catch(() => {});
  }, []);

  // A key's secret is shown once, at creation, and never stored in readable
  // form — so this page cannot fill it in. Naming the key and its prefix lets
  // somebody match the sample to the key they saved; printing the project name
  // in the Authorization header, as this page used to, produces a sample that
  // is guaranteed to fail with 401.
  const first = tokens[0];
  const keyPlaceholder = first ? `${first.prefix}…` : 'ng-…';
  const model = aliases.length > 0 ? aliases[0].id : 'nabu-fast';

  const codeBlock = {
    direction: 'ltr',
    textAlign: 'left',
    background: 'var(--ng-code-bg, var(--ng-surface-soft))',
    color: 'var(--ng-code-text, var(--ng-text))',
    border: '1px solid var(--ng-border)',
    padding: 16,
    borderRadius: 8,
    overflowX: 'auto',
    fontSize: 13,
  };

  return (
    <Layout title="اتصال به دروازه" subtitle="هر ابزار سازگار با OpenAI را در چند گام وصل کنید.">
      <div className="card" style={{ marginBottom: 24, padding: 24 }}>
        <h3 style={{ marginTop: 0, marginBottom: 16 }}>درگاه سازگار با OpenAI</h3>
        <div style={{ ...codeBlock, marginBottom: 12 }}>
          <div style={{ marginBottom: 8 }}><strong>Base URL:</strong> <code>{origin}/v1</code></div>
          <div><strong>API key:</strong> <code>{keyPlaceholder}</code></div>
        </div>
        {tokens.length === 0 ? (
          <p className="muted" style={{ fontSize: 13, margin: 0 }}>
            هنوز کلیدی نساخته‌اید؛ از بخش «کلیدهای API» یکی بسازید.
          </p>
        ) : (
          <p className="muted" style={{ fontSize: 13, margin: 0, lineHeight: 1.7 }}>
            کلیدِ «{first.name}» با پیشوندِ <code dir="ltr">{first.prefix}</code>.
            متنِ کاملِ کلید فقط یک‌بار موقعِ ساخت نمایش داده می‌شود و جایی ذخیره
            نمی‌شود، پس اینجا قابلِ نمایش نیست — همانی را بگذارید که ذخیره کرده‌اید.
          </p>
        )}
      </div>

      <div className="card" style={{ marginBottom: 24, padding: 24 }}>
        <h3 style={{ marginTop: 0, marginBottom: 16 }}>شروع سریع</h3>

        <h4 style={{ marginBottom: 8 }}>OpenAI Python SDK</h4>
        <pre className="code-block" style={codeBlock}>
          <code>
{`from openai import OpenAI

client = OpenAI(base_url="${origin}/v1", api_key="${keyPlaceholder}")
client.chat.completions.create(
    model="${model}",
    messages=[{"role": "user", "content": "سلام"}],
)`}
          </code>
        </pre>

        <h4 style={{ marginTop: 24, marginBottom: 8 }}>cURL</h4>
        <pre className="code-block" style={codeBlock}>
          <code>
{`curl ${origin}/v1/chat/completions \\
  -H "Authorization: Bearer ${keyPlaceholder}" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"${model}","messages":[{"role":"user","content":"سلام"}]}'`}
          </code>
        </pre>
      </div>

      <div className="card" style={{ marginBottom: 24, padding: 24 }}>
        <h3 style={{ marginTop: 0, marginBottom: 8 }}>نام‌های در دسترس</h3>
        <p className="muted" style={{ fontSize: 13, marginBottom: 16 }}>
          {/* Read from the running router, not a list written by hand. A
              hard-coded catalogue describes whatever gateway existed the day
              somebody typed it — this one used to name ten models, none of
              which is an alias this deployment routes. */}
          همین‌ها را در فیلدِ <code dir="ltr">model</code> بگذارید. مستقیم از
          روترِ در حالِ اجرا خوانده می‌شود.
        </p>
        {aliases.length === 0 ? (
          <p className="muted" style={{ fontSize: 13, margin: 0 }}>چیزی برای نمایش نیست.</p>
        ) : (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(200px, 1fr))', gap: 12 }}>
            {aliases.map((a) => (
              <div key={a.id} style={{ padding: 12, border: '1px solid var(--ng-border)', borderRadius: 8 }}>
                <div className="mono" dir="ltr" style={{ fontWeight: 700, marginBottom: 4 }}>{a.id}</div>
                {a.owner && (
                  <span className="badge badge-muted" style={{ fontSize: 10 }}>{a.owner}</span>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="card" style={{ padding: 24 }}>
        <h3 style={{ marginTop: 0, marginBottom: 16 }}>راهنمای ابزارها</h3>
        <ul style={{ paddingRight: 20, fontSize: 14, lineHeight: '1.8' }}>
          <li><strong>Codex / OpenAI SDK:</strong> <code dir="ltr">OPENAI_BASE_URL</code> و <code dir="ltr">OPENAI_API_KEY</code>.</li>
          <li><strong>Cursor:</strong> بازنویسیِ آدرسِ پایهٔ OpenAI در تنظیمات.</li>
          <li><strong>VS Code:</strong> همان دو متغیر در <code dir="ltr">settings.json</code>.</li>
        </ul>
      </div>
    </Layout>
  );
}
