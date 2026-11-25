import { useEffect, useRef, useState } from 'react';
import Icon from '../../components/Icon.jsx';
import CodeBlock from '../../components/CodeBlock.jsx';
import { fmtInt, usd, useT } from '../../i18n/index.jsx';
import { gatewayBase } from '../../origin.js';
import { markEngagement } from './engagement.js';
import '../../styles/trylive.css';

/*
 * The landing page's live demo: a visitor asks a question and a real model
 * answers through the gateway's fallback chain, with the route, the latency
 * and the cost on screen, then the same call as code to copy.
 *
 * The server fences it (internal/server/demo.go): one alias, short prompts,
 * short answers, five tries per visitor per hour and a daily dollar cap. Its
 * refusals come back with an error code, translated here.
 */

const MAX_CHARS = 400;

const T = {
  fa: {
    eyebrow: 'امتحانِ زنده · بدونِ ثبت‌نام',
    title: 'یک سؤال بپرس؛ جوابِ واقعی را از دروازه ببین',
    sub: 'یک مدلِ واقعی از مسیرِ fallbackِ نبوگیت جواب می‌دهد. می‌بینی کدام مدل جواب داد، چقدر طول کشید و چقدر خرج داشت.',
    placeholder: 'سؤالت را بنویس…',
    samples: [
      'یک ایمیلِ کوتاه برای پیگیریِ مشتری بنویس',
      'فرقِ GPT و Claude را در سه خط بگو',
      'یک تابعِ پایتون بنویس که تکراری‌های لیست را حذف کند',
    ],
    send: 'بفرست',
    sending: 'در حالِ جواب…',
    triesLeft: ({ n }) => `${fmtInt(n)} امتحانِ دیگر در این ساعت`,
    route: 'مسیرِ این درخواست',
    served: 'جواب داد',
    latency: 'زمان',
    tokens: 'توکن',
    cost: 'هزینه',
    ms: 'میلی‌ثانیه',
    empty: 'جواب این‌جا ظاهر می‌شود — همراهِ مسیر، زمان و هزینهٔ واقعی.',
    codeTitle: 'همین درخواست از کدِ خودت:',
    getKey: 'کلیدِ خودت را بگیر — از ۱ دلار',
    console: 'برو به پنل',
    errors: {
      demo_rate_limited: 'پنج امتحانِ این ساعت تمام شد. با یک حسابِ رایگان بدونِ محدودیت ادامه بده.',
      demo_budget: 'سهمِ امروزِ دمو تمام شد؛ فردا دوباره باز می‌شود. با حسابِ خودت همین حالا ادامه بده.',
      demo_busy: 'دمو الان شلوغ است؛ چند ثانیهٔ دیگر دوباره بفرست.',
      demo_disabled: 'دموی زنده روی این سرور فعال نیست.',
      upstream_failed: 'همهٔ مدل‌های پشتِ دمو الان جواب ندادند؛ کمی بعد دوباره امتحان کن.',
      bad_prompt: 'سؤال باید بینِ ۱ تا ۴۰۰ نویسه باشد.',
      network: 'ارتباط برقرار نشد؛ اینترنت را بررسی کن و دوباره بفرست.',
    },
  },
  en: {
    eyebrow: 'Try it live · no sign-up',
    title: 'Ask anything. Watch the gateway answer.',
    sub: 'A real model answers through NabuGate’s fallback chain. You see which model served it, how long it took and what it cost.',
    placeholder: 'Type your question…',
    samples: [
      'Write a short follow-up email to a customer',
      'Explain GPT vs Claude in three lines',
      'Write a Python function that removes duplicates from a list',
    ],
    send: 'Send',
    sending: 'Answering…',
    triesLeft: ({ n }) => `${n} more ${n === 1 ? 'try' : 'tries'} this hour`,
    route: 'Route of this request',
    served: 'served',
    latency: 'Latency',
    tokens: 'Tokens',
    cost: 'Cost',
    ms: 'ms',
    empty: 'The answer appears here — with its real route, latency and cost.',
    codeTitle: 'The same request from your own code:',
    getKey: 'Get your own key — from $1',
    console: 'Open the console',
    errors: {
      demo_rate_limited: 'That’s five tries this hour. A free account keeps going with no limit.',
      demo_budget: 'Today’s demo budget is used up; it reopens tomorrow. Your own account works right now.',
      demo_busy: 'The demo is busy right now; send again in a few seconds.',
      demo_disabled: 'The live demo isn’t enabled on this server.',
      upstream_failed: 'Every model behind the demo failed just now; try again shortly.',
      bad_prompt: 'Your question must be 1 to 400 characters.',
      network: 'Couldn’t connect; check your internet and send again.',
    },
  },
  ar: {
    eyebrow: 'جرّب مباشرة · دون تسجيل',
    title: 'اسأل أي سؤال وشاهد البوابة تجيب',
    sub: 'يجيب نموذج حقيقي عبر سلسلة التحويل في نابوغيت، وترى أي نموذج أجاب وكم استغرق وكم كلّف.',
    placeholder: 'اكتب سؤالك…',
    samples: [
      'اكتب رسالة متابعة قصيرة لعميل',
      'اشرح الفرق بين GPT وClaude في ثلاثة أسطر',
      'اكتب دالة Python تحذف العناصر المكررة من قائمة',
    ],
    send: 'إرسال',
    sending: 'جارٍ الرد…',
    triesLeft: ({ n }) => `${fmtInt(n)} محاولات متبقية هذه الساعة`,
    route: 'مسار هذا الطلب',
    served: 'أجاب',
    latency: 'الزمن',
    tokens: 'الرموز',
    cost: 'التكلفة',
    ms: 'ملّي ثانية',
    empty: 'ستظهر الإجابة هنا مع مسارها وزمنها وتكلفتها الحقيقية.',
    codeTitle: 'الطلب نفسه من شيفرتك:',
    getKey: 'احصل على مفتاحك — من ١ دولار',
    console: 'افتح اللوحة',
    errors: {
      demo_rate_limited: 'انتهت محاولات هذه الساعة الخمس. حساب مجاني يكمل دون حدود.',
      demo_budget: 'نفدت ميزانية العرض لليوم وتعود غدًا. حسابك يعمل الآن.',
      demo_busy: 'العرض مشغول الآن؛ أعد الإرسال بعد ثوانٍ.',
      demo_disabled: 'العرض الحي غير مفعّل على هذا الخادم.',
      upstream_failed: 'تعذّر على كل النماذج خلف العرض الرد الآن؛ حاول بعد قليل.',
      bad_prompt: 'يجب أن يكون السؤال بين ١ و٤٠٠ حرف.',
      network: 'تعذّر الاتصال؛ تحقق من الإنترنت وأعد الإرسال.',
    },
  },
};

