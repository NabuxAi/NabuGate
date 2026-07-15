# NabuGate — راهنمای Claude

## پروژه چیست
NabuGate **دروازهٔ مرکزیِ هوش مصنوعیِ** سازمان است: یک نقطهٔ ورودِ سازگار با OpenAI
که همهٔ پروژه‌ها از آن استفاده می‌کنند. پروژه‌ها هرگز مستقیم به OpenAI/Anthropic/
Gemini/Groq/OpenRouter وصل نمی‌شوند؛ با یک **alias** مثل `nabu-fast` به NabuGate
درخواست می‌دهند و دروازه انتخابِ provider، fallback، نگه‌داری سکرت، سهمیه و رصدِ
هزینه را انجام می‌دهد.

## فناوری
- **زبان:** Go 1.24 (فقط stdlib + `gopkg.in/yaml.v3`).
- **اجرا:** باینریِ استاتیک، ایمیجِ distroless، پورت `8080`.

## ساختار
```
cmd/gateway/main.go        # نقطهٔ شروع: load config → adapters → router → server
internal/server/           # HTTP سازگار با OpenAI (auth/policy/usage)
internal/router/           # alias → target، fallback چندلایه
internal/provider/         # آداپتورها: openai (+groq/openrouter)، anthropic، gemini
internal/config/           # خواندن config.yaml و ساختِ آداپتورها
internal/policy/           # کلیدِ پروژه‌ای: allow-list + rate-limit
internal/usage/            # شمارشِ توکن و هزینه per-project/per-model
internal/agent/            # ساب‌ایجنت‌ها: system prompt + پارامترهای پیش‌فرض روی یک alias
agents/                    # تعریفِ ساب‌ایجنت‌ها به‌صورتِ فایلِ YAML (بارگذاری از بیرون)
config.example.yaml        # نمونهٔ پیکربندی (alias‌ها، providerها، pricing، agents)
```

## قراردادِ مهم
- API باید **سازگار با OpenAI-wire** بماند (`/v1/chat/completions`، `/v1/responses`،
  `/v1/embeddings`، `/v1/images/generations`، `/v1/audio/speech`، `/v1/models`،
  `/v1/usage`).
- providerهای چندمدلی (مثل Parspack) را با `passthrough: true` علامت بزن: مدل‌هایشان
  به‌صورتِ `"<provider>/<model>"` مستقیم route می‌شوند (بدون alias) و کاتالوگشان از
  `/v1/models` خودِ provider به‌صورت زنده کشف و در `/v1/models` دروازه نمایش داده
  می‌شود. جداکننده اولین `/` است تا شناسه‌های تودرتو (`openai/gpt-5.5`) سالم بمانند.
- در چت، بدنهٔ درخواست به‌صورتِ **passthroughِ شفاف** به provider منتقل می‌شود؛ فقط
  `model` (به مدلِ upstream) و پرچم‌های stream بازنویسی می‌شوند. یعنی `tools`,
  `tool_choice`, `response_format`, `top_p`, `stop`, `seed`, penalties و … خودکار رد
  می‌شوند و `tool_calls` در پاسخ برمی‌گردد. این رفتار را نشکن.
- آداپتورهای Anthropic/Gemini سازگارِ wire نیستند؛ پارامترهای typed (temperature,
  top_p, max_tokens, stop) به فرمتِ بومی نگاشت می‌شوند.
- سکرت‌ها فقط از env خوانده می‌شوند؛ هرگز در کد/کانفیگِ ایمیج نوشته نشوند.
- providerی که env کلیدش خالی باشد خودکار رد می‌شود تا دروازه با زیرمجموعه‌ای از
  providerها هم بالا بیاید.

## ساب‌ایجنت‌ها (agents)
- **ساب‌ایجنت** = یک دستیارِ نام‌دار: یک **system prompt + پارامترهای پیش‌فرض** که
  روی یک alias‌ِ موجود سوار می‌شود. کاملاً با **config تعریف می‌شود، بدونِ کد**؛ یا
  inline زیرِ `agents:` یا — برای «تعریف از بیرون» — هر ایجنت در یک فایلِ YAML
  داخلِ پوشهٔ `agents_dir`.
- ایجنت مثلِ یک `model` صدا زده می‌شود (`POST /v1/chat/completions` با
  `model: "cine-motion-designer"`)، پس هر کلاینتِ سازگارِ OpenAI با **یک درخواست**
  آن را اجرا می‌کند و از همان زنجیرهٔ fallbackِ router استفاده می‌شود.
- دروازه system prompt را جلوی پیام‌ها می‌گذارد، پارامترهایی که کاربر نداده را با
  پیش‌فرضِ ایجنت پر می‌کند (مقدارِ صریحِ کاربر همیشه برنده است)، به `model`ِ زیرین
  route می‌کند و نامِ ایجنت را در پاسخ و هدرِ `X-Nabu-Agent` برمی‌گرداند. ایجنت‌ها در
  `/v1/models` هم فهرست می‌شوند و با allow-listِ کلید (مثلِ گلابِ `cine-*`) کنترل
  می‌شوند. این رفتارِ سازگاریِ OpenAI را نشکن.
- پوشهٔ `agents/` گروهِ **Cinematic Scrollytelling** را دارد: هفت متخصص برای ساختِ
  صفحاتِ سینماییِ اسکرول‌محور (کارگردانِ خلاق، طراحِ تعاملی، موشن، سه‌بعدی،
  فرانت‌اند، محتوا، و مهندسِ کارایی/دسترس‌پذیری).

## دستورات
```bash
go build ./...            # ساخت
go vet ./...              # بررسی ایستا
go test ./...             # تست‌ها
go run ./cmd/gateway -config config.yaml   # اجرای محلی
```

## افزودنِ provider/alias
معمولاً بدونِ تغییرِ کد و فقط با ویرایشِ `config.yaml` (یا `config.example.yaml`)
انجام می‌شود: یک provider با `type` و `api_key_env` اضافه کن و alias را زیرِ
`models`/`images`/`audio`/`embeddings` به آن نگاشت کن. آداپتورِ جدید فقط وقتی لازم
است که provider سازگارِ wire با OpenAI/Anthropic/Gemini نباشد.

## سبک کد و PR
- دیفِ کوچک و کم‌ریسک؛ رفتارِ سازگاریِ OpenAI را نشکن.
- برای هر تغییرِ رفتاری، تست اضافه/به‌روزرسانی کن (`httptest` برای آداپتورها).
- پیامِ کامیت/PR/کد بدونِ شناسهٔ مدل یا اطلاعاتِ داخلیِ ابزار.
