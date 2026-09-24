import Layout from '../components/Layout.jsx';
import { useState, useEffect } from 'react';
import * as api from '../api.js';
import CodeBlock from '../components/CodeBlock.jsx';
import { Skeleton } from '../components/Skeleton.jsx';
import { useT } from '../i18n/index.jsx';

const T = {
  fa: {
    title: 'اتصال به دروازه',
    subtitle: 'هر ابزار سازگار با OpenAI را در چند گام وصل کنید.',
    endpoint: 'درگاه سازگار با OpenAI',
    noKey: 'هنوز کلیدی نساخته‌اید؛ از بخش «کلیدهای API» یکی بسازید.',
    keyInfo: (v) => (
      <>
        کلیدِ «{v.name}» با پیشوندِ <code dir="ltr">{v.prefix}</code>.
        متنِ کاملِ کلید فقط یک‌بار موقعِ ساخت نمایش داده می‌شود و جایی ذخیره
        نمی‌شود، پس اینجا قابلِ نمایش نیست — همانی را بگذارید که ذخیره کرده‌اید.
      </>
    ),
    quickStart: 'شروع سریع',
    hello: 'سلام',
    names: 'نام‌های در دسترس',
    namesIntro: () => (
      <>
        همین‌ها را در فیلدِ <code dir="ltr">model</code> بگذارید. مستقیم از
        روترِ در حالِ اجرا خوانده می‌شود.
      </>
    ),
    tools: 'راهنمای ابزارها',
    toolSdk: () => <><code dir="ltr">OPENAI_BASE_URL</code> و <code dir="ltr">OPENAI_API_KEY</code>.</>,
    toolCursor: 'بازنویسیِ آدرسِ پایهٔ OpenAI در تنظیمات.',
    toolVscode: () => <>همان دو متغیر در <code dir="ltr">settings.json</code>.</>,
  },
  en: {
    title: 'Connect to the gateway',
    subtitle: 'Connect any OpenAI-compatible tool in a few steps.',
    endpoint: 'OpenAI-compatible endpoint',
    noKey: 'You haven’t created a key yet — create one under “API keys”.',
    keyInfo: (v) => (
      <>
        Key “{v.name}”, prefix <code dir="ltr">{v.prefix}</code>. The full key is shown only once, when it is
        created, and is not stored anywhere, so it can’t be shown here — use the one you saved.
      </>
    ),
    quickStart: 'Quick start',
    hello: 'Hello',
    names: 'Available names',
    namesIntro: () => (
      <>
        Put any of these in the <code dir="ltr">model</code> field. The list is read live from the running router.
      </>
    ),
    tools: 'Tool setup',
    toolSdk: () => <><code dir="ltr">OPENAI_BASE_URL</code> and <code dir="ltr">OPENAI_API_KEY</code>.</>,
    toolCursor: 'Override the OpenAI base URL in Settings.',
    toolVscode: () => <>The same two variables in <code dir="ltr">settings.json</code>.</>,
  },
};

export default function Integration() {
  const t = useT(T);
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
    textAlign: 'start',
    background: 'var(--ng-code-bg, var(--ng-surface-soft))',
    color: 'var(--ng-code-text, var(--ng-text))',
    border: '1px solid var(--ng-border)',
    padding: 16,
    borderRadius: 8,
    overflowX: 'auto',
    fontSize: 13,
  };

  return (
    <Layout title={t('title')} subtitle={t('subtitle')}>
      <div className="card" style={{ marginBottom: 24, padding: 24 }}>
        <h3 style={{ marginTop: 0, marginBottom: 16 }}>{t('endpoint')}</h3>
        <div style={{ ...codeBlock, marginBottom: 12 }}>
          <div style={{ marginBottom: 8 }}><strong>Base URL:</strong> <code>{origin}/v1</code></div>
          <div><strong>API key:</strong> <code>{keyPlaceholder}</code></div>
        </div>
        {tokens.length === 0 ? (
          <p className="muted" style={{ fontSize: 13, margin: 0 }}>{t('noKey')}</p>
        ) : (
          <p className="muted" style={{ fontSize: 13, margin: 0, lineHeight: 1.7 }}>
            {t('keyInfo', { name: first.name, prefix: first.prefix })}
          </p>
        )}
      </div>

      <div className="card" style={{ marginBottom: 24, padding: 24 }}>
        <h3 style={{ marginTop: 0, marginBottom: 16 }}>{t('quickStart')}</h3>

        <h4 style={{ marginBottom: 8 }}>OpenAI Python SDK</h4>
        <CodeBlock code={`from openai import OpenAI

client = OpenAI(base_url="${origin}/v1", api_key="${keyPlaceholder}")
client.chat.completions.create(
    model="${model}",
    messages=[{"role": "user", "content": "${t('hello')}"}],
)`} />

        <h4 style={{ marginTop: 24, marginBottom: 8 }}>cURL</h4>
        <CodeBlock code={`curl ${origin}/v1/chat/completions \\
  -H "Authorization: Bearer ${keyPlaceholder}" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"${model}","messages":[{"role":"user","content":"${t('hello')}"}]}'`} />
      </div>

      <div className="card" style={{ marginBottom: 24, padding: 24 }}>
        <h3 style={{ marginTop: 0, marginBottom: 8 }}>{t('names')}</h3>
        <p className="muted" style={{ fontSize: 13, marginBottom: 16 }}>
          {/* Read from the running router, not a list written by hand. A
              hard-coded catalogue describes whatever gateway existed the day
              somebody typed it — this one used to name ten models, none of
              which is an alias this deployment routes. */}
          {t('namesIntro')}
        </p>
        {aliases.length === 0 ? (
          <div className="chips"><Skeleton w={110} h={40} /><Skeleton w={130} h={40} /><Skeleton w={100} h={40} /></div>
        ) : (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(min(200px, 100%), 1fr))', gap: 12 }}>
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
        <h3 style={{ marginTop: 0, marginBottom: 16 }}>{t('tools')}</h3>
        <ul style={{ paddingInlineStart: 20, fontSize: 14, lineHeight: '1.8' }}>
          <li><strong>Codex / OpenAI SDK:</strong> {t('toolSdk')}</li>
          <li><strong>Cursor:</strong> {t('toolCursor')}</li>
          <li><strong>VS Code:</strong> {t('toolVscode')}</li>
        </ul>
      </div>
    </Layout>
  );
}
