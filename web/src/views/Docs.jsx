import { useEffect, useState } from 'react';
import CodeBlock from '../components/CodeBlock.jsx';

/*
 * Public documentation, served at /docs without a session.
 *
 * Every section is addressable (#billing, #cursor …) so a support reply can
 * point at one answer. The base URL is the host the page is served from, so a
 * self-hosted gateway documents itself without an edit.
 */
const ORIGIN = typeof window !== 'undefined' && window.location.origin.startsWith('http')
  ? window.location.origin
  : 'https://gate.nabuxai.com';
const BASE = `${ORIGIN}/v1`;
const KEY = 'ng_xxxxxxxxxxxxxxxxxxxx';

const GROUPS = [
  { title: 'شروع', items: [
    { id: 'intro', title: 'شروع سریع' },
    { id: 'billing', title: 'خرید و شارژ حساب' },
    { id: 'payment-issues', title: 'مشکلات پرداخت' },
    { id: 'models', title: 'مدل‌ها و alias‌ها' },
  ] },
  { title: 'اتصال ابزارها', items: [
    { id: 'env', title: 'متغیرهای محیطی' },
    { id: 'cursor', title: 'Cursor' },
    { id: 'cline', title: 'Cline / Roo Code / Continue' },
    { id: 'claude-code', title: 'Claude Code' },
    { id: 'codex', title: 'Codex CLI' },
    { id: 'vscode', title: 'VS Code' },
    { id: 'sdk', title: 'SDK پایتون و Node' },
    { id: 'curl', title: 'cURL' },
  ] },
  { title: 'مرجع', items: [
    { id: 'api-reference', title: 'مرجع API' },
    { id: 'errors', title: 'خطاها و عیب‌یابی' },
    { id: 'keys', title: 'کلیدها و امنیت' },
    { id: 'gateway-setup', title: 'راه‌اندازی درگاه پرداخت (مدیر)' },
  ] },
];
const ALL = GROUPS.flatMap((g) => g.items.map((i) => i.id));

function H1({ children }) {
  return <h1 style={{ fontSize: 30, marginBottom: 8, fontWeight: 900, letterSpacing: '-0.02em', lineHeight: 1.15 }}>{children}</h1>;
}
function Lead({ children }) {
  return <p style={{ color: 'var(--ng-muted)', marginBottom: 24, fontSize: 15, lineHeight: 1.9 }}>{children}</p>;
}
function H2({ children }) {
  return <h2 style={{ fontSize: 19, margin: '30px 0 12px', fontWeight: 800, letterSpacing: '-0.01em' }}>{children}</h2>;
}
function Steps({ items }) {
  return (
    <ol style={{ paddingInlineStart: 0, listStyle: 'none', margin: '0 0 20px', display: 'flex', flexDirection: 'column', gap: 10 }} className="stagger">
      {items.map((t, i) => (
        <li key={i} className="step"><span className="num">{['۱', '۲', '۳', '۴', '۵', '۶'][i]}</span><div><span style={{ color: 'var(--ng-text)', fontSize: 13.5 }}>{t}</span></div></li>
      ))}
    </ol>
  );
}
function Callout({ kind, icon, children }) {
  return <div className={'callout ' + (kind || '')} style={{ margin: '16px 0' }}><span className="ci">{icon}</span><div>{children}</div></div>;
}
function Faq({ q, children }) {
  return <details className="faq"><summary>{q}</summary><div className="faq-body">{children}</div></details>;
}

const ENV_BASH = `export OPENAI_BASE_URL="${BASE}"
export OPENAI_API_KEY="${KEY}"`;
const ENV_PS = `$env:OPENAI_BASE_URL="${BASE}"
$env:OPENAI_API_KEY="${KEY}"`;

function EnvTabs() {
  const [os, setOs] = useState('mac');
  return (
    <div>
      <div className="chips" style={{ marginBottom: 8 }}>
        {[['mac', 'macOS / Linux'], ['win', 'Windows (PowerShell)']].map(([id, l]) => (
          <button key={id} type="button" className={'chip' + (os === id ? ' active' : '')} onClick={() => setOs(id)}>{l}</button>
        ))}
      </div>
      <CodeBlock code={os === 'win' ? ENV_PS : ENV_BASH} label={os === 'win' ? 'PowerShell' : 'bash · zsh'} />
    </div>
  );
}

