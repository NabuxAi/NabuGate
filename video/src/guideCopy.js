// Script for the "Claude Desktop on NabuGate" tutorial. One object per site
// language. `say` is the narration (sent to text-to-speech by tts.mjs), `title`
// and `body` are what the picture shows. Narration never reads an ID or a URL
// aloud: those are on screen, exactly, in Latin script, left to right.
//
// The steps follow Anthropic's own third-party Claude Desktop docs
// (claude.com/docs/third-party/claude-desktop/gateway). Labels in `ui` are the
// app's English menu labels, kept identical in every language.

export const GUIDE_FPS = 30;
export const GUIDE_LANGS = ['fa', 'en', 'ar'];

// Shown on screen, never narrated.
export const UI = {
  menu: ['Help', 'Troubleshooting', 'Enable Developer Mode'],
  dev: ['Developer', 'Configure Third-Party Inference…'],
  fields: [
    ['Inference provider', 'Gateway'],
    ['Gateway base URL', 'https://gate.nabuxai.com'],
    ['Credential kind', 'Static API key'],
    ['Gateway auth scheme', 'Bearer'],
    ['API key', 'ng-••••••••••••••••••'],
  ],
  iranUrl: 'https://gate.nabuxai.ir',
  models: ['claude-sonnet-5-5', 'claude-opus-5-5', 'claude-haiku-5-5'],
  apply: ['Apply Changes', 'Save & Restart'],
};

