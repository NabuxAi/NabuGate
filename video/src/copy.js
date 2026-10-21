// On-screen copy for the landing intro. One object per locale the site
// speaks (fa, en, ar). Facts only: no customer counts, no latency, no prices.
// Code, alias names and header names stay in Latin in every language.

export const FPS = 24;
// Six scenes, 7.5s each. 45s sits inside the 30–60s brief.
export const SCENE_FRAMES = 180;
export const SCENE_COUNT = 6;
export const DURATION = SCENE_FRAMES * SCENE_COUNT;

const modalities = {
  en: ['Chat', 'Embeddings', 'Images', 'Speech', 'Transcription', 'Live voice'],
  fa: ['چت', 'Embedding', 'تصویر', 'گفتار', 'رونویسی', 'صدای زنده'],
  ar: ['دردشة', 'تضمينات', 'صور', 'كلام', 'تفريغ', 'صوت مباشر'],
};

export const COPY = {
  en: {
    dir: 'ltr',
    font: 'Inter',
    brand: 'NabuGate',
    modalities: modalities.en,
    scenes: [
      {
        kicker: 'OpenAI-compatible AI gateway',
        title: 'One endpoint\nfor every project',
        body: 'Projects never call a provider directly. They call NabuGate, with one key.',
        visual: 'endpoint',
      },
      {
        kicker: 'Aliases',
        title: 'Ask for an alias,\nnot a vendor',
        body: 'nabu-fast and nabu-smart are names you keep. The gateway chooses the provider behind them.',
        visual: 'aliases',
      },
      {
        kicker: 'Fallback',
        title: 'If one vendor\ndoesn’t answer',
        body: 'A rate limit, an outage, or an empty reply moves the call to a different provider.',
        visual: 'route',
      },
      {
        kicker: 'Keys and cost',
        title: 'A key and a quota\nfor each project',
        body: 'Allow-lists and rate limits live on the key. Spend is on GET /v1/usage, per project and per model.',
        visual: 'usage',
      },
      {
        kicker: 'Bring your own key',
        title: 'Your key,\nor the gateway’s',
        body: 'Send X-Nabu-Key-<provider>. A call served by your key is metered here and not billed here.',
        visual: 'providers',
      },
      {
        kicker: 'Same key, every modality',
        title: 'More than chat',
        body: 'Images, speech, transcription, embeddings and live voice use the same key and the same base URL.',
        visual: 'modalities',
      },
    ],
  },
  fa: {
    dir: 'rtl',
    font: 'Vazirmatn',
    brand: 'نبوگیت',
    modalities: modalities.fa,
    scenes: [
      {
        kicker: 'دروازهٔ هوش مصنوعی سازگار با OpenAI',
        title: 'یک نقطهٔ ورود\nبرای هر پروژه',
        body: 'پروژه‌ها مستقیم به فروشنده وصل نمی‌شوند. با یک کلید به نبوگیت درخواست می‌دهند.',
        visual: 'endpoint',
      },
      {
        kicker: 'Alias',
        title: 'اسم alias را بخواهید،\nنه اسم فروشنده را',
        body: 'nabu-fast و nabu-smart نام‌هایی‌اند که شما نگه می‌دارید. دروازه فروشندهٔ پشتشان را انتخاب می‌کند.',
        visual: 'aliases',
      },
      {
        kicker: 'Fallback',
        title: 'اگر یک فروشنده\nجواب ندهد',
        body: 'سقف نرخ، قطعی، یا پاسخ خالی، درخواست را به فروشندهٔ دیگری می‌برد.',
        visual: 'route',
      },
      {
        kicker: 'کلید و هزینه',
        title: 'کلید و سهمیهٔ\nهر پروژه',
        body: 'allow-list و سقف نرخ روی همان کلید است. هزینه در GET /v1/usage، به‌ازای پروژه و مدل.',
        visual: 'usage',
      },
      {
        kicker: 'کلید خودتان',
        title: 'کلید شما،\nیا کلید دروازه',
        body: 'هدر X-Nabu-Key-<provider> را بفرستید. درخواستی که با کلید شما سرو شود ثبت می‌شود و اینجا هزینه ندارد.',
        visual: 'providers',
      },
      {
        kicker: 'همان کلید، همهٔ مدالیته‌ها',
        title: 'فقط چت نیست',
        body: 'تصویر، گفتار، رونویسی، embedding و صدای زنده با همان کلید و همان آدرس پایه.',
        visual: 'modalities',
      },
    ],
  },
  ar: {
    dir: 'rtl',
    font: 'Noto Naskh Arabic',
    brand: 'نابوغيت',
    modalities: modalities.ar,
    scenes: [
      {
        kicker: 'بوابة ذكاء اصطناعي متوافقة مع OpenAI',
        title: 'نقطة دخول واحدة\nلكل مشروع',
        body: 'المشاريع لا تتصل بمزوّد مباشرة. تتصل بـ نابوغيت، بمفتاح واحد.',
        visual: 'endpoint',
      },
      {
        kicker: 'الأسماء المستعارة',
        title: 'اطلب الاسم المستعار\nلا اسم المزوّد',
        body: 'nabu-fast و nabu-smart اسمان تبقيهما أنت. البوابة تختار المزوّد خلفهما.',
        visual: 'aliases',
      },
      {
        kicker: 'الاحتياط',
        title: 'إذا لم يجب\nمزوّد',
        body: 'حدّ المعدّل، أو انقطاع، أو رد فارغ ينقل الطلب إلى مزوّد آخر.',
        visual: 'route',
      },
      {
        kicker: 'المفاتيح والتكلفة',
        title: 'مفتاح وحصّة\nلكل مشروع',
        body: 'قائمة السماح وحدّ المعدّل على المفتاح. التكلفة في GET /v1/usage، لكل مشروع ولكل نموذج.',
        visual: 'usage',
      },
      {
        kicker: 'مفتاحك أنت',
        title: 'مفتاحك،\nأو مفتاح البوابة',
        body: 'أرسل الترويسة X-Nabu-Key-<provider>. الطلب الذي يُخدم بمفتاحك يُقاس هنا ولا يُحاسب هنا.',
        visual: 'providers',
      },
      {
        kicker: 'المفتاح نفسه، كل الوسائط',
        title: 'أكثر من الدردشة',
        body: 'الصور والكلام والتفريغ والتضمينات والصوت المباشر بالمفتاح نفسه وعنوان الأساس نفسه.',
        visual: 'modalities',
      },
    ],
  },
};

export const LANGS = Object.keys(COPY);