export default function Docs() {
  const fromHash = () => {
    const h = window.location.hash.replace(/^#\/?/, '');
    return ALL.includes(h) ? h : 'intro';
  };
  const [active, setActive] = useState(fromHash);

  useEffect(() => {
    const onHash = () => setActive(fromHash());
    window.addEventListener('hashchange', onHash);
    return () => window.removeEventListener('hashchange', onHash);
  }, []);

  const go = (id) => {
    window.history.replaceState(null, '', '#' + id);
    setActive(id);
    window.scrollTo({ top: 0, behavior: 'smooth' });
  };

  return (
    <div style={{ minHeight: '100vh', display: 'flex', flexDirection: 'column', background: 'var(--ng-bg)' }}>
      <header className="jv-header">
        <div className="jv-container" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <a href="/" className="jv-logo">
            <span style={{ background: 'var(--ng-accent-grad)', WebkitBackgroundClip: 'text', color: 'transparent', fontWeight: 900, fontSize: 22, letterSpacing: '-0.02em' }}>NabuGate Docs</span>
          </a>
          <nav className="jv-nav hidden-mobile">
            <a href="/">صفحهٔ اصلی</a>
            <a href="/panel/plans">پلن‌ها</a>
            <a href="/panel/">ورود به پنل</a>
          </nav>
        </div>
      </header>

      <main style={{ flex: 1, display: 'flex', maxWidth: 1200, margin: '0 auto', width: '100%', padding: '36px 20px', gap: 36, flexWrap: 'wrap' }}>
        <aside style={{ width: 240, flexShrink: 0, position: 'sticky', top: 84, alignSelf: 'flex-start' }}>
          {GROUPS.map((g) => (
            <div key={g.title} style={{ marginBottom: 18 }}>
              <h3 style={{ fontSize: 11, color: 'var(--ng-subtle)', margin: '0 12px 8px', letterSpacing: '0.06em', fontWeight: 700 }}>{g.title}</h3>
              <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'flex', flexDirection: 'column', gap: 2 }}>
                {g.items.map((t) => (
                  <li key={t.id}>
                    <button
                      type="button"
                      className={'nav-item' + (active === t.id ? ' active' : '')}
                      onClick={() => go(t.id)}
                      style={{ padding: '8px 12px', fontSize: 13 }}
                    >{t.title}</button>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </aside>

        <div key={active} className="view-enter" style={{ flex: 1, minWidth: 280, color: 'var(--ng-heading)', lineHeight: 1.85, maxWidth: 820 }}>
          {active === 'intro' && (
            <section>
              <H1>شروع سریع</H1>
              <Lead>NabuGate یک درگاه سازگار با OpenAI است: یک آدرس، یک کلید، دسترسی به مدل‌های OpenAI، Anthropic، Google و بقیه با fallback خودکار. هر ابزار یا کتابخانه‌ای که با OpenAI کار می‌کند، با تغییر آدرس پایه با NabuGate هم کار می‌کند.</Lead>
              <Steps items={[
                <>در <a href="/panel/">پنل</a> ثبت‌نام کنید. حساب با ایمیل ساخته می‌شود؛ ورود با حساب گوگل و نابو هم هست.</>,
                <>از «خرید و شارژ» حساب را شارژ کنید. پرداخت به‌ازای مصرف است و موجودی منقضی نمی‌شود. <button className="linklike" onClick={() => go('billing')}>جزئیات</button></>,
                <>در «کلیدهای API» یک کلید بسازید. با <code>ng_</code> شروع می‌شود و فقط یک‌بار کامل نمایش داده می‌شود.</>,
                <>آدرس پایه را روی <code dir="ltr">{BASE}</code> و مدل را روی یکی از نام‌های <code dir="ltr">GET {BASE}/models</code> بگذارید (مثلاً <code>nabu-fast</code>).</>,
              ]} />
              <H2>سه مقداری که همه‌جا لازم است</H2>
              <div className="card card-flat" style={{ padding: 18 }}>
                <div className="kv">
                  <div className="line"><span>Base URL</span><strong className="mono" dir="ltr">{BASE}</strong></div>
                  <div className="line"><span>API Key</span><strong className="mono" dir="ltr">ng_…</strong></div>
                  <div className="line"><span>Model</span><strong className="mono" dir="ltr">nabu-fast · nabu-smart · openai/gpt-… · anthropic/claude-…</strong></div>
                </div>
              </div>
              <H2>متغیرهای محیطی</H2>
              <EnvTabs />
              <Callout icon="💡"><strong>برای Claude Code آدرس پایه فرق دارد:</strong> بدون <code>/v1</code> و با متغیرهای <code>ANTHROPIC_*</code>. <button className="linklike" onClick={() => go('claude-code')}>راهنمای Claude Code</button></Callout>
            </section>
          )}

          {active === 'billing' && (
            <section>
              <H1>خرید و شارژ حساب</H1>
              <Lead>NabuGate اشتراک ماهانه ندارد. یک کیف پول دلاری دارید که با هر درخواست، دقیقاً به اندازهٔ قیمت واقعی مدل از آن کم می‌شود. هر وقت خواستید شارژ می‌کنید و موجودی هرگز منقضی نمی‌شود.</Lead>
              <H2>پرداخت چطور انجام می‌شود</H2>
              <Steps items={[
                'در پنل به «خرید و شارژ» بروید و یک بسته یا مبلغ دلخواه (۱ تا ۵٬۰۰۰ دلار) انتخاب کنید.',
                'به درگاه بانکی منتقل می‌شوید. مبلغ به تومان و با نرخ روز نشان داده می‌شود. اطلاعات کارت را همان‌جا وارد می‌کنید؛ این پنل هیچ اطلاعات کارتی نمی‌بیند و ذخیره نمی‌کند.',
                'بعد از پرداخت به «حساب و مصرف» برمی‌گردید. پنل از خودِ درگاه می‌پرسد پول رسیده یا نه و تنها بعد از تأیید، موجودی را اضافه می‌کند.',
                'کلیدها همان لحظه با موجودی جدید کار می‌کنند. لازم نیست کلید جدید بسازید.',
              ]} />
              <H2>چرا دلار؟</H2>
              <p>دروازه قیمت هر مدل را به دلار حساب می‌کند (همان واحدی که ارائه‌دهنده‌ها اعلام می‌کنند) و موجودی هم با همان واحد نگه‌داری می‌شود تا یک عدد در دو جا دو معنی نداشته باشد. معادل تومانی را درگاه بانکی در لحظهٔ پرداخت نشان می‌دهد و روی رسید بانک است.</p>
              <H2>هزینهٔ هر درخواست چقدر است؟</H2>
              <p>قیمت هر مدل به‌ازای یک میلیون توکن ورودی و خروجی تعیین می‌شود و در «مدل‌ها» قابل مشاهده است. هر پاسخ دو هدر دارد تا هرگز غافلگیر نشوید:</p>
              <CodeBlock code={`X-Nabu-Balance-USD: 4.1837       # باقی‌ماندهٔ موجودی بعد از همین درخواست
X-Nabu-Balance-Warning: low       # فقط وقتی موجودی زیر ۱ دلار باشد`} label="response headers" />
              <p style={{ marginTop: 12 }}>با هر کلید می‌توانید مصرف را جداگانه ببینید؛ برای هر برنامه یک کلید بسازید تا بدانید پول کجا خرج شده.</p>
              <H2>وقتی موجودی تمام شود</H2>
              <p>درخواست‌ها با کد <code>402 Payment Required</code> رد می‌شوند و در «درخواست‌های اخیر» با دلیل «insufficient balance» ثبت می‌شوند. با هر مبلغی شارژ کنید، همان لحظه کلید دوباره کار می‌کند.</p>
              <Callout kind="ok" icon="✓">موجودی قابل انتقال بین حساب‌ها یا برداشت نقدی نیست، ولی هیچ‌وقت هم از بین نمی‌رود.</Callout>
            </section>
          )}

          {active === 'payment-issues' && (
            <section>
              <H1>مشکلات پرداخت</H1>
              <Lead>قاعدهٔ کلی: پنل به چیزی که در آدرسِ بازگشت از بانک نوشته شده اعتماد نمی‌کند. هر بار که «حساب و مصرف» یا «پرداخت‌ها» را باز کنید، وضعیت فاکتورهای در انتظار مستقیم از خودِ درگاه پرسیده می‌شود و به‌محض تأیید، موجودی یک‌بار اضافه می‌شود. پس اولین کار برای هر مشکلی: صفحه را دوباره باز کنید یا «بررسی وضعیت» را بزنید.</Lead>
              <Faq q="پول از حسابم کم شد ولی موجودی اضافه نشد">
                به «پرداخت‌ها» بروید و «بررسی وضعیت» را بزنید. اگر بانک تراکنش را تأیید کرده باشد، همان لحظه اضافه می‌شود. بعضی درگاه‌ها (مثلاً پرداخت رمزارزی) چند دقیقه تا تأیید فاصله دارند. اگر بعد از ۳۰ دقیقه هنوز «در انتظار» است، شناسهٔ فاکتور را برای پشتیبانی بفرستید. پرداختی که هرگز تأیید نشود ظرف ۷۲ ساعت از طرف بانک به کارت برمی‌گردد.
              </Faq>
              <Faq q="بعد از پرداخت صفحهٔ خطا دیدم یا مرورگر بسته شد">
                مهم نیست. فاکتور پیش از رفتن به بانک به نام حساب شما ثبت شده و تأیید آن به مرورگرتان وابسته نیست. وارد پنل شوید و «پرداخت‌ها» را باز کنید.
              </Faq>
              <Faq q="خطای «درگاه پرداخت آدرسی برای ادامه نداد»">
                پل پرداخت نتوانست از بانک فاکتور بگیرد؛ معمولاً قطعی موقت درگاه است. چند دقیقه بعد دوباره تلاش کنید. اگر تکرار شد، مدیر باید لاگ دروازه را برای <code>payment bridge refused</code> ببیند (بخش راه‌اندازی درگاه).
              </Faq>
              <Faq q="پیام «no payment gateway is configured on this deployment»">
                این استقرار درگاه پرداخت ندارد؛ دکمه‌های پرداخت کار نمی‌کنند تا مدیر متغیرهای <code>NABUPAY_URL</code> و <code>NABUPAY_SECRET</code> را تنظیم کند. برای شارژ دستی با پشتیبانی تماس بگیرید.
              </Faq>
              <Faq q="دو بار پرداخت کردم">
                هر فاکتور فقط یک‌بار به موجودی اضافه می‌شود و رفرش‌کردن صفحه چیزی را دوبار حساب نمی‌کند. اگر واقعاً دو تراکنش موفق در بانک دارید، هر دو در «پرداخت‌ها» با شناسهٔ جدا دیده می‌شوند و هر دو به موجودی اضافه شده‌اند. برای بازگشت یکی، شناسهٔ فاکتور را به پشتیبانی بدهید.
              </Faq>
              <Faq q="مبلغ روی صفحهٔ بانک با چیزی که انتخاب کردم فرق دارد">
                شما دلار انتخاب می‌کنید و بانک تومان می‌گیرد؛ تبدیل با نرخ لحظه‌ای درگاه انجام می‌شود. آنچه به موجودی اضافه می‌شود همان مبلغ دلاری انتخابی است، مستقل از نرخ آن روز.
              </Faq>
              <Faq q="کلیدم بعد از شارژ هنوز ۴۰۲ می‌دهد">
                در «حساب و مصرف» موجودی را ببینید. اگر هنوز صفر است، پرداخت تأیید نشده؛ «بررسی دوباره» را بزنید. اگر موجودی مثبت است ولی ۴۰۲ می‌گیرید، کلید متعلق به حساب دیگری است؛ در «کلیدهای API» مطمئن شوید همان کلیدی را استفاده می‌کنید که در این حساب ساخته شده.
              </Faq>
              <H2>برای پیگیری چه چیزی لازم است</H2>
              <p>شناسهٔ فاکتور از جدول «پرداخت‌ها»، تاریخ و مبلغ، و ایمیل حساب. شمارهٔ کارت یا رسید کامل بانک لازم نیست.</p>
            </section>
          )}

          {active === 'models' && (
            <section>
              <H1>مدل‌ها و alias‌ها</H1>
              <Lead>به‌جای نام یک مدل خاص، معمولاً یک <strong>alias</strong> صدا می‌زنید: نامی مثل <code>nabu-fast</code> که دروازه پشتش یک زنجیرهٔ مدل از چند ارائه‌دهنده گذاشته. اگر اولی قطع شود یا پاسخ خالی بدهد، بعدی امتحان می‌شود و شما فقط پاسخ را می‌بینید.</Lead>
              <H2>فهرست زنده</H2>
              <CodeBlock code={`curl ${BASE}/models -H "Authorization: Bearer ${KEY}"`} />
              <p style={{ marginTop: 12 }}>خروجی همهٔ alias‌ها، ساب‌ایجنت‌ها، فلوها و مدل‌های مستقیم را دارد. همین نام‌ها را در فیلد <code>model</code> بگذارید.</p>
              <H2>سه نوع نام</H2>
              <ul style={{ paddingInlineStart: 20, display: 'flex', flexDirection: 'column', gap: 8 }}>
                <li><strong>alias</strong> مثل <code>nabu-fast</code>، <code>nabu-smart</code>، <code>nabu-embed</code>: زنجیرهٔ چند ارائه‌دهنده با fallback. برای کار روزمره این را انتخاب کنید.</li>
                <li><strong>مدل مستقیم</strong> مثل <code dir="ltr">openai/gpt-…</code> یا <code dir="ltr">anthropic/claude-…</code>: بدون fallback، دقیقاً همان مدل. وقتی نتیجهٔ تکرارپذیر می‌خواهید.</li>
                <li><strong>ایجنت / فلو</strong> مثل <code>cine-motion-designer</code>: یک دستیار با system prompt آماده که مثل یک مدل صدا زده می‌شود.</li>
              </ul>
              <Callout kind="warn" icon="⚠️">برای <strong>ساخت ایندکس برداری</strong> از alias با fallback استفاده نکنید؛ عرض بردار بین ارائه‌دهنده‌ها فرق دارد. alias بدون fallback مثل <code>write-embed</code> را بگیرید.</Callout>
            </section>
          )}

          {active === 'env' && (
            <section>
              <H1>متغیرهای محیطی</H1>
              <Lead>بیشتر ابزارها و کتابخانه‌ها همین دو متغیر را می‌خوانند و به کد دست نمی‌زنید.</Lead>
              <EnvTabs />
              <H2>دائمی کردن</H2>
              <p>در macOS/Linux دو خط را به <code dir="ltr">~/.zshrc</code> یا <code dir="ltr">~/.bashrc</code> اضافه کنید. در Windows از «Edit environment variables for your account» یا <code dir="ltr">[Environment]::SetEnvironmentVariable(...)</code> در PowerShell.</p>
              <Callout icon="💡">SDK‌های رسمی OpenAI (پایتون، Node، Go و…) بدون هیچ آرگومانی همین دو متغیر را برمی‌دارند.</Callout>
            </section>
          )}

          {active === 'cursor' && (
            <section>
              <H1>اتصال به Cursor</H1>
              <Lead>Cursor اجازه می‌دهد آدرس پایهٔ OpenAI را بازنویسی کنید. بعد از آن هر مدلی که NabuGate دارد در Cursor قابل انتخاب است.</Lead>
              <Steps items={[
                <>Settings → Models را باز کنید.</>,
                <>در بخش OpenAI API Key کلید <code>ng_…</code> را وارد کنید.</>,
                <>گزینهٔ <strong>Override OpenAI Base URL</strong> را فعال کنید و <code dir="ltr">{BASE}</code> را بنویسید.</>,
                <>با «+ Add model» نام alias را اضافه کنید (مثلاً <code>nabu-smart</code>) و Verify بزنید.</>,
              ]} />
              <Callout kind="warn" icon="⚠️">Cursor برای Verify یک درخواست کوچک می‌فرستد؛ اگر موجودی صفر باشد با ۴۰۲ شکست می‌خورد. اول شارژ کنید.</Callout>
            </section>
          )}

          {active === 'cline' && (
            <section>
              <H1>Cline، Roo Code و Continue</H1>
              <Lead>هر سه ارائه‌دهندهٔ «OpenAI Compatible» را می‌پذیرند و سه مقدار می‌گیرند.</Lead>
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
              <Callout icon="💡">برای کدنویسی <code>nabu-smart</code> و برای کارهای سریع و ارزان <code>nabu-fast</code>. tool-calling کامل پشتیبانی می‌شود.</Callout>
            </section>
          )}

          {active === 'claude-code' && (
            <section>
              <H1>Claude Code</H1>
              <Lead>Claude Code با پروتکل بومی Anthropic حرف می‌زند و خودش <code>/v1/messages</code> را به آدرس پایه اضافه می‌کند. پس آدرس پایه <strong>بدون</strong> <code>/v1</code> است و هر دو متغیر احراز هویت باید ست شوند.</Lead>
              <CodeBlock code={`export ANTHROPIC_BASE_URL="${ORIGIN}"
export ANTHROPIC_API_KEY="${KEY}"
export ANTHROPIC_AUTH_TOKEN="${KEY}"
export ANTHROPIC_MODEL="nabu-smart"
export ANTHROPIC_SMALL_FAST_MODEL="nabu-fast"

claude`} label="bash · zsh" />
              <H2>تنظیم دائمی برای CLI و افزونهٔ VS Code</H2>
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
                <strong>۴۰۱</strong> یعنی یکی از دو متغیر <code>ANTHROPIC_API_KEY</code> / <code>ANTHROPIC_AUTH_TOKEN</code> جا مانده. <strong>خطای مسیر</strong> یعنی آدرس پایه <code>/v1</code> دارد. <strong>مدل پیدا نشد</strong> یعنی <code>ANTHROPIC_MODEL</code> پین نشده و Claude Code نام پیش‌فرض خودش را فرستاده.
              </Callout>
            </section>
          )}

          {active === 'codex' && (
            <section>
              <H1>Codex CLI</H1>
              <Lead>Codex از واسط Responses استفاده می‌کند که دروازه پشتیبانی می‌کند؛ باید یک ارائه‌دهندهٔ سفارشی با <code dir="ltr">wire_api = "responses"</code> تعریف کنید.</Lead>
              <CodeBlock code={`model = "nabu-smart"
model_provider = "nabugate"

[model_providers.nabugate]
name = "NabuGate"
base_url = "${BASE}"
env_key = "OPENAI_API_KEY"
wire_api = "responses"`} label="~/.codex/config.toml" />
              <p style={{ marginTop: 12 }}>و کلید را در متغیر محیطی بگذارید:</p>
              <CodeBlock code={`export OPENAI_API_KEY="${KEY}"`} />
            </section>
          )}

          {active === 'vscode' && (
            <section>
              <H1>VS Code</H1>
              <Lead>سه راه: افزونهٔ Claude Code، افزونهٔ Codex، یا افزونه‌های سازگار با OpenAI (Cline، Roo Code، Continue).</Lead>
              <H2>افزونهٔ Claude Code</H2>
              <p>همان <code dir="ltr">~/.claude/settings.json</code> بخش <button className="linklike" onClick={() => go('claude-code')}>Claude Code</button> را می‌خواند؛ چیزی جدا لازم نیست.</p>
              <H2>افزونهٔ Codex</H2>
              <p>همان <code dir="ltr">~/.codex/config.toml</code> بخش <button className="linklike" onClick={() => go('codex')}>Codex</button>.</p>
              <H2>Cline / Roo / Continue</H2>
              <p>راهنمای <button className="linklike" onClick={() => go('cline')}>OpenAI Compatible</button>.</p>
              <H2>ترمینال داخلی VS Code</H2>
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
              <H1>SDK پایتون و Node</H1>
              <Lead>کتابخانه‌های رسمی OpenAI بدون تغییر کار می‌کنند؛ فقط آدرس پایه و کلید را بدهید.</Lead>
              <H2>Python</H2>
              <CodeBlock code={`from openai import OpenAI

client = OpenAI(base_url="${BASE}", api_key="${KEY}")

r = client.chat.completions.create(
    model="nabu-fast",
    messages=[{"role": "user", "content": "سلام!"}],
)
print(r.choices[0].message.content)`} label="python" />
              <H2>Node.js / TypeScript</H2>
              <CodeBlock code={`import OpenAI from "openai";

const client = new OpenAI({ baseURL: "${BASE}", apiKey: "${KEY}" });

const r = await client.chat.completions.create({
  model: "nabu-fast",
  messages: [{ role: "user", content: "سلام!" }],
});
console.log(r.choices[0].message.content);`} label="node" />
              <H2>استریم</H2>
              <CodeBlock code={`stream = client.chat.completions.create(model="nabu-fast", messages=msgs, stream=True)
for chunk in stream:
    print(chunk.choices[0].delta.content or "", end="", flush=True)`} label="python" />
              <H2>Embedding با عرض ثابت</H2>
              <CodeBlock code={`client.embeddings.create(model="write-embed", input=["متن"], dimensions=1536)`} label="python" />
              <Callout icon="💡">SDK‌های اختصاصی هم هست: <code>@nabugate/sdk</code> برای npm، <code>nabugate</code> برای PyPI، Go، Rust، Dart و Laravel. همه همان یک آدرس و کلید را می‌گیرند.</Callout>
            </section>
          )}

          {active === 'curl' && (
            <section>
              <H1>cURL</H1>
              <Lead>تست سریع از ترمینال. هر مثال را کپی کنید و کلیدتان را جایگزین کنید.</Lead>
              <H2>چت</H2>
              <CodeBlock code={`curl ${BASE}/chat/completions \\
  -H "Authorization: Bearer ${KEY}" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"nabu-fast","messages":[{"role":"user","content":"سلام، تو کی هستی؟"}]}'`} />
              <H2>فهرست مدل‌ها</H2>
              <CodeBlock code={`curl ${BASE}/models -H "Authorization: Bearer ${KEY}"`} />
              <H2>مصرف همین کلید</H2>
              <CodeBlock code={`curl ${BASE}/usage -H "Authorization: Bearer ${KEY}"`} />
              <H2>دیدن موجودی از روی هدر پاسخ</H2>
              <CodeBlock code={`curl -si ${BASE}/chat/completions -H "Authorization: Bearer ${KEY}" \\
  -H "Content-Type: application/json" -d '{"model":"nabu-fast","messages":[{"role":"user","content":"hi"}]}' \\
  | grep -i x-nabu-balance`} />
            </section>
          )}

          {active === 'api-reference' && (
            <section>
              <H1>مرجع API</H1>
              <Lead>همهٔ مسیرها با استاندارد OpenAI سازگارند. بدنهٔ درخواست دست‌نخورده به ارائه‌دهنده می‌رسد؛ فقط <code>model</code> و پرچم‌های stream بازنویسی می‌شوند. پس <code>tools</code>، <code>response_format</code>، <code>seed</code>، <code>top_p</code> و … همه کار می‌کنند.</Lead>
              <div className="stagger">
                {[
                  ['POST', '/v1/chat/completions', 'چت و بینایی؛ استریم و tool calling کامل.'],
                  ['POST', '/v1/responses', 'واسط Responses، برای Codex و کلاینت‌هایی که این شکل را می‌خواهند.'],
                  ['POST', '/v1/embeddings', 'بردار برای RAG. پارامتر dimensions تا خودِ ارائه‌دهنده می‌رود.'],
                  ['POST', '/v1/images/generations', 'تولید تصویر؛ با alias عکس، جست‌وجوی تصویر استوک.'],
                  ['POST', '/v1/audio/speech', 'تبدیل متن به گفتار.'],
                  ['POST', '/v1/audio/transcriptions', 'تبدیل گفتار به متن.'],
                  ['GET', '/v1/models', 'فهرست زندهٔ alias‌ها، ایجنت‌ها، فلوها و مدل‌های مستقیم.'],
                  ['GET', '/v1/agents', 'ساب‌ایجنت‌ها با مدل و ابزارهایشان.'],
                  ['GET', '/v1/usage', 'مصرف همین کلید: توکن و هزینه به تفکیک مدل.'],
                  ['GET', '/v1/photos/search', 'جست‌وجوی عکس استوک بدون تولید تصویر.'],
                  ['GET', '/v1/health', 'سلامت دروازه.'],
                ].map(([m, p, d]) => (
                  <div key={p} className="card card-flat" style={{ padding: '14px 18px', marginBottom: 10, display: 'flex', gap: 14, alignItems: 'flex-start', flexWrap: 'wrap' }}>
                    <span className={'badge ' + (m === 'GET' ? 'badge-info' : 'badge-ok')} style={{ fontFamily: 'var(--ng-mono)' }}>{m}</span>
                    <code style={{ fontSize: 14, fontWeight: 700, minWidth: 220 }} dir="ltr">{p}</code>
                    <span style={{ color: 'var(--ng-muted)', fontSize: 13, flex: 1 }}>{d}</span>
                  </div>
                ))}
              </div>
              <H2>هدرهای پاسخ</H2>
              <ul style={{ paddingInlineStart: 20 }}>
                <li><code>X-Nabu-Balance-USD</code>: موجودی بعد از همین درخواست.</li>
                <li><code>X-Nabu-Balance-Warning: low</code>: موجودی زیر ۱ دلار.</li>
                <li><code>X-Nabu-Agent</code>: وقتی مدل یک ساب‌ایجنت بوده، نامش.</li>
              </ul>
            </section>
          )}

          {active === 'errors' && (
            <section>
              <H1>خطاها و عیب‌یابی</H1>
              <Lead>هر خطا در «درخواست‌های اخیر» با دلیلش ثبت می‌شود؛ اول آنجا را ببینید.</Lead>
              {[
                ['401 Unauthorized', 'کلید اشتباه، حذف‌شده یا غیرفعال است، یا هدر Authorization: Bearer ندارد. برای Claude Code هر دو متغیر ANTHROPIC_API_KEY و ANTHROPIC_AUTH_TOKEN لازم است.'],
                ['402 Payment Required', 'موجودی حساب صاحب کلید صفر است. از «خرید و شارژ» شارژ کنید؛ نیازی به کلید جدید نیست.'],
                ['403 Forbidden', 'کلید از این مبدأ (Origin) مجاز نیست، یا مدل در allow-list کلید نیست. در «کلیدهای API» مبدأ و دسترسی را ببینید.'],
                ['404 model not found', 'نام مدل یکی از نام‌های GET /v1/models نیست. حروف کوچک و پیشوند ارائه‌دهنده (openai/…) را چک کنید.'],
                ['429 Too Many Requests', 'به سقف rate-limit کلید رسیده‌اید. برای برنامه‌های پرمصرف کلید جدا با سقف بالاتر بسازید.'],
                ['502 all targets failed', 'همهٔ ارائه‌دهنده‌های زنجیرهٔ alias شکست خوردند. معمولاً موقتی است؛ چند ثانیه بعد تلاش کنید یا مدل مستقیم دیگری را امتحان کنید.'],
                ['پاسخ خالی', 'اگر استریم بدون محتوا بسته شود، دروازه خودش سراغ target بعدی می‌رود. اگر همچنان خالی است، مدل مستقیم را با همان پیام امتحان کنید و به پشتیبانی بگویید.'],
                ['عرض بردار عوض شد', 'alias embedding با fallback از عرض‌های مختلف عبور می‌کند. برای ایندکس ذخیره‌شده از alias بدون fallback (write-embed) و dimensions ثابت استفاده کنید.'],
              ].map(([t, d]) => (
                <details key={t} className="faq"><summary dir="auto"><code dir="ltr">{t}</code></summary><div className="faq-body">{d}</div></details>
              ))}
              <H2>مشکل پرداخت؟</H2>
              <p>بخش جداگانه: <button className="linklike" onClick={() => go('payment-issues')}>مشکلات پرداخت</button>.</p>
            </section>
          )}

          {active === 'keys' && (
            <section>
              <H1>کلیدها و امنیت</H1>
              <Lead>برای هر برنامه یک کلید. مصرف جدا ثبت می‌شود، دسترسی جدا محدود می‌شود و لو رفتن یکی بقیه را نمی‌سوزاند.</Lead>
              <H2>چه چیزی روی هر کلید تنظیم می‌شود</H2>
              <ul style={{ paddingInlineStart: 20, display: 'flex', flexDirection: 'column', gap: 8 }}>
                <li><strong>allow-list مدل‌ها</strong> با گلاب، مثلاً <code dir="ltr">nabu-*</code> یا <code dir="ltr">cine-*</code>.</li>
                <li><strong>مبدأ مجاز</strong> (Origin/Referer) برای کلیدهایی که از مرورگر استفاده می‌شوند.</li>
                <li><strong>ارائه‌دهندهٔ مجاز</strong>، اگر بخواهید یک برنامه فقط از یک vendor بگذرد.</li>
                <li><strong>rate-limit</strong> درخواست در دقیقه.</li>
              </ul>
              <H2>نکات</H2>
              <Callout icon="🔒">متن کامل کلید فقط یک‌بار موقع ساخت نمایش داده می‌شود و فقط هَش آن ذخیره می‌شود. اگر گم شد، کلید جدید بسازید و قدیمی را حذف کنید.</Callout>
              <Callout kind="warn" icon="⚠️">کلید را در کد فرانت‌اند نگذارید مگر با مبدأ محدود و allow-list کوچک. برای سرور از متغیر محیطی استفاده کنید.</Callout>
            </section>
          )}

          {active === 'gateway-setup' && (
            <section>
              <H1>راه‌اندازی درگاه پرداخت (برای مدیر استقرار)</H1>
              <Lead>NabuGate خودش با بانک حرف نمی‌زند. پرداخت‌ها از <strong>NabuPay</strong>، پل پرداختی که NabuDesk ارائه می‌کند، می‌گذرند؛ درگاه‌ها (زرین‌پال، آقای پرداخت، Larapay، Stripe، PayPal، Polar، NowPayments) آن‌جا پیکربندی می‌شوند و هیچ کلید مرچنتی در این سرویس نیست.</Lead>
              <H2>متغیرهای محیطی</H2>
              <CodeBlock code={`NABUPAY_URL=https://desk.nabuxai.com   # آدرس پل پرداخت
NABUPAY_SECRET=...                      # سکرت مشترک؛ درخواست‌ها با HMAC-SHA256 امضا می‌شوند
NABUPAY_APP_ID=gate                     # شناسهٔ این سرویس نزد پل (پیش‌فرض gate)
NABUPAY_GATEWAY=zarinpal                # درگاه پیش‌فرض (اسلاگ پل)
NABU_PUBLIC_URL=https://gate.example.com # آدرس بازگشت پرداخت‌کننده؛ اگر خالی باشد از خود درخواست ساخته می‌شود`} label=".env" />
              <p style={{ marginTop: 12 }}>اگر <code>NABUPAY_URL</code> یا <code>NABUPAY_SECRET</code> خالی باشد، دروازه بالا می‌آید ولی پنل می‌گوید شارژ در دسترس نیست و <code>/api/status</code> مقدار <code dir="ltr">payments_enabled: false</code> برمی‌گرداند. لاگ شروع سرویس این را صریح می‌نویسد.</p>
              <H2>جریان پرداخت</H2>
              <Steps items={[
                <><code dir="ltr">POST /api/me/recharge</code> روی پل <code dir="ltr">/api/v1/pay/checkout</code> فاکتور می‌سازد، فاکتور را <em>قبل از</em> رفتن کاربر با وضعیت pending به نام حساب ثبت می‌کند و <code>checkout_url</code> را برمی‌گرداند.</>,
                <>کاربر به بانک می‌رود و به <code dir="ltr">{'{NABU_PUBLIC_URL}'}/panel/account</code> برمی‌گردد.</>,
                <>پنل <code dir="ltr">POST /api/me/payments/settle</code> را می‌زند. سرور فقط فاکتورهای pending <em>همین حساب</em> را از پل (<code dir="ltr">/api/v1/pay/verify/{'{invoice}'}</code>) می‌پرسد و اگر <code>paid</code> بود یک‌بار اعتبار می‌دهد. هیچ چیزی از query string بازگشت خوانده نمی‌شود.</>,
              ]} />
              <H2>عیب‌یابی از لاگ</H2>
              <ul style={{ paddingInlineStart: 20, display: 'flex', flexDirection: 'column', gap: 8 }}>
                <li><code>payment bridge refused the request (401)</code>: سکرت یا app id با پل نمی‌خواند، یا ساعت سرور بیش از حد جلو/عقب است (امضا timestamp دارد).</li>
                <li><code>payment bridge refused the request (422)</code>: خودِ درگاه بانکی فاکتور را رد کرده؛ پیام پل در ادامهٔ خط است.</li>
                <li><code>could not confirm a payment</code>: پل موقتاً پاسخ نداد. فاکتور pending می‌ماند و با باز شدن بعدیِ صفحه دوباره پرسیده می‌شود؛ پرداخت گم نمی‌شود.</li>
                <li><code>wallet credited</code>: اعتبار اضافه شد، با شماره فاکتور و موجودی جدید.</li>
              </ul>
              <Callout kind="warn" icon="⚠️">
                <strong>آدرس بازگشت غلط:</strong> اگر پشت پروکسی هستید و <code>NABU_PUBLIC_URL</code> ست نیست، آدرس بازگشت از هدر Host ساخته می‌شود و ممکن است به آدرس داخلی برگردد. آن را صریح ست کنید.
              </Callout>
              <Callout icon="💡">شارژ دستی از پنل مدیریت («کاربران» → شارژ) بدون درگاه انجام می‌شود و به‌صورت <code>admin-recharge</code> ثبت می‌شود.</Callout>
              <p>راهنمای کامل‌تر در repo: <code dir="ltr">docs/payments.md</code>.</p>
            </section>
          )}
        </div>
      </main>
    </div>
  );
}
