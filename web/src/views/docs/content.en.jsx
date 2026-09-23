import CodeBlock from '../../components/CodeBlock.jsx';
import { Callout, EnvTabs, Faq, H1, H2, Lead, Steps } from './parts.jsx';

/*
 * The English documentation. It mirrors content.fa.jsx section for section
 * and element for element; a section added there needs its counterpart here,
 * or English readers get the Persian one (see Docs.jsx).
 */
export default function DocsEn({ active, go, BASE, ORIGIN, KEY }) {
  return (
    <>
      {active === 'intro' && (
        <section>
          <H1>Quickstart</H1>
          <Lead>NabuGate is an OpenAI-compatible gateway: one URL, one key, and access to models from OpenAI, Anthropic, Google, and more, with automatic fallback. Any tool or library that works with OpenAI works with NabuGate once you change the base URL.</Lead>
          <Steps items={[
            <>Sign up in the <a href="/panel/">console</a>. Accounts are created with an email address; you can also sign in with Google or Nabu.</>,
            <>Top up your account from “Plans & top-up”. You pay for what you use, and your balance never expires. <button className="linklike" onClick={() => go('billing')}>Details</button></>,
            <>Create a key in “API keys”. It starts with <code>ng_</code> and is shown in full only once.</>,
            <>Set the base URL to <code dir="ltr">{BASE}</code> and the model to one of the names from <code dir="ltr">GET {BASE}/models</code> (for example <code>nabu-fast</code>).</>,
          ]} />
          <H2>The three values you need everywhere</H2>
          <div className="card card-flat" style={{ padding: 18 }}>
            <div className="kv">
              <div className="line"><span>Base URL</span><strong className="mono" dir="ltr">{BASE}</strong></div>
              <div className="line"><span>API Key</span><strong className="mono" dir="ltr">ng_…</strong></div>
              <div className="line"><span>Model</span><strong className="mono" dir="ltr">nabu-fast · nabu-smart · openai/gpt-… · anthropic/claude-…</strong></div>
            </div>
          </div>
          <H2>Environment variables</H2>
          <EnvTabs BASE={BASE} KEY={KEY} />
          <Callout icon="💡"><strong>Claude Code uses a different base URL:</strong> no <code>/v1</code>, and <code>ANTHROPIC_*</code> variables instead. <button className="linklike" onClick={() => go('claude-code')}>Claude Code guide</button></Callout>
        </section>
      )}

      {active === 'billing' && (
        <section>
          <H1>Billing & top-ups</H1>
          <Lead>NabuGate has no monthly subscription. You have a USD wallet, and each request deducts exactly the model’s real price from it. Top up whenever you like; your balance never expires.</Lead>
          <H2>How payment works</H2>
          <Steps items={[
            'In the console, go to “Plans & top-up” and choose a package or a custom amount ($1 to $5,000).',
            'You’re redirected to the bank gateway, which shows the amount in Toman at the day’s rate. You enter your card details there; this console never sees or stores any card information.',
            'After paying, you land back on “Balance & usage”. The console asks the gateway itself whether the payment arrived, and credits your balance only once it’s confirmed.',
            'Your keys work with the new balance immediately. There’s no need to create a new key.',
          ]} />
          <H2>Why USD?</H2>
          <p>The gateway prices every model in US dollars (the same unit providers publish their prices in), and your balance is kept in that unit too, so a number never means two different things in two places. The bank gateway shows the Toman equivalent at the moment you pay, and it appears on your bank receipt.</p>
          <H2>How much does a request cost?</H2>
          <p>Each model is priced per million input and output tokens; you can see the rates in “Models”. Every response carries two headers, so you’re never caught off guard:</p>
          <CodeBlock code={`X-Nabu-Balance-USD: 4.1837       # remaining balance after this request
X-Nabu-Balance-Warning: low       # only when the balance is below $1`} label="response headers" />
          <p style={{ marginTop: 12 }}>Usage is tracked separately for each key; create one key per app so you know where the money went.</p>
          <H2>When your balance runs out</H2>
          <p>Requests are rejected with <code>402 Payment Required</code> and logged in “Recent requests” with the reason “insufficient balance”. Top up any amount and the key works again immediately.</p>
          <Callout kind="ok" icon="✓">Your balance can’t be transferred between accounts or withdrawn as cash, but it never expires either.</Callout>
        </section>
      )}

      {active === 'payment-issues' && (
        <section>
          <H1>Payment issues</H1>
          <Lead>The general rule: the console doesn’t trust anything written in the URL the bank sends you back to. Every time you open “Balance & usage” or “Payments”, the status of your pending invoices is fetched directly from the gateway itself, and as soon as one is confirmed, your balance is credited exactly once. So the first step for any problem is to reload the page or click “Check status”.</Lead>
          <Faq q="Money left my account but my balance didn’t go up">
            Go to “Payments” and click “Check status”. If the bank has confirmed the transaction, your balance is credited on the spot. Some gateways (crypto payments, for example) take a few minutes to confirm. If it’s still “Pending” after 30 minutes, send the invoice ID to support. A payment that is never confirmed is returned to your card by the bank within 72 hours.
          </Faq>
          <Faq q="I saw an error page after paying, or my browser closed">
            That’s fine. The invoice is registered to your account before you’re sent to the bank, and confirming it doesn’t depend on your browser. Sign in to the console and open “Payments”.
          </Faq>
          <Faq q="Error: “The payment gateway did not return a checkout URL”">
            The payment bridge couldn’t get an invoice from the bank; this is usually a temporary gateway outage. Try again in a few minutes. If it keeps happening, an admin should check the gateway logs for <code>payment bridge refused</code> (see the gateway setup section).
          </Faq>
          <Faq q="The message “no payment gateway is configured on this deployment”">
            This deployment has no payment gateway; the payment buttons won’t work until an admin sets <code>NABUPAY_URL</code> and <code>NABUPAY_SECRET</code>. Contact support for a manual top-up.
          </Faq>
          <Faq q="I paid twice">
            Each invoice is credited to your balance only once, and refreshing the page never counts anything twice. If you really do have two successful transactions at the bank, both appear in “Payments” with separate IDs, and both have been credited to your balance. To get one of them refunded, give the invoice ID to support.
          </Faq>
          <Faq q="The amount on the bank page is different from what I chose">
            You choose an amount in dollars and the bank charges in Toman; the conversion uses the gateway’s live rate. What’s added to your balance is exactly the dollar amount you chose, regardless of that day’s rate.
          </Faq>
          <Faq q="My key still returns 402 after topping up">
            Check your balance in “Balance & usage”. If it’s still zero, the payment hasn’t been confirmed; click “Check again”. If your balance is positive but you still get 402, the key belongs to a different account; in “API keys”, make sure you’re using a key that was created in this account.
          </Faq>
          <H2>What support needs to follow up</H2>
          <p>The invoice ID from the “Payments” table, the date and amount, and your account email. Your card number and the full bank receipt aren’t needed.</p>
        </section>
      )}

      {active === 'models' && (
        <section>
          <H1>Models & aliases</H1>
          <Lead>Instead of naming a specific model, you usually call an <strong>alias</strong>: a name like <code>nabu-fast</code> that the gateway backs with a chain of models from several providers. If the first one is down or returns an empty response, the next one is tried, and you only ever see the answer.</Lead>
          <H2>Live list</H2>
          <CodeBlock code={`curl ${BASE}/models -H "Authorization: Bearer ${KEY}"`} />
          <p style={{ marginTop: 12 }}>The output includes every alias, sub-agent, flow, and direct model. Put any of these names in the <code>model</code> field.</p>
          <H2>Three kinds of names</H2>
          <ul style={{ paddingInlineStart: 20, display: 'flex', flexDirection: 'column', gap: 8 }}>
            <li><strong>Aliases</strong> such as <code>nabu-fast</code>, <code>nabu-smart</code>, <code>nabu-embed</code>: a chain across several providers, with fallback. Choose these for everyday work.</li>
            <li><strong>Direct models</strong> such as <code dir="ltr">openai/gpt-…</code> or <code dir="ltr">anthropic/claude-…</code>: no fallback, exactly that model. Use these when you need reproducible results.</li>
            <li><strong>Agents / flows</strong> such as <code>cine-motion-designer</code>: an assistant with a ready-made system prompt, called just like a model.</li>
          </ul>
          <Callout kind="warn" icon="⚠️">Don’t use an alias with fallback to <strong>build a vector index</strong>: vector width differs between providers. Use an alias without fallback, such as <code>write-embed</code>.</Callout>
        </section>
      )}

      {active === 'env' && (
        <section>
          <H1>Environment variables</H1>
          <Lead>Most tools and libraries read these two variables, so you don’t have to touch your code.</Lead>
          <EnvTabs BASE={BASE} KEY={KEY} />
          <H2>Making them permanent</H2>
          <p>On macOS/Linux, add the two lines to <code dir="ltr">~/.zshrc</code> or <code dir="ltr">~/.bashrc</code>. On Windows, use “Edit environment variables for your account”, or <code dir="ltr">[Environment]::SetEnvironmentVariable(...)</code> in PowerShell.</p>
          <Callout icon="💡">The official OpenAI SDKs (Python, Node, Go, and others) pick up these two variables without any arguments.</Callout>
        </section>
      )}

      {active === 'cursor' && (
        <section>
          <H1>Connect Cursor</H1>
          <Lead>Cursor lets you override the OpenAI base URL. Once you do, every model NabuGate offers can be selected in Cursor.</Lead>
          <Steps items={[
            <>Open Settings → Models.</>,
            <>Under OpenAI API Key, enter your <code>ng_…</code> key.</>,
            <>Turn on <strong>Override OpenAI Base URL</strong> and enter <code dir="ltr">{BASE}</code>.</>,
            <>Add an alias name with “+ Add model” (for example <code>nabu-smart</code>) and click Verify.</>,
          ]} />
          <Callout kind="warn" icon="⚠️">Cursor sends a small request to verify the setup; if your balance is zero, it fails with 402. Top up first.</Callout>
        </section>
      )}

      {active === 'cline' && (
        <section>
          <H1>Cline, Roo Code, and Continue</H1>
          <Lead>All three accept an “OpenAI Compatible” provider and take three values.</Lead>
          <CodeBlock code={`API Provider : OpenAI Compatible
Base URL     : ${BASE}
API Key      : ${KEY}
Model ID     : nabu-smart`} label="settings" />
          <H2>Continue</H2>
          <CodeBlock code={`{
  "models": [{
    "title": "NabuGate",
    "provider": "openai",
    "apiBase": "${BASE}",
    "apiKey": "${KEY}",
    "model": "nabu-smart"
  }]
}`} label="~/.continue/config.json" />
          <Callout icon="💡">Use <code>nabu-smart</code> for coding and <code>nabu-fast</code> for quick, cheap tasks. Tool calling is fully supported.</Callout>
        </section>
      )}

      {active === 'claude-code' && (
        <section>
          <H1>Claude Code</H1>
          <Lead>Claude Code speaks Anthropic’s native protocol and appends <code>/v1/messages</code> to the base URL itself. So the base URL goes <strong>without</strong> <code>/v1</code>, and both authentication variables must be set.</Lead>
          <CodeBlock code={`export ANTHROPIC_BASE_URL="${ORIGIN}"
export ANTHROPIC_API_KEY="${KEY}"
export ANTHROPIC_AUTH_TOKEN="${KEY}"
export ANTHROPIC_MODEL="nabu-smart"
export ANTHROPIC_SMALL_FAST_MODEL="nabu-fast"

claude`} label="bash · zsh" />
          <H2>Persistent setup for the CLI and the VS Code extension</H2>
          <CodeBlock code={`{
  "env": {
    "ANTHROPIC_BASE_URL": "${ORIGIN}",
    "ANTHROPIC_API_KEY": "${KEY}",
    "ANTHROPIC_AUTH_TOKEN": "${KEY}",
    "ANTHROPIC_MODEL": "nabu-smart",
    "ANTHROPIC_SMALL_FAST_MODEL": "nabu-fast"
  }
}`} label="~/.claude/settings.json" />
          <Callout kind="warn" icon="⚠️">
            <strong>401</strong> means one of the two variables, <code>ANTHROPIC_API_KEY</code> / <code>ANTHROPIC_AUTH_TOKEN</code>, is missing. <strong>A path error</strong> means the base URL includes <code>/v1</code>. <strong>Model not found</strong> means <code>ANTHROPIC_MODEL</code> isn’t pinned, so Claude Code sent its own default model name.
          </Callout>
        </section>
      )}

      {active === 'codex' && (
        <section>
          <H1>Codex CLI</H1>
          <Lead>Codex uses the Responses API, which the gateway supports; you need to define a custom provider with <code dir="ltr">wire_api = "responses"</code>.</Lead>
          <CodeBlock code={`model = "nabu-smart"
model_provider = "nabugate"

[model_providers.nabugate]
name = "NabuGate"
base_url = "${BASE}"
env_key = "OPENAI_API_KEY"
wire_api = "responses"`} label="~/.codex/config.toml" />
          <p style={{ marginTop: 12 }}>Then put the key in an environment variable:</p>
          <CodeBlock code={`export OPENAI_API_KEY="${KEY}"`} />
        </section>
      )}

      {active === 'vscode' && (
        <section>
          <H1>VS Code</H1>
          <Lead>Three options: the Claude Code extension, the Codex extension, or an OpenAI-compatible extension (Cline, Roo Code, Continue).</Lead>
          <H2>Claude Code extension</H2>
          <p>It reads the same <code dir="ltr">~/.claude/settings.json</code> as in the <button className="linklike" onClick={() => go('claude-code')}>Claude Code</button> section; nothing else is needed.</p>
          <H2>Codex extension</H2>
          <p>It uses the same <code dir="ltr">~/.codex/config.toml</code> as in the <button className="linklike" onClick={() => go('codex')}>Codex</button> section.</p>
          <H2>Cline / Roo / Continue</H2>
          <p>See the <button className="linklike" onClick={() => go('cline')}>OpenAI Compatible</button> guide.</p>
          <H2>VS Code integrated terminal</H2>
          <CodeBlock code={`{
  "terminal.integrated.env.osx": {
    "OPENAI_BASE_URL": "${BASE}",
    "OPENAI_API_KEY": "${KEY}"
  }
}`} label="settings.json" />
        </section>
      )}

      {active === 'sdk' && (
        <section>
          <H1>Python & Node SDKs</H1>
          <Lead>The official OpenAI libraries work unchanged; just pass the base URL and your key.</Lead>
          <H2>Python</H2>
          <CodeBlock code={`from openai import OpenAI

client = OpenAI(base_url="${BASE}", api_key="${KEY}")

r = client.chat.completions.create(
    model="nabu-fast",
    messages=[{"role": "user", "content": "Hello!"}],
)
print(r.choices[0].message.content)`} label="python" />
          <H2>Node.js / TypeScript</H2>
          <CodeBlock code={`import OpenAI from "openai";

const client = new OpenAI({ baseURL: "${BASE}", apiKey: "${KEY}" });

const r = await client.chat.completions.create({
  model: "nabu-fast",
  messages: [{ role: "user", content: "Hello!" }],
});
console.log(r.choices[0].message.content);`} label="node" />
          <H2>Streaming</H2>
          <CodeBlock code={`stream = client.chat.completions.create(model="nabu-fast", messages=msgs, stream=True)
for chunk in stream:
    print(chunk.choices[0].delta.content or "", end="", flush=True)`} label="python" />
          <H2>Fixed-width embeddings</H2>
          <CodeBlock code={`client.embeddings.create(model="write-embed", input=["text"], dimensions=1536)`} label="python" />
          <Callout icon="💡">Dedicated SDKs are available too: <code>@nabugate/sdk</code> on npm, <code>nabugate</code> on PyPI, plus Go, Rust, Dart, and Laravel. They all take the same URL and key.</Callout>
        </section>
      )}

      {active === 'curl' && (
        <section>
          <H1>cURL</H1>
          <Lead>Quick tests from the terminal. Copy any example and substitute your own key.</Lead>
          <H2>Chat</H2>
          <CodeBlock code={`curl ${BASE}/chat/completions \\
  -H "Authorization: Bearer ${KEY}" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"nabu-fast","messages":[{"role":"user","content":"Hi, who are you?"}]}'`} />
          <H2>List models</H2>
          <CodeBlock code={`curl ${BASE}/models -H "Authorization: Bearer ${KEY}"`} />
          <H2>Usage for this key</H2>
          <CodeBlock code={`curl ${BASE}/usage -H "Authorization: Bearer ${KEY}"`} />
          <H2>Read your balance from the response headers</H2>
          <CodeBlock code={`curl -si ${BASE}/chat/completions -H "Authorization: Bearer ${KEY}" \\
  -H "Content-Type: application/json" -d '{"model":"nabu-fast","messages":[{"role":"user","content":"hi"}]}' \\
  | grep -i x-nabu-balance`} />
        </section>
      )}

      {active === 'api-reference' && (
        <section>
          <H1>API reference</H1>
          <Lead>Every endpoint follows the OpenAI standard. The request body reaches the provider untouched; only <code>model</code> and the stream flags are rewritten. So <code>tools</code>, <code>response_format</code>, <code>seed</code>, <code>top_p</code>, and the rest all work.</Lead>
          <div className="stagger">
            {[
              ['POST', '/v1/chat/completions', 'Chat and vision, with full streaming and tool calling.'],
              ['POST', '/v1/responses', 'The Responses API, for Codex and clients that expect this shape.'],
              ['POST', '/v1/embeddings', 'Vectors for RAG. The dimensions parameter is passed through to the provider.'],
              ['POST', '/v1/images/generations', 'Image generation; with the photo alias, a stock image search.'],
              ['POST', '/v1/audio/speech', 'Text to speech.'],
              ['POST', '/v1/audio/transcriptions', 'Speech to text.'],
              ['GET', '/v1/models', 'Live list of aliases, agents, flows, and direct models.'],
              ['GET', '/v1/agents', 'Sub-agents with their models and tools.'],
              ['GET', '/v1/usage', 'Usage for this key: tokens and cost, broken down by model.'],
              ['GET', '/v1/photos/search', 'Stock photo search, without generating an image.'],
              ['GET', '/v1/health', 'Gateway health.'],
            ].map(([m, p, d]) => (
              <div key={p} className="card card-flat" style={{ padding: '14px 18px', marginBottom: 10, display: 'flex', gap: 14, alignItems: 'flex-start', flexWrap: 'wrap' }}>
                <span className={'badge ' + (m === 'GET' ? 'badge-info' : 'badge-ok')} style={{ fontFamily: 'var(--ng-mono)' }}>{m}</span>
                <code style={{ fontSize: 14, fontWeight: 700, minWidth: 220 }} dir="ltr">{p}</code>
                <span style={{ color: 'var(--ng-muted)', fontSize: 13, flex: 1 }}>{d}</span>
              </div>
            ))}
          </div>
          <H2>Response headers</H2>
          <ul style={{ paddingInlineStart: 20 }}>
            <li><code>X-Nabu-Balance-USD</code>: your balance after this request.</li>
            <li><code>X-Nabu-Balance-Warning: low</code>: your balance is below $1.</li>
            <li><code>X-Nabu-Agent</code>: the sub-agent’s name, when the model was a sub-agent.</li>
          </ul>
        </section>
      )}

      {active === 'errors' && (
        <section>
          <H1>Errors & troubleshooting</H1>
          <Lead>Every error is logged in “Recent requests” along with its reason; start there.</Lead>
          {[
            ['401 Unauthorized', 'The key is wrong, deleted, or disabled, or the request has no Authorization: Bearer header. Claude Code needs both ANTHROPIC_API_KEY and ANTHROPIC_AUTH_TOKEN.'],
            ['402 Payment Required', 'The balance of the account that owns the key is zero. Top up from “Plans & top-up”; you don’t need a new key.'],
            ['403 Forbidden', 'The key isn’t allowed from this origin, or the model isn’t on the key’s allow-list. Check the origin and access settings in “API keys”.'],
            ['404 model not found', 'The model name isn’t one of the names returned by GET /v1/models. Check that it’s lowercase and includes the provider prefix (openai/…).'],
            ['429 Too Many Requests', 'You’ve hit the key’s rate limit. For high-volume apps, create a separate key with a higher limit.'],
            ['502 all targets failed', 'Every provider in the alias chain failed. This is usually temporary; retry in a few seconds or try a different direct model.'],
            ['Empty response', 'If a stream closes without any content, the gateway moves on to the next target on its own. If the response is still empty, try a direct model with the same message and let support know.'],
            ['Vector width changed', 'An embedding alias with fallback passes through models of different widths. For a stored index, use an alias without fallback (write-embed) and a fixed dimensions value.'],
          ].map(([t, d]) => (
            <details key={t} className="faq"><summary dir="auto"><code dir="ltr">{t}</code></summary><div className="faq-body">{d}</div></details>
          ))}
          <H2>Payment problem?</H2>
          <p>It has its own section: <button className="linklike" onClick={() => go('payment-issues')}>Payment issues</button>.</p>
        </section>
      )}

      {active === 'keys' && (
        <section>
          <H1>Keys & security</H1>
          <Lead>One key per app. Usage is recorded separately, access is restricted separately, and a leaked key doesn’t compromise the others.</Lead>
          <H2>What you can set on each key</H2>
          <ul style={{ paddingInlineStart: 20, display: 'flex', flexDirection: 'column', gap: 8 }}>
            <li><strong>Model allow-list</strong> with globs, for example <code dir="ltr">nabu-*</code> or <code dir="ltr">cine-*</code>.</li>
            <li><strong>Allowed origin</strong> (Origin/Referer) for keys used from a browser.</li>
            <li><strong>Allowed provider</strong>, if you want an app to go through a single vendor only.</li>
            <li><strong>Rate limit</strong> in requests per minute.</li>
          </ul>
          <H2>Good to know</H2>
          <Callout icon="🔒">The full key is shown only once, when it’s created, and only its hash is stored. If you lose it, create a new key and delete the old one.</Callout>
          <Callout kind="warn" icon="⚠️">Don’t put a key in frontend code unless it has a restricted origin and a small allow-list. On servers, use an environment variable.</Callout>
        </section>
      )}

      {active === 'gateway-setup' && (
        <section>
          <H1>Payment gateway setup (for deployment admins)</H1>
          <Lead>NabuGate never talks to a bank directly. Payments go through <strong>NabuPay</strong>, the payment bridge provided by NabuDesk. The gateways (Zarinpal, Aqaye Pardakht, Larapay, Stripe, PayPal, Polar, NowPayments) are configured there, and this service holds no merchant keys.</Lead>
          <H2>Environment variables</H2>
          <CodeBlock code={`NABUPAY_URL=https://desk.nabuxai.com   # payment bridge URL
NABUPAY_SECRET=...                      # shared secret; requests are signed with HMAC-SHA256
NABUPAY_APP_ID=gate                     # this service's ID at the bridge (default: gate)
NABUPAY_GATEWAY=zarinpal                # default gateway (the bridge's slug)
NABU_PUBLIC_URL=https://gate.example.com # where the payer returns; if empty, derived from the request itself`} label=".env" />
          <p style={{ marginTop: 12 }}>If <code>NABUPAY_URL</code> or <code>NABUPAY_SECRET</code> is empty, the gateway still starts, but the console reports that top-ups are unavailable and <code>/api/status</code> returns <code dir="ltr">payments_enabled: false</code>. The startup log states this explicitly.</p>
          <H2>Payment flow</H2>
          <Steps items={[
            <><code dir="ltr">POST /api/me/recharge</code> creates an invoice through the bridge’s <code dir="ltr">/api/v1/pay/checkout</code>, records it as pending against the account <em>before</em> the user leaves, and returns <code>checkout_url</code>.</>,
            <>The user goes to the bank and returns to <code dir="ltr">{'{NABU_PUBLIC_URL}'}/panel/account</code>.</>,
            <>The console calls <code dir="ltr">POST /api/me/payments/settle</code>. The server checks only <em>this account’s</em> pending invoices with the bridge (<code dir="ltr">/api/v1/pay/verify/{'{invoice}'}</code>) and credits each one that is <code>paid</code> exactly once. Nothing is read from the return query string.</>,
          ]} />
          <H2>Troubleshooting from the logs</H2>
          <ul style={{ paddingInlineStart: 20, display: 'flex', flexDirection: 'column', gap: 8 }}>
            <li><code>payment bridge refused the request (401)</code>: the secret or app ID doesn’t match the bridge, or the server clock is too far off (signatures include a timestamp).</li>
            <li><code>payment bridge refused the request (422)</code>: the bank gateway itself rejected the invoice; the bridge’s message follows on the same line.</li>
            <li><code>could not confirm a payment</code>: the bridge didn’t respond for a moment. The invoice stays pending and is checked again the next time the page is opened; the payment isn’t lost.</li>
            <li><code>wallet credited</code>: the credit was added, with the invoice number and the new balance.</li>
          </ul>
          <Callout kind="warn" icon="⚠️">
            <strong>Wrong return URL:</strong> if you’re behind a proxy and <code>NABU_PUBLIC_URL</code> isn’t set, the return URL is built from the Host header and may point back to an internal address. Set it explicitly.
          </Callout>
          <Callout icon="💡">Manual top-ups from the admin console (“Users” → top-up) need no gateway and are recorded as <code>admin-recharge</code>.</Callout>
          <p>A fuller guide lives in the repo: <code dir="ltr">docs/payments.md</code>.</p>
        </section>
      )}
    </>
  );
}