// `visual` picks the illustration; the order is the order of the steps.
export const GUIDE = {
  fa: {
    dir: 'rtl',
    font: 'Vazirmatn',
    stepWord: 'مرحلهٔ',
    scenes: [
      {
        visual: 'intro',
        kicker: 'آموزش',
        title: 'Claude Desktop با کلید NabuGate',
        body: 'گفت‌وگوها از کیف‌پول NabuGate حساب می‌شوند، نه از حساب Anthropic.',
        say: 'در این آموزش، اپ دسکتاپِ Claude را به نابوگیت وصل می‌کنیم؛ تا هزینهٔ گفت‌وگوها از کیف‌پولِ نابوگیت کم شود، نه از حسابِ Anthropic.',
      },
      {
        visual: 'key',
        kicker: 'مرحلهٔ ۱',
        title: 'یک کلید API بسازید',
        body: 'پنل NabuGate ← کلیدهای API ← کلید جدید. متن کلید فقط یک‌بار نمایش داده می‌شود.',
        say: 'اول وارد پنلِ نابوگیت شوید، به بخشِ کلیدهای API بروید و یک کلیدِ تازه بسازید. متنِ کلید فقط یک بار نشان داده می‌شود؛ همان لحظه کپی‌اش کنید.',
      },
      {
        visual: 'devmode',
        kicker: 'مرحلهٔ ۲',
        title: 'حالت توسعه‌دهنده را روشن کنید',
        body: 'وارد حساب Claude نشوید. ویندوز: منوی ☰ بالا سمت چپ. مک: نوار منو.',
        say: 'حالا Claude Desktop را باز کنید، ولی وارد حساب نشوید. در ویندوز از منوی سه‌خطِ بالا سمتِ چپ، و در مک از نوارِ منو، به Help بروید، بعد Troubleshooting، و گزینهٔ Enable Developer Mode را بزنید.',
      },
      {
        visual: 'developer',
        kicker: 'مرحلهٔ ۳',
        title: 'پنجرهٔ Third-Party Inference',
        body: 'از منوی تازهٔ Developer گزینهٔ Configure Third-Party Inference را باز کنید.',
        say: 'بعد از این کار، منوی تازه‌ای به اسمِ Developer ظاهر می‌شود. از همین منو، پنجرهٔ Configure Third-Party Inference را باز کنید.',
      },
      {
        visual: 'connection',
        kicker: 'مرحلهٔ ۴',
        title: 'اتصال به Gateway',
        body: 'از داخل ایران آدرس پایه را gate.nabuxai.ir بگذارید؛ انتهای آدرس v1 نمی‌آید.',
        say: 'در بخشِ Connection، ارائه‌دهنده را Gateway بگذارید و مقادیرِ روی صفحه را وارد کنید: آدرسِ نابوگیت، نوعِ کلیدِ ثابت، روشِ Bearer، و کلیدی که ساختید. اگر داخلِ ایران هستید، از آدرسِ دامنهٔ ir استفاده کنید.',
      },
      {
        visual: 'models',
        kicker: 'مرحلهٔ ۵',
        title: 'مدل‌ها را اضافه کنید',
        body: 'شناسه‌ها را دقیقاً همین‌طور بنویسید. Claude Desktop فقط نامی را نشان می‌دهد که Claude بشناسد.',
        say: 'در بخشِ Models، سه مدلِ کلود را با همین شناسه‌هایی که روی صفحه می‌بینید اضافه کنید: سانِت، اوپوس و هایکو. شناسه را دقیقاً همین‌طور بنویسید.',
      },
      {
        visual: 'apply',
        kicker: 'مرحلهٔ ۶',
        title: 'ذخیره و راه‌اندازی دوباره',
        body: 'Apply Changes ← Save & Restart. در صفحهٔ ورود، ادامه در حالت third-party را انتخاب کنید.',
        say: 'حالا Apply Changes و بعد Save and Restart را بزنید. وقتی اپ دوباره باز شد، در صفحهٔ ورود، ادامه در حالتِ third-party را انتخاب کنید و گفت‌وگو را شروع کنید.',
      },
      {
        visual: 'usage',
        kicker: 'تمام شد',
        title: 'مصرف را در پنل ببینید',
        body: 'هر پیام با مدل و هزینه‌اش ثبت می‌شود. کلید لو رفته را همان‌جا باطل کنید.',
        say: 'از این به بعد، هر پیام با مدل و هزینه‌اش در پنلِ نابوگیت ثبت می‌شود. اگر کلیدتان جایی لو رفت، همان‌جا باطلش کنید و کلیدِ تازه بسازید.',
      },
    ],
  },
  en: {
    dir: 'ltr',
    font: 'Inter',
    stepWord: 'Step',
    scenes: [
      {
        visual: 'intro',
        kicker: 'Tutorial',
        title: 'Claude Desktop on a NabuGate key',
        body: 'Conversations are billed to your NabuGate wallet, not to an Anthropic account.',
        say: 'In this tutorial we connect the Claude desktop app to NabuGate, so your conversations are billed to your NabuGate wallet instead of an Anthropic account.',
      },
      {
        visual: 'key',
        kicker: 'Step 1',
        title: 'Create an API key',
        body: 'NabuGate panel → API keys → New key. The key text is shown only once.',
        say: 'First, sign in to the NabuGate panel, open API keys, and create a new key. The key is shown only once, so copy it right away.',
      },
      {
        visual: 'devmode',
        kicker: 'Step 2',
        title: 'Turn on Developer Mode',
        body: 'Do not sign in to Claude. Windows: the ☰ menu, top left. macOS: the menu bar.',
        say: 'Now open Claude Desktop, but do not sign in. On Windows use the menu in the top-left corner, on a Mac the menu bar. Go to Help, then Troubleshooting, then Enable Developer Mode.',
      },
      {
        visual: 'developer',
        kicker: 'Step 3',
        title: 'Open Third-Party Inference',
        body: 'From the new Developer menu, open Configure Third-Party Inference.',
        say: 'A new Developer menu appears. From it, open Configure Third-Party Inference.',
      },
      {
        visual: 'connection',
        kicker: 'Step 4',
        title: 'Connect the Gateway',
        body: 'From inside Iran, use gate.nabuxai.ir as the base URL. No /v1 at the end.',
        say: 'Under Connection, set the provider to Gateway and enter the values on screen: the NabuGate address, a static API key, the Bearer scheme, and the key you just created. From inside Iran, use the dot I R address.',
      },
      {
        visual: 'models',
        kicker: 'Step 5',
        title: 'Add the models',
        body: 'Type the IDs exactly. Claude Desktop only lists a name it recognises as Claude.',
        say: 'Under Models, add the three Claude models shown on screen: Sonnet, Opus and Haiku. Type each ID exactly as it appears.',
      },
      {
        visual: 'apply',
        kicker: 'Step 6',
        title: 'Save and restart',
        body: 'Apply Changes → Save & Restart. On the sign-in screen, continue in third-party mode.',
        say: 'Click Apply Changes, then Save and Restart. When the app reopens, choose to continue in third-party mode on the sign-in screen, and start chatting.',
      },
      {
        visual: 'usage',
        kicker: 'Done',
        title: 'Watch usage in the panel',
        body: 'Every message is logged with its model and cost. Revoke a leaked key right there.',
        say: 'From now on, every message shows up in the NabuGate panel with its model and cost. If a key ever leaks, revoke it there and create a new one.',
      },
    ],
  },
  ar: {
    dir: 'rtl',
    font: 'Noto Naskh Arabic',
    stepWord: 'الخطوة',
    scenes: [
      {
        visual: 'intro',
        kicker: 'شرح',
        title: 'Claude Desktop بمفتاح NabuGate',
        body: 'تُحتسب المحادثات من محفظة NabuGate، لا من حساب Anthropic.',
        say: 'في هذا الشرح نربط تطبيق Claude لسطح المكتب بنابوغيت، لتُحتسب تكلفة المحادثات من محفظة نابوغيت بدلاً من حساب Anthropic.',
      },
      {
        visual: 'key',
        kicker: 'الخطوة ١',
        title: 'أنشئ مفتاح API',
        body: 'لوحة NabuGate ← مفاتيح API ← مفتاح جديد. يُعرض نص المفتاح مرة واحدة فقط.',
        say: 'أولاً، سجّل الدخول إلى لوحة نابوغيت، وافتح قسم مفاتيح API، وأنشئ مفتاحاً جديداً. يظهر المفتاح مرة واحدة فقط، فانسخه فوراً.',
      },
      {
        visual: 'devmode',
        kicker: 'الخطوة ٢',
        title: 'فعّل وضع المطوّر',
        body: 'لا تسجّل الدخول إلى Claude. ويندوز: قائمة ☰ أعلى اليسار. ماك: شريط القوائم.',
        say: 'الآن افتح Claude Desktop من دون تسجيل الدخول. في ويندوز استخدم القائمة في الزاوية العلوية اليسرى، وفي ماك شريط القوائم. اذهب إلى Help، ثم Troubleshooting، ثم Enable Developer Mode.',
      },
      {
        visual: 'developer',
        kicker: 'الخطوة ٣',
        title: 'افتح Third-Party Inference',
        body: 'من قائمة Developer الجديدة افتح Configure Third-Party Inference.',
        say: 'تظهر قائمة جديدة باسم Developer. افتح منها نافذة Configure Third-Party Inference.',
      },
      {
        visual: 'connection',
        kicker: 'الخطوة ٤',
        title: 'اربط الـ Gateway',
        body: 'من داخل إيران استخدم gate.nabuxai.ir عنواناً أساسياً، من دون v1 في آخره.',
        say: 'في قسم Connection، اختر Gateway مزوّداً، وأدخل القيم الظاهرة على الشاشة: عنوان نابوغيت، ومفتاحاً ثابتاً، وطريقة Bearer، والمفتاح الذي أنشأته. ومن داخل إيران استخدم عنوان النطاق الإيراني.',
      },
      {
        visual: 'models',
        kicker: 'الخطوة ٥',
        title: 'أضف النماذج',
        body: 'اكتب المعرّفات كما هي تماماً. لا يعرض Claude Desktop إلا اسماً يتعرّف عليه.',
        say: 'في قسم Models، أضف نماذج كلود الثلاثة الظاهرة على الشاشة: سونيت وأوبوس وهايكو. اكتب كل معرّف كما هو تماماً.',
      },
      {
        visual: 'apply',
        kicker: 'الخطوة ٦',
        title: 'احفظ وأعد التشغيل',
        body: 'Apply Changes ← Save & Restart. في شاشة الدخول، تابع في وضع third-party.',
        say: 'اضغط Apply Changes ثم Save and Restart. عندما يُفتح التطبيق مجدداً، اختر المتابعة في وضع third-party من شاشة الدخول، وابدأ المحادثة.',
      },
      {
        visual: 'usage',
        kicker: 'تم',
        title: 'تابع الاستهلاك في اللوحة',
        body: 'تُسجَّل كل رسالة مع نموذجها وتكلفتها. ألغِ أي مفتاح مسرَّب من هناك.',
        say: 'من الآن، تظهر كل رسالة في لوحة نابوغيت مع النموذج والتكلفة. وإذا تسرّب مفتاحك يوماً، ألغه من هناك وأنشئ مفتاحاً جديداً.',
      },
    ],
  },
};
