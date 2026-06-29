# مقایسهٔ کارکرد و فریمورک/قالب مشترک پروژه‌های Nabu

> سندِ تحلیلیِ ۸ مخزن: NabuGate · NabuDesk · NabuVoice · NabuGen · NabuChat · NabuMind · NabuPilot · intership-video-making-ai
> هدف: استخراج «قالب/فریمورک مشترک» و مقایسهٔ کارکرد.

---

## ۱) نقشهٔ اکوسیستم (هر پروژه چه نقشی دارد)

```
                       ┌─────────────────────────────┐
                       │  NabuGate  (Go)             │  ← دروازهٔ مرکزی LLM
                       │  OpenAI-compatible Gateway  │     (alias routing, fallback,
                       │  nabu-fast / nabu-smart ... │      policy, cost tracking)
                       └──────────────┬──────────────┘
                                      │ همهٔ سرویس‌ها از اینجا به مدل‌ها وصل می‌شوند
        ┌─────────────────┬──────────┼───────────┬──────────────────┐
        ▼                 ▼          ▼           ▼                  ▼
   ┌─────────┐      ┌──────────┐ ┌──────────┐ ┌──────────┐   ┌──────────────┐
   │NabuDesk │◄────►│NabuVoice │ │ NabuGen  │ │ NabuChat │   │  NabuPilot   │
   │Laravel  │ هاب  │ صدا/تماس │ │ تولید    │ │ ساخت     │   │ کوپایلوتِ تیم │
   │پنل/CRM  │      │ تلگرام   │ │ محتوا    │ │ ایجنت    │   │ (Mini App)   │
   └─────────┘      └──────────┘ └──────────┘ └──────────┘   └──────────────┘
        ▲
        │ صفحهٔ فرود/فروش
   ┌─────────┐                         ┌────────────────────────────────┐
   │NabuMind │ (Caddy static)          │ intership-video-making-ai      │
   │ Landing │                         │ بوتِ کارآموزیِ ساخت ویدیو (مجزا)│
   └─────────┘                         └────────────────────────────────┘
```

- **NabuGate** = لایهٔ زیرساخت. تنها دروازهٔ سازگار با OpenAI؛ بقیه به‌جای تماس مستقیم با OpenAI/Anthropic/Gemini با aliasهای `nabu-*` به آن وصل می‌شوند (fallback + سهمیه + هزینه).
- **NabuDesk** = «صفحهٔ کنترل» و هابِ کسب‌وکار (multi-tenant: پیام‌رسان‌ها، کیف‌پول/USDT، KYC، دانش‌نامه). سرویس‌های دیگر با آن یکپارچه می‌شوند (مثلاً NabuVoice پرسونا را از آن می‌خواند).
- **NabuVoice / NabuGen / NabuChat / NabuPilot** = سرویس‌های ماهواره‌ای (هرکدام یک کانال/قابلیتِ AI).
- **NabuMind** = ویترین/لندینگِ محصولِ «مغز تیمی» on-prem.
- **intership-video-making-ai** = پروژهٔ مجزا (مالکِ متفاوت `shm379`) ولی هم‌خانوادهٔ کانونشن‌ها؛ بالغ‌ترین سیستمِ اتوماسیون را دارد.

---

## ۲) جدول مقایسهٔ کارکرد

