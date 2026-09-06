// Package vendors is the catalogue of upstream AI providers: the ones this
// deployment already routes to, and the ones it does not yet but a user might
// ask for.
//
// It exists because three separate things need the same facts and were
// otherwise going to invent them three times — the console needs a name, a
// logo and a link to where you get a key; the access-request flow needs to
// know a provider is real before someone can request it; and the config needs
// none of that, only a base URL and a key env.
//
// So the catalogue lives here rather than in config.yaml. A provider in the
// config with no entry here still works — it renders as a lettermark with its
// own name. Adding a provider never requires touching the frontend.
package vendors

import "strings"

// Vendor is one upstream, described for a human rather than for the router.
type Vendor struct {
	// Name is the key the config uses, and the one an X-Nabu-Key-<name> header
	// is matched against.
	Name string `json:"name"`
	// Label is what a person calls it.
	Label string `json:"label"`
	// Blurb is one line: what it is good at, in the words a customer would use.
	Blurb string `json:"blurb"`
	// Capabilities are the coarse kinds of work it does. Not derived from the
	// adapter interface, because a provider can implement an interface and still
	// not sell that capability.
	Capabilities []string `json:"capabilities"`
	// Site is the vendor's home; KeysURL is the page where a key is issued —
	// the single most useful link on a bring-your-own-key screen.
	Site    string `json:"site"`
	KeysURL string `json:"keys_url"`
	// Icon is an SVG path drawn in a 24x24 box, and Color the brand colour it
	// is drawn in. Both empty means the console draws a lettermark instead;
	// that is a supported state, not a missing asset.
	Icon  string `json:"icon,omitempty"`
	Color string `json:"color,omitempty"`
	// Iran is true for providers reachable from Iran without a detour, which is
	// the first question a user here asks about any of them.
	Iran bool `json:"iran,omitempty"`
}

const (
	CapChat       = "chat"
	CapImage      = "image"
	CapSpeech     = "speech"
	CapTranscribe = "transcription"
	CapEmbed      = "embedding"
	CapVideo      = "video"
	CapDocs       = "documents"
	CapPhotos     = "photos"
)

