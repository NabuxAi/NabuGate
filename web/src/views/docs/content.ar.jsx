import DocsFa from './content.fa.jsx';
import CodeBlock from '../../components/CodeBlock.jsx';
import { H1, H2, Lead } from './parts.jsx';

/*
 * Arabic readers otherwise see the Persian docs. Live voice and the vision
 * alias are written here; every other section stays the Persian page.
 */
export default function DocsAr(props) {
  if (props.active === 'live-voice') {
    const { BASE, KEY } = props;
    return (
      <section>
        <H1>الصوت المباشر</H1>
        <Lead>تُفتح المكالمة هنا وتُحاسب بالدقيقة. إذا كان المرحّل مطفأً فالصوت يذهب إلى المزود. إذا كان مشغّلاً فالمتصفح يتحدث مع البوابة فقط.</Lead>
        <H2>GPT-Live</H2>
        <p><code>POST /v1/live/sessions</code> مع <code dir="ltr">model: &quot;nabu-live&quot;</code> وعرض SDP من المتصفح. يُمرَّر <code>session.tools</code> كما هو. أرسل المدة عبر <code>POST /v1/live/sessions/&#123;id&#125;/usage</code> و<code dir="ltr">&#123;&quot;seconds&quot;: 90, &quot;final&quot;: true&#125;</code>. الثواني لقطة تراكمية. الدقيقة الأولى مدفوعة مقدمًا. يظهر المبلغ في <code>GET /v1/usage</code>.</p>
        <H2>Gemini Live</H2>
        <p><code>POST /v1/live/gemini/token</code> يستخدم مفتاح البوابة نفسه، فيُطبَّق حد المعدل ورصيد الحساب. الجسم <code dir="ltr">&#123;&quot;setup&quot;: &#123;…&#125;&#125;</code>. تُثبَّت <code>setup.tools[].functionDeclarations</code> داخل الرمز المؤقت. الجواب: <code>token</code> و<code>expireTime</code> و<code>url</code> و<code>session_id</code>.</p>
        <CodeBlock code={`curl -X POST ${BASE}/live/gemini/token \\
  -H "Authorization: Bearer ${KEY}" \\
  -H "Content-Type: application/json" \\
  -d '{"setup":{"model":"models/gemini-2.5-flash-native-audio-preview-12-2025","generationConfig":{"responseModalities":["AUDIO"]},"tools":[{"functionDeclarations":[{"name":"search_products","description":"Find products","parameters":{"type":"object","properties":{"q":{"type":"string"}}}}]}]}}'`} />
        <H2>المرحّل</H2>
        <p>فعّل <code>live_relay.enabled</code>. يصبح <code>url</code> عندها <code dir="ltr">wss://&lt;this host&gt;/v1/live/ws?ticket=…</code>. <code>POST /v1/live/ws/session</code> مع <code dir="ltr">&#123;&quot;provider&quot;:&quot;openai&quot;,&quot;model&quot;:&quot;nabu-live&quot;&#125;</code> يفتح المقبس نفسه لـ OpenAI Realtime. تمر إطارات استدعاء الأدوات كما هي. الحد <code>max_minutes</code> (٣٠) و<code>max_sessions_per_key</code> (١٠). يحتاج المرحّل <code>GEMINI_API_KEY</code> و<code>OPENAI_API_KEY</code> على البوابة، ويبقى مطفأً حتى يشغّله المشغّل.</p>
        <H2>قراءة الصور</H2>
        <p>الاسم <code>nabu-vision</code> موجود. السلسلة: <code>gpt-4o</code> أولاً، ثم Gemini 2.5 Flash على واجهة غوغل المتوافقة مع OpenAI (ومشروع غوغل ثانٍ)، ثم Qwen3-VL، ثم Claude. أرسل الصورة كجزء <code>image_url</code> بعنوان <code>data:</code> أو <code>https</code>.</p>
      </section>
    );
  }
  return <DocsFa {...props} />;
}
