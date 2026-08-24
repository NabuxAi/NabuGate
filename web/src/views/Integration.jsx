import Layout from '../components/Layout.jsx';
import { useState, useEffect } from 'react';
import * as api from '../api.js';

export default function Integration() {
  const origin = typeof window !== 'undefined' ? window.location.origin : 'https://gate.nabuxai.com';
  const [tokens, setTokens] = useState([]);
  
  useEffect(() => {
    api.listTokens().then(res => setTokens(res || [])).catch(() => {});
  }, []);

  const defaultToken = tokens.length > 0 ? tokens[0].name : "pmai_xxxxxxxx";

  return (
    <Layout title="راهنمای توسعه‌دهندگان" subtitle="ابزارهای سازگار با OpenAI را در چند گام به درگاه متصل کنید.">
      
      <div className="card" style={{ marginBottom: 24, padding: 24 }}>
        <h3 style={{ marginTop: 0, marginBottom: 16 }}>درگاه سازگار با OpenAI</h3>
        <p className="muted" style={{ fontSize: 13, marginBottom: 12 }}>
          درگاه با API چت OpenAI سازگار است؛ کافی است آدرس پایه و کلید را در ابزار خود تنظیم کنید.
        </p>
        <div style={{ background: '#0b1020', padding: '16px', borderRadius: 8, border: '1px solid #1c2742', fontSize: 13 }}>
          <div style={{ marginBottom: 8 }}><strong>Base URL:</strong> <code style={{ color: '#66b2ff' }}>{origin}/v1</code></div>
          <div style={{ marginBottom: 8 }}><strong>کلید API:</strong> <code style={{ color: '#66b2ff' }}>{defaultToken}</code></div>
          {tokens.length === 0 && <div className="muted">هنوز کلیدی نساخته‌اید. ابتدا از بخش «کلیدهای من» کلید بسازید.</div>}
        </div>
      </div>

      <div className="card" style={{ marginBottom: 24, padding: 24 }}>
        <h3 style={{ marginTop: 0, marginBottom: 16 }}>شروع سریع</h3>
        
        <h4 style={{ marginBottom: 8 }}>OpenAI Python SDK</h4>
        <pre className="code-block" style={{ direction: 'ltr', textAlign: 'left', background: '#0b1020', padding: 16, borderRadius: 8, overflowX: 'auto', border: '1px solid #1c2742', fontSize: 13, color: '#e6ecff' }}>
          <code>
{`from openai import OpenAI
client = OpenAI(base_url="${origin}/v1", api_key="${defaultToken}")
client.chat.completions.create(model="gpt-5.5", messages=[{"role":"user","content":"سلام"}])`}
          </code>
        </pre>

        <h4 style={{ marginTop: 24, marginBottom: 8 }}>cURL</h4>
        <pre className="code-block" style={{ direction: 'ltr', textAlign: 'left', background: '#0b1020', padding: 16, borderRadius: 8, overflowX: 'auto', border: '1px solid #1c2742', fontSize: 13, color: '#e6ecff' }}>
          <code>
{`curl ${origin}/v1/chat/completions \\
  -H "Authorization: Bearer ${defaultToken}" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"gpt-5.5","messages":[{"role":"user","content":"سلام"}],"max_tokens":256}'`}
          </code>
        </pre>
      </div>

      <div className="card" style={{ marginBottom: 24, padding: 24 }}>
        <h3 style={{ marginTop: 0, marginBottom: 16 }}>راهنمای ابزارها</h3>
        <ul style={{ paddingRight: 20, fontSize: 14, lineHeight: '1.8' }}>
          <li><strong>Claude Code:</strong> تنظیم آدرس پایه و <code>ANTHROPIC_API_KEY</code>.</li>
          <li><strong>Codex:</strong> تنظیم <code>OPENAI_BASE_URL</code> و <code>OPENAI_API_KEY</code> در config.toml.</li>
          <li><strong>Cursor:</strong> بازنویسی آدرس پایه OpenAI در تنظیمات Cursor.</li>
          <li><strong>VS Code:</strong> نصب افزونه و تنظیم دائمی در settings.json.</li>
        </ul>
      </div>
      
      <div className="card" style={{ marginBottom: 24, padding: 24 }}>
        <h3 style={{ marginTop: 0, marginBottom: 16 }}>مدل‌های پشتیبانی‌شده</h3>
        <p className="muted" style={{ fontSize: 13, marginBottom: 12 }}>
          کد مدل (مثل gpt-5.5) را در فیلد model درخواست‌ها قرار دهید.
        </p>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(200px, 1fr))', gap: 16 }}>
          {[
            { name: "Claude 3.5 Haiku", id: "claude-3-5-haiku-latest", features: ["گفت‌وگو", "استریم", "فراخوانی ابزار"] },
            { name: "Claude Haiku 4.5", id: "claude-haiku-4.5", features: ["گفت‌وگو", "استریم", "ابزار", "بینایی", "امبدینگ"] },
            { name: "Claude Opus 4.8", id: "claude-opus-4.8", features: ["گفت‌وگو", "استریم", "ابزار", "بینایی"] },
            { name: "Claude Opus 5", id: "claude-opus-5", features: ["گفت‌وگو", "استریم", "ابزار", "بینایی", "امبدینگ"] },
            { name: "Claude Sonnet 4", id: "claude-sonnet-4", features: ["گفت‌وگو", "استریم", "ابزار", "امبدینگ"] },
            { name: "Claude Sonnet 5", id: "claude-sonnet-5", features: ["گفت‌وگو", "استریم", "ابزار", "بینایی"] },
            { name: "GPT-5.5", id: "gpt-5.5", features: ["گفت‌وگو", "استریم", "ابزار", "بینایی"] },
            { name: "GPT-5.6 Luna", id: "gpt-5.6-luna", features: ["گفت‌وگو", "استریم", "ابزار", "بینایی"] },
            { name: "GPT-5.6 Sol", id: "gpt-5.6-sol", features: ["گفت‌وگو", "استریم", "ابزار", "بینایی"] },
            { name: "GPT-5.6 Terra", id: "gpt-5.6-terra", features: ["گفت‌وگو", "استریم", "ابزار", "بینایی"] },
          ].map(m => (
            <div key={m.id} style={{ padding: 12, border: '1px solid var(--ng-border)', borderRadius: 8 }}>
              <div style={{ fontWeight: 'bold', marginBottom: 4 }}>{m.name}</div>
              <div style={{ fontSize: 11, color: 'var(--ng-muted)', marginBottom: 8, direction: 'ltr', textAlign: 'right' }}>{m.id}</div>
              <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>
                {m.features.map(f => (
                  <span key={f} style={{ fontSize: 10, background: '#1c2742', padding: '2px 6px', borderRadius: 4 }}>{f}</span>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>

    </Layout>
  );
}