// catalogue is keyed by the config's provider name. Icon paths are the vendors'
// own marks, drawn on a 24x24 viewBox.
var catalogue = map[string]Vendor{
	"openai": {
		Label: "OpenAI", Blurb: "GPT، Whisper و DALL·E — مرجعِ همان wire format که این دروازه صحبت می‌کند.",
		Capabilities: []string{CapChat, CapImage, CapSpeech, CapTranscribe, CapEmbed},
		Site:         "https://openai.com", KeysURL: "https://platform.openai.com/api-keys",
		Color: "#000000",
		Icon:  "M22.28 9.82a5.99 5.99 0 0 0-.52-4.91 6.05 6.05 0 0 0-6.51-2.9A6.07 6.07 0 0 0 4.98 4.18a6 6 0 0 0-4 2.9 6.05 6.05 0 0 0 .74 7.1 5.98 5.98 0 0 0 .51 4.91 6.05 6.05 0 0 0 6.52 2.9A5.99 5.99 0 0 0 13.26 24a6.06 6.06 0 0 0 5.77-4.21 6 6 0 0 0 4-2.9 6.06 6.06 0 0 0-.75-7.07Z",
	},
	"anthropic": {
		Label: "Anthropic", Blurb: "کلود — دنبال‌کردنِ دستور و کارِ طولانی، بهترین در متنِ بلند.",
		Capabilities: []string{CapChat},
		Site:         "https://anthropic.com", KeysURL: "https://console.anthropic.com/settings/keys",
		Color: "#D97757",
		Icon:  "M17.3 3.7h-3.2l5.8 16.6h3.2L17.3 3.7ZM6.7 3.7 .9 20.3h3.3l1.2-3.5h6.1l1.2 3.5h3.3L10.2 3.7H6.7Zm-.4 10.2 2-5.8 2 5.8H6.3Z",
	},
	"gemini": {
		Label: "Google Gemini", Blurb: "چندرسانه‌ای در همه‌چیز؛ مدل‌های transcribe‌اش خطای متن را هم اصلاح می‌کنند.",
		Capabilities: []string{CapChat, CapImage, CapSpeech, CapTranscribe, CapEmbed},
		Site:         "https://ai.google.dev", KeysURL: "https://aistudio.google.com/apikey",
		Color: "#4285F4",
		Icon:  "M12 24A14.3 14.3 0 0 0 0 12 14.3 14.3 0 0 0 12 0a14.3 14.3 0 0 0 12 12 14.3 14.3 0 0 0-12 12Z",
	},
	"groq": {
		Label: "Groq", Blurb: "همان مدل‌های باز، چند برابر سریع‌تر. ارزان‌ترین whisper-large-v3-turbo.",
		Capabilities: []string{CapChat, CapTranscribe},
		Site:         "https://groq.com", KeysURL: "https://console.groq.com/keys",
		Color: "#F55036",
	},
	"openrouter": {
		Label: "OpenRouter", Blurb: "یک کلید، صدها مدل از ده‌ها فروشنده. برای وقتی نمی‌خواهی حساب جدا باز کنی.",
		Capabilities: []string{CapChat},
		Site:         "https://openrouter.ai", KeysURL: "https://openrouter.ai/keys",
		Color: "#6467F2",
	},
	"openrouter2": {
		Label: "OpenRouter (کلید دوم)", Blurb: "همان OpenRouter روی حسابِ دوم — سهمیه‌اش جداست.",
		Capabilities: []string{CapChat},
		Site:         "https://openrouter.ai", KeysURL: "https://openrouter.ai/keys",
		Color: "#6467F2",
	},
	"mistral": {
		Label: "Mistral", Blurb: "مدل‌های اروپایی، و Voxtral که دقیق‌ترین رونویسیِ ارزان بازار است.",
		Capabilities: []string{CapChat, CapTranscribe, CapEmbed},
		Site:         "https://mistral.ai", KeysURL: "https://console.mistral.ai/api-keys",
		Color: "#FA520F",
	},
	"cohere": {
		Label: "Cohere", Blurb: "امبدینگ و rerank در سطح تولید؛ تیرِ رایگانِ توسعه‌دهنده دارد.",
		Capabilities: []string{CapChat, CapEmbed},
		Site:         "https://cohere.com", KeysURL: "https://dashboard.cohere.com/api-keys",
		Color: "#39594D",
	},
	"together": {
		Label: "Together AI", Blurb: "مدل‌های بازِ میزبانی‌شده، و whisper-large-v3 روی سخت‌افزارِ خودشان.",
		Capabilities: []string{CapChat, CapTranscribe, CapImage, CapEmbed},
		Site:         "https://together.ai", KeysURL: "https://api.together.xyz/settings/api-keys",
		Color: "#0F6FFF",
	},
	"cerebras": {
		Label: "Cerebras", Blurb: "سریع‌ترین inference موجود روی مدل‌های باز.",
		Capabilities: []string{CapChat},
		Site:         "https://cerebras.ai", KeysURL: "https://cloud.cerebras.ai",
		Color: "#F15A29",
	},
	"nvidia": {
		Label: "NVIDIA NIM", Blurb: "بیش از ۱۰۰ مدلِ میزبانی‌شده، تیرِ رایگانِ سخاوتمند.",
		Capabilities: []string{CapChat, CapEmbed},
		Site:         "https://build.nvidia.com", KeysURL: "https://build.nvidia.com/settings/api-keys",
		Color: "#76B900",
		Icon:  "M8.9 8.6v-1.8c.2 0 .4 0 .5-.1 4.9-.3 8.1 4 8.1 4s-3.5 4.8-7.2 4.8c-.5 0-1-.1-1.4-.2V9.7c1.9.2 2.3 1.1 3.4 3l2.6-2.1s-1.9-2.5-5-2.5c-.3 0-.7 0-1 .1Z",
	},
	"github": {
		Label: "GitHub Models", Blurb: "مدل‌های چند فروشنده با همان توکنِ گیت‌هاب. برای آزمایش رایگان.",
		Capabilities: []string{CapChat, CapEmbed},
		Site:         "https://github.com/marketplace/models", KeysURL: "https://github.com/settings/tokens",
		Color: "#181717",
		Icon:  "M12 .3a12 12 0 0 0-3.8 23.4c.6.1.8-.3.8-.6v-2c-3.3.7-4-1.6-4-1.6-.6-1.4-1.4-1.8-1.4-1.8-1-.7.1-.7.1-.7 1.2.1 1.8 1.2 1.8 1.2 1 1.8 2.8 1.3 3.5 1 0-.8.4-1.3.7-1.6-2.7-.3-5.5-1.3-5.5-5.9 0-1.3.5-2.4 1.2-3.2 0-.4-.5-1.6.2-3.2 0 0 1-.3 3.3 1.2a11.5 11.5 0 0 1 6 0c2.3-1.5 3.3-1.2 3.3-1.2.7 1.6.2 2.8.1 3.2.8.8 1.2 1.9 1.2 3.2 0 4.6-2.8 5.6-5.5 5.9.5.4.9 1.1.9 2.2v3.3c0 .3.2.7.8.6A12 12 0 0 0 12 .3Z",
	},
	"cloudflare": {
		Label: "Cloudflare Workers AI", Blurb: "مدل روی لبه، نزدیک کاربر. سهمیهٔ رایگانِ روزانه دارد.",
		Capabilities: []string{CapChat, CapImage, CapEmbed},
		Site:         "https://developers.cloudflare.com/workers-ai", KeysURL: "https://dash.cloudflare.com/profile/api-tokens",
		Color: "#F38020",
		Icon:  "M16.5 16.3c.2-.5.1-1-.1-1.4-.2-.3-.6-.5-1-.6l-7.7-.1a.15.15 0 0 1-.1-.1v-.2l.2-.1 7.8-.1c.9 0 1.9-.8 2.2-1.7l.5-1.2a10.7 10.7 0 0 0-20.1 1.4c.5-.4 1.2-.6 1.9-.5l4.6.6.2.1v.2l-.2.1-4.5.6C-.5 13.6-1 15-.7 16.3h17.2Z",
	},
	"elevenlabs": {
		Label: "ElevenLabs", Blurb: "طبیعی‌ترین صدای مصنوعی، و Scribe که روی زبان‌های کم‌منبع بهتر از whisper است.",
		Capabilities: []string{CapSpeech, CapTranscribe},
		Site:         "https://elevenlabs.io", KeysURL: "https://elevenlabs.io/app/settings/api-keys",
		Color: "#000000",
	},
	"speechmatics": {
		Label: "Speechmatics", Blurb: "قوی‌ترین تبدیل گفتار به متن روی لهجه و زبانِ درهم‌آمیخته.",
		Capabilities: []string{CapTranscribe},
		Site:         "https://speechmatics.com", KeysURL: "https://portal.speechmatics.com",
		Color: "#1B1B1B",
	},
	"pexels": {
		Label: "Pexels", Blurb: "عکسِ استوکِ رایگان — جای تصویرِ تولیدی وقتی عکسِ واقعی می‌خواهی.",
		Capabilities: []string{CapPhotos},
		Site:         "https://pexels.com", KeysURL: "https://www.pexels.com/api/new",
		Color: "#05A081",
	},
	"gamma": {
		Label: "Gamma", Blurb: "اسلاید، سند و پستِ شبکه‌های اجتماعی — خروجی یک لینکِ میزبانی‌شده است.",
		Capabilities: []string{CapDocs},
		Site:         "https://gamma.app", KeysURL: "https://gamma.app/settings/api",
		Color: "#8B5CF6",
	},
	"parspack": {
		Label: "پارس‌پک", Blurb: "میزبانِ ایرانی با مدل‌های OpenAI و متن‌باز. ریالی، بدون تحریم.",
		Capabilities: []string{CapChat, CapTranscribe, CapEmbed}, Iran: true,
		Site: "https://parspack.com", KeysURL: "https://console.parspack.com",
		Color: "#0F62FE",
	},
	"avalai": {
		Label: "AvalAI", Blurb: "دسترسیِ ایرانی به مدل‌های جهانی با پرداخت ریالی.",
		Capabilities: []string{CapChat, CapImage, CapTranscribe, CapEmbed}, Iran: true,
		Site: "https://avalai.ir", KeysURL: "https://avalai.ir/panel",
		Color: "#00A4A6",
	},
	"gapgpt": {
		Label: "GapGPT", Blurb: "درگاهِ ایرانیِ چندمدلی، سازگار با OpenAI.",
		Capabilities: []string{CapChat, CapImage, CapTranscribe}, Iran: true,
		Site: "https://gapgpt.app", KeysURL: "https://gapgpt.app/panel",
		Color: "#7C3AED",
	},
	"arvan": {
		Label: "ابر آروان", Blurb: "AIaaS آروان. هدرِ احرازش Bearer نیست، apikey است.",
		Capabilities: []string{CapChat, CapEmbed}, Iran: true,
		Site: "https://arvancloud.ir/ai", KeysURL: "https://panel.arvancloud.ir",
		Color: "#F04E37",
	},
	"dahl": {
		Label: "دال", Blurb: "درگاهِ ایرانیِ مدل‌های زبانی.",
		Capabilities: []string{CapChat}, Iran: true,
		Site: "https://dahl.ai", Color: "#111827",
	},
	"9router": {
		Label: "9Router", Blurb: "روترِ چندفروشنده‌ای که خودمان اداره می‌کنیم.",
		Capabilities: []string{CapChat}, Iran: true,
		Color: "#0EA5E9",
	},
	"tokenrouter": {
		Label: "TokenRouter", Blurb: "بیش از ۳۰۰ مدل با مسیریابیِ auto:*؛ مدل‌ها را با : جدا می‌کند نه /.",
		Capabilities: []string{CapChat},
		Site:         "https://tokenrouter.io", Color: "#0891B2",
	},
	"siliconflow": {
		Label: "SiliconFlow", Blurb: "مدل‌های بازِ چینی، ارزان و سریع.",
		Capabilities: []string{CapChat, CapEmbed, CapImage},
		Site:         "https://siliconflow.cn", KeysURL: "https://cloud.siliconflow.cn/account/ak",
		Color: "#7C3AED",
	},
	"whisper":  {Label: "Whisper (خودمان)", Blurb: "موتورِ رونویسیِ روی سرورِ خودمان. رایگان، بی‌کلید.", Capabilities: []string{CapTranscribe}, Color: "#059669"},
	"llamacpp": {Label: "llama.cpp (خودمان)", Blurb: "مدلِ محلی روی سرورِ خودمان. رایگان، بی‌کلید.", Capabilities: []string{CapChat}, Color: "#059669"},
	"infinity": {Label: "Infinity (خودمان)", Blurb: "امبدینگِ محلی. رایگان، بی‌کلید.", Capabilities: []string{CapEmbed}, Color: "#059669"},
	"ollama":   {Label: "Ollama (خودمان)", Blurb: "مدلِ محلی. رایگان، بی‌کلید.", Capabilities: []string{CapChat}, Color: "#059669"},
	"nabuocr":  {Label: "NabuOCR (خودمان)", Blurb: "OCR محلی روی Tesseract.", Capabilities: []string{CapDocs}, Color: "#059669"},
	"imagegen": {Label: "MRC ImageGen", Blurb: "گرافیکِ برندشده — رندرِ قالب است، نه مدلِ diffusion.", Capabilities: []string{CapImage}, Color: "#DB2777"},

	// Not wired here yet. They are listed so a user can ask for them, and so
	// the answer to "چرا فلان جا نیست؟" is on the same screen as everything
	// else rather than in someone's head.
	"deepgram": {
		Label: "Deepgram", Blurb: "رونویسیِ بلادرنگ با تأخیرِ زیرِ ثانیه. هنوز وصل نشده.",
		Capabilities: []string{CapTranscribe},
		Site:         "https://deepgram.com", KeysURL: "https://console.deepgram.com",
		Color: "#13EF93",
	},
	"assemblyai": {
		Label: "AssemblyAI", Blurb: "رونویسی به‌همراه خلاصه و استخراجِ موضوع. هنوز وصل نشده.",
		Capabilities: []string{CapTranscribe},
		Site:         "https://assemblyai.com", KeysURL: "https://www.assemblyai.com/app/account",
		Color: "#2545F6",
	},
	"deepseek": {
		Label: "DeepSeek", Blurb: "استدلالِ قوی با قیمتِ بسیار پایین. هنوز وصل نشده.",
		Capabilities: []string{CapChat},
		Site:         "https://deepseek.com", KeysURL: "https://platform.deepseek.com/api_keys",
		Color: "#4D6BFE",
	},
	"xai": {
		Label: "xAI Grok", Blurb: "گراک، با دسترسی زندهٔ ایکس. هنوز وصل نشده.",
		Capabilities: []string{CapChat, CapImage},
		Site:         "https://x.ai", KeysURL: "https://console.x.ai",
		Color: "#000000",
	},
	"fireworks": {
		Label: "Fireworks AI", Blurb: "مدل‌های باز با سرعتِ بالا و whisper. هنوز وصل نشده.",
		Capabilities: []string{CapChat, CapTranscribe, CapImage},
		Site:         "https://fireworks.ai", KeysURL: "https://fireworks.ai/account/api-keys",
		Color: "#7B3FE4",
	},
	"fal": {
		Label: "fal.ai", Blurb: "تصویر و ویدیوی سریع، مدل‌های بازِ دیداری. هنوز وصل نشده.",
		Capabilities: []string{CapImage, CapVideo},
		Site:         "https://fal.ai", KeysURL: "https://fal.ai/dashboard/keys",
		Color: "#EC4899",
	},
	"replicate": {
		Label: "Replicate", Blurb: "هر مدلِ متن‌بازی که کسی منتشر کرده. هنوز وصل نشده.",
		Capabilities: []string{CapImage, CapVideo, CapChat},
		Site:         "https://replicate.com", KeysURL: "https://replicate.com/account/api-tokens",
		Color: "#000000",
	},
	"stability": {
		Label: "Stability AI", Blurb: "Stable Diffusion از خودِ سازنده. هنوز وصل نشده.",
		Capabilities: []string{CapImage, CapVideo},
		Site:         "https://stability.ai", KeysURL: "https://platform.stability.ai/account/keys",
		Color: "#330066",
	},
	"azure": {
		Label: "Azure OpenAI", Blurb: "همان مدل‌های OpenAI با قراردادِ سازمانیِ مایکروسافت. هنوز وصل نشده.",
		Capabilities: []string{CapChat, CapImage, CapSpeech, CapTranscribe, CapEmbed},
		Site:         "https://azure.microsoft.com/products/ai-services/openai-service",
		Color:        "#0078D4",
	},
	"bedrock": {
		Label: "AWS Bedrock", Blurb: "کلود، لاما و تایتان از داخلِ AWS. هنوز وصل نشده.",
		Capabilities: []string{CapChat, CapImage, CapEmbed},
		Site:         "https://aws.amazon.com/bedrock", Color: "#FF9900",
	},
	"perplexity": {
		Label: "Perplexity", Blurb: "پاسخ با جست‌وجوی زنده و ارجاع. هنوز وصل نشده.",
		Capabilities: []string{CapChat},
		Site:         "https://perplexity.ai", KeysURL: "https://www.perplexity.ai/settings/api",
		Color: "#20808D",
	},
	"runway": {
		Label: "Runway", Blurb: "ویدیوی تولیدی. هنوز وصل نشده.",
		Capabilities: []string{CapVideo},
		Site:         "https://runwayml.com", Color: "#000000",
	},
}

// Lookup returns the catalogue entry for a provider name, and whether one
// exists. A miss is ordinary: the caller renders a lettermark.
func Lookup(name string) (Vendor, bool) {
	v, ok := catalogue[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return Vendor{}, false
	}
	v.Name = name
	return v, true
}

// Get returns the entry for a name, inventing a minimal one when the catalogue
// does not know it, so callers never have to branch on presence.
func Get(name string) Vendor {
	if v, ok := Lookup(name); ok {
		return v
	}
	return Vendor{Name: name, Label: name}
}

// Known returns every catalogued vendor, including those this deployment does
// not route to — those are the ones a user can ask for.
func Known() []Vendor {
	out := make([]Vendor, 0, len(catalogue))
	for name := range catalogue {
		out = append(out, Get(name))
	}
	return out
}