// reveal shows the answer at reading pace instead of all at once — about the
// speed a streamed answer arrives — unless the visitor asked for less motion.
function useReveal(text) {
  const [shown, setShown] = useState('');
  useEffect(() => {
    if (!text) { setShown(''); return undefined; }
    if (window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) { setShown(text); return undefined; }
    let raf = 0;
    const start = performance.now();
    const perMs = 0.35; // ~350 characters a second
    const step = (now) => {
      const n = Math.min(text.length, Math.ceil((now - start) * perMs));
      setShown(text.slice(0, n));
      if (n < text.length) raf = requestAnimationFrame(step);
    };
    raf = requestAnimationFrame(step);
    return () => cancelAnimationFrame(raf);
  }, [text]);
  return shown;
}

function curlFor(prompt, alias) {
  const body = JSON.stringify({ model: alias, messages: [{ role: 'user', content: prompt }] });
  return `curl ${gatewayBase()}/chat/completions \\\n  -H "Authorization: Bearer $NABUGATE_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '${body.replace(/'/g, "'\\''")}'`;
}

export default function TryLive({ lang, signedIn = false }) {
  const t = useT(T);
  const [prompt, setPrompt] = useState('');
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState(null);
  const [error, setError] = useState('');
  const [asked, setAsked] = useState('');
  const [remaining, setRemaining] = useState(null);
  const input = useRef(null);
  const answer = useReveal(result?.text || '');
  const samples = t('samples');
  const errors = t('errors');

  const ask = async (text) => {
    const q = (text ?? prompt).trim();
    if (!q || busy) return;
    if (text !== undefined) setPrompt(text);
    setBusy(true);
    setError('');
    setResult(null);
    setAsked(q);
    markEngagement('tried');
    try {
      const res = await fetch('/api/public/try', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ prompt: q, lang }),
      });
      const body = await res.json().catch(() => ({}));
      if (!res.ok) {
        const code = body?.error?.code;
        setError(errors[code] || errors.upstream_failed);
        if (code === 'demo_rate_limited' || code === 'demo_budget') setRemaining(0);
        return;
      }
      setResult(body);
      setRemaining(body.remaining);
    } catch {
      setError(errors.network);
    } finally {
      setBusy(false);
    }
  };

  const onKey = (e) => {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); ask(); }
  };
  const outOfTries = remaining === 0;
  const cta = signedIn ? '/panel/' : '/panel/login';

  return (
    <section id="try" className="lp-section lp-try">
      <div className="lp-container">
        <header className="lp-section-head lp-reveal">
          <span className="lp-eyebrow"><Icon name="zap" size={14} /> {t('eyebrow')}</span>
          <h2>{t('title')}</h2>
          <p>{t('sub')}</p>
        </header>

        <div className="try-grid lp-reveal">
          <div className="try-ask">
            <div className="try-input">
              <textarea
                ref={input}
                value={prompt}
                dir="auto"
                rows={4}
                maxLength={MAX_CHARS}
                placeholder={t('placeholder')}
                onChange={(e) => setPrompt(e.target.value)}
                onKeyDown={onKey}
                aria-label={t('placeholder')}
              />
              <div className="try-input-foot">
                <span className="try-count" dir="ltr">{fmtInt(prompt.length)}/{fmtInt(MAX_CHARS)}</span>
                <button type="button" className="lp-btn lp-btn-primary try-send" onClick={() => ask()} disabled={busy || !prompt.trim() || outOfTries}>
                  {busy ? t('sending') : t('send')}
                  {!busy && <Icon name="arrow" size={16} className="flip-rtl" />}
                </button>
              </div>
            </div>
            <div className="try-samples">
              {samples.map((s) => (
                <button key={s} type="button" className="try-chip" onClick={() => ask(s)} disabled={busy || outOfTries}>{s}</button>
              ))}
            </div>
            {remaining !== null && !outOfTries && <p className="try-left">{t('triesLeft', { n: remaining })}</p>}
          </div>

          <div className={'try-out' + (busy ? ' busy' : '')} aria-live="polite">
            {!result && !error && !busy && <p className="try-empty">{t('empty')}</p>}
            {busy && (
              <div className="try-wait" role="status">
                <span /><span /><span />
              </div>
            )}
            {error && (
              <div className="try-error">
                <Icon name="info" size={18} />
                <p>{error}</p>
                {outOfTries && <a href={cta} className="lp-btn lp-btn-primary">{t('getKey')}</a>}
              </div>
            )}
            {result && (
              <>
                <div className="try-route">
                  <span className="try-label">{t('route')}</span>
                  <ol dir="ltr">
                    {(result.chain?.length ? result.chain : [result.model]).map((m) => (
                      <li key={m} className={m === result.model ? 'on' : ''}>
                        {m === result.model && <Icon name="check" size={12} />}
                        <code>{m}</code>
                        {m === result.model && <em>{t('served')} · {result.provider}</em>}
                      </li>
                    ))}
                  </ol>
                </div>
                <dl className="try-stats">
                  <div><dt><Icon name="clock" size={14} />{t('latency')}</dt><dd>{fmtInt(result.latency_ms)} <small>{t('ms')}</small></dd></div>
                  <div><dt><Icon name="activity" size={14} />{t('tokens')}</dt><dd>{fmtInt((result.prompt_tokens || 0) + (result.completion_tokens || 0))}</dd></div>
                  <div><dt><Icon name="wallet" size={14} />{t('cost')}</dt><dd dir="ltr">{usd(result.cost_usd)}</dd></div>
                </dl>
                <div className="try-answer" dir="auto">{answer}{answer.length < (result.text || '').length && <span className="try-caret" />}</div>
                {answer.length === (result.text || '').length && (
                  <div className="try-next">
                    <p>{t('codeTitle')}</p>
                    <CodeBlock code={curlFor(asked, result.alias)} label="bash" />
                    <div className="try-next-cta">
                      <a href={cta} className="lp-btn lp-btn-primary">{signedIn ? t('console') : t('getKey')} <Icon name="arrow" size={16} className="flip-rtl" /></a>
                    </div>
                  </div>
                )}
              </>
            )}
          </div>
        </div>
      </div>
    </section>
  );
}