| پروژه | کارکرد اصلی | زبان/فریمورک | داده | استقرار | نقش در اکوسیستم |
|---|---|---|---|---|---|
| **NabuGate** | دروازهٔ متمرکز LLM سازگار با OpenAI (روتینگ alias، fallback چندلایه، policy/rate-limit، رصد هزینه) | Go 1.24، stdlib + yaml | in-memory usage | Docker (distroless) + Compose | زیرساخت/پلتفرم |
| **NabuDesk** | پنل کسب‌وکارِ چندمستأجری: پیام‌رسان (تلگرام/واتساپ)، کیف‌پول USDT-TRON، KYC، دانش‌نامه، پاسخ AI | Laravel 12 / PHP 8.2+، Livewire/Volt، Alpine، Tailwind/DaisyUI | MySQL/MariaDB، Qdrant (اختیاری) | Docker + Coolify (Traefik) | هاب/Control-plane |
| **NabuVoice** | دستیار صوتیِ تماسِ تلگرام (STT→LLM→TTS) فارسی روی یوزربات MTProto | Python async، telethon، pytgcalls، faster-whisper، edge-tts | MySQL (از NabuDesk) | Docker + Compose + Coolify؛ systemd | سرویس ماهواره‌ای (صدا) |
| **NabuGen** | تولید و زمان‌بندی خودکارِ محتوای کانال (کپشن + تصویر) برای تلگرام/بله | Python FastAPI + React/Vite؛ Directus CMS | PostgreSQL+pgvector، Redis | Docker Compose (۷ سرویس) + Coolify | سرویس ماهواره‌ای (محتوا) |
| **NabuChat** | پلتفرم no-code ساختِ ایجنت LLM + RAG/datastore + ویجت چت (فورک Chaindesk، ری‌برند) | TS/Next.js مونوریپو (pnpm+Turbo)، Prisma | PostgreSQL، Qdrant، Redis، MinIO/S3 | Docker/Coolify + Fly.io | سرویس ماهواره‌ای (ایجنت/RAG) |
| **NabuMind** | لندینگ‌پیجِ سینماییِ محصولِ «مغز تیمی» on-prem | HTML/CSS/JS تک‌فایل + GSAP/Lenis | — | Caddy (Docker) + Coolify | ویترین/بازاریابی |
| **NabuPilot** | کوپایلوتِ عملیاتِ تیمِ پشتیبانی: توزیع روزانهٔ تسک با AI، داشبورد، آنبوردینگ | Telegram Mini App (vanilla JS) + n8n | Telegram CloudStorage | استاتیک (GH Pages/Vercel) + n8n | سرویس ماهواره‌ای (پروتوتایپ) |
| **intership-video-making-ai** | بوتِ مدیریتِ دورهٔ کارآموزیِ ۴‌هفته‌ایِ ساخت ویدیو با AI (تلگرام+واتساپ)، پنل ادمین، مدرک، اکسل | Python 3.12، python-telegram-bot، aiohttp، Pillow/cairosvg، Anthropic | SQLite | Docker + Compose + Coolify | آموزشی/مجزا |

---

## ۳) فریمورک/قالبِ مشترک (الگوهای تکرارشونده)

این هفت کانونشن در عمل «فریمورک نانوشتهٔ Nabu» هستند:

### ۳٫۱ برندینگ و زبان
- خانوادهٔ نام `Nabu*` و دامنه‌های زیرِ `nabuxai.com`.
- **فارسی-اول**: متنِ کاربر، کامنت، کامیت، Issue/PR و مستندات فارسی؛ RTL.

### ۳٫۲ یکپارچگیِ AI
- همه‌جا **سازگار با OpenAI** (متغیّرِ `OPENAI_BASE_URL`/`OpenRouter`).
- مدلِ ایده‌آل: عبور از **NabuGate** با aliasهای `nabu-fast | nabu-smart | nabu-cheap | nabu-vision`.
- **Anthropic Claude** برای اتوماسیونِ مخزن (و دستیارِ داخلِ بوتِ کارآموزی).

### ۳٫۳ پیکربندی
- هر مخزن یک **`.env.example`** مستند دارد؛ `.env` در gitignore. (تنها استثناء: NabuPilot که نمونه‌فایل ندارد.)

### ۳٫۴ استقرار
- **Docker چندمرحله‌ای + docker-compose**، طراحی‌شده برای **Coolify**: شبکهٔ خارجیِ `coolify`، لیبل‌های **Traefik**، HTTPS با Let's Encrypt، healthcheck، `restart: unless-stopped`.

### ۳٫۵ اتوماسیونِ مخزن با Claude (قوی‌ترین وجهِ مشترک)
GitHub Actions که `anthropics/claude-code-action@v1` را روی cron روزانه اجرا می‌کند:
- **شکارِ باگ**: hunt → Issue → fix → PR (نام‌ها: `daily-bug-hunt` / `daily-bug-check` / `claude-daily-bug-scan`).
- **بهبودِ روزانه**: انتخاب از backlog → PR → merge اگر سبز (`daily-auto-improve` / `daily-maintenance`).
- جانبی: `setup-labels` و پاسخ‌دهِ تعاملیِ `@claude`.
- پیش‌نیاز: سکرتِ `ANTHROPIC_API_KEY` + مجوزِ نوشتنِ Actions.
- نام‌گذاریِ شاخه: `claude/auto/<slug>`، `claude/bugfix/<slug>`، `bot/auto-…`، `fix/issue-<n>`.
- قواعد: دیفِ کوچک و کم‌ریسک، PR به‌صورت draft، عبور از تست‌ها قبل از merge.

### ۳٫۶ راهنمای پروژه برای Claude
- فایلِ فارسیِ **`CLAUDE.md`** (هست در NabuDesk، NabuVoice، intership).

### ۳٫۷ تست به‌عنوان دروازهٔ merge
- جایی که تست هست merge را gate می‌کند (PHPUnit / pytest / Jest/Vitest).

---

## ۴) ماتریسِ یکدستی (کدام مخزن چه چیزی از قالب را دارد)

| عنصرِ قالب | NabuGate | NabuDesk | NabuVoice | NabuGen | NabuChat | NabuMind | NabuPilot | intern-bot |
|---|:--:|:--:|:--:|:--:|:--:|:--:|:--:|:--:|
| `.env.example` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ |
| Dockerfile | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ |
| docker-compose | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ |
| Coolify/Traefik | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ |
| `CLAUDE.md` | ❌ | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ✅ |
| CI ساخت/تست | ❌ | ◐ | ◐ | ✅ | ✅ | ❌ | ❌ | ✅ |
| daily-bug-hunt | ❌ | ✅ | ✅ | ◐ | ◐ | ❌ | ❌ | ✅ |
| daily-auto-improve | ❌ | ❌ | ❌ | ✅ | ✅ | ❌ | ❌ | ✅ |
| پاسخ‌دهِ `@claude` | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| تست‌ها | ❌ | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ✅ |
| setup-labels | ❌ | ✅ | ◐ | ◐ | ◐ | ❌ | ❌ | ◐ |

(◐ = به‌شکلِ ادغام‌شده/جزئی موجود است؛ مثلاً NabuGen/NabuChat شکارِ باگ را داخلِ `daily-maintenance` دارند.)

**جمع‌بندیِ بلوغ:**
- بالغ‌ترین از نظرِ قالب: **intership-bot** و **NabuChat/NabuGen** (CI + اتوماسیونِ کامل + تست).
- میانه: **NabuDesk**، **NabuVoice**.
- ناقص: **NabuGate** (بدونِ CI/تست/CLAUDE.md)، **NabuMind** (فقط استقرار)، **NabuPilot** (پروتوتایپ، بدونِ Docker/CI/تست).

---

## ۵) قالبِ کانونیکالِ پیشنهادی (`nabu-service-template`)

ساختاری که هر مخزنِ `Nabu*` جدید باید با آن شروع شود تا یکدست شوند:

```
.
├── CLAUDE.md                      # راهنمای فارسی: استک، کانونشن، نکات امنیتی، دستورات
├── README.md
├── .env.example                   # تمام تنظیمات، مستند و دسته‌بندی‌شده
├── .gitignore  /  .dockerignore
├── Dockerfile                     # چندمرحله‌ای، non-root، HEALTHCHECK
├── docker-compose.yml             # شبکهٔ coolify + لیبل Traefik + healthcheck
├── .github/
│   ├── workflows/
│   │   ├── ci.yml                 # ساخت + تست (gate برای merge)
│   │   ├── daily-bug-hunt.yml     # claude-code-action: شکارِ باگ → Issue → fix → PR
│   │   ├── daily-auto-improve.yml # claude-code-action: backlog → PR → merge اگر سبز
│   │   ├── claude.yml             # پاسخ‌دهِ تعاملیِ @claude (اختیاری)
│   │   └── setup-labels.yml       # لیبل‌های استاندارد
│   ├── CLAUDE_AUTOMATION.md       # راه‌اندازی: ANTHROPIC_API_KEY، مجوزها، لیبل‌ها
│   └── maintenance-backlog.md     # (یا docs/BACKLOG.md)
├── src/  |  app/  |  bot/         # کد
└── tests/                         # تست + dataِ نمونه
```

**لیبل‌های استانداردِ مشترک:**
`bug` · `auto-detected` · `auto-fixed` · `auto-fix` · `severity:critical|high|medium|low` · `daily-report` · `security` · `performance` · `code-quality`

**کانونشنِ Git:** شاخهٔ `claude/auto/<slug>` و `claude/bugfix/<slug>`؛ PR با `Closes #<n>`؛ بدونِ force-push روی main؛ بدونِ نوشتنِ سکرت/شناسهٔ مدل در کد و کامیت.

---

## ۶) توصیه‌ها برای یکدست‌سازی

1. **NabuGate**: افزودنِ `ci.yml` (go vet/test/build) + `daily-bug-hunt` + `CLAUDE.md`؛ نوشتنِ تستِ واحد برای router/policy/usage.
2. **NabuPilot**: افزودنِ `.env.example`, `Dockerfile`, `docker-compose.yml` (Coolify) و حداقل یک CI؛ کامیتِ `bot/workflow.json`.
3. **NabuMind**: افزودنِ یک workflowِ سادهٔ lint/link-check؛ اختیاری `daily-bug-hunt` سبک.
4. همه: یکسان‌سازیِ نامِ workflowها (`daily-bug-hunt.yml` + `daily-auto-improve.yml`) و افزودنِ `CLAUDE.md` در مخازنی که ندارند (NabuGate/NabuGen/NabuChat/NabuMind/NabuPilot).
5. تثبیتِ NabuGate به‌عنوانِ تنها مسیرِ دسترسی به مدل‌ها در بقیهٔ سرویس‌ها (حذف کلیدهای مستقیمِ provider از سرویس‌های ماهواره‌ای).
