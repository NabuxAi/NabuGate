---
name: mrc-imagegen
description: Generate branded Persian and multi-language graphics, Instagram post cards (1080x1350), video headers (1600x900), story carousels (1080x1920), and cinematic MP4 motion videos with sound design using the mrc_imagegen API. Supports 15 canvas aspect ratios, 14 visual style archetypes, 18 slide primitives, and 152 Shotcraft motion recipes.
---

# MrChatGPT Graphic & Motion Video Generator

Call `mrc_imagegen` API (`https://imagen.nabuxai.com` or local `http://localhost:8000`) to render high-DPI raster images and MP4 motion videos.

## 1. Quick Endpoints

| Endpoint | Method | Purpose |
|---|---|---|
| `/v1/deck/render` | `POST` | Render structured multi-slide JSON deck into PNG/JPG or ZIP bundle |
| `/v1/video/render` | `POST` | Render structured deck into an MP4 motion video with transitions and SFX audio |
| `/v1/director/direct` | `POST` | Auto-direct a deck with pacing, slide timings, and shotcraft recipe cues |
| `/v1/director/hooks` | `POST` | Generate high-converting viral hooks for Slide 1 |
| `/v1/motion/recipes` | `GET` | Browse 152 Shotcraft motion recipes |

## 2. Rendering a Multi-Slide Deck (Python Example)

```python
import httpx

deck_payload = {
    "format": "story",  # story, post, card, header, cinematic
    "style": "cyber",   # editorial, brutalist, glass, paper, cyber, swiss, bento, synthwave, claymorphism, solar, chromatic, comic, nordic, gold
    "lang": "fa",       # fa, en, ar, tr, es, de, fr, ru, zh, ja, ko, auto
    "accent": "#00ff88",
    "slides": [
        {
            "type": "cover",
            "kicker": "هوش مصنوعی ۲۰۲۶",
            "l1": "راهنمای جامع کاربری",
            "l2": "موتورهای نسل جدید",
            "sub": "گام‌به‌گام از ایده تا خروجی نهایی.",
            "chips": ["پرامپت", "ویدیو", "موشن"]
        },
        {
            "type": "stat",
            "kicker": "شاخص کلیدی",
            "big": "۱۸۰",
            "unit": "توکن در ثانیه",
            "body": "افزایش ۴ برابری سرعت تولید در مقایسه با نسل قبلی."
        },
        {
            "type": "cta",
            "kicker": "دانلود و شروع",
            "l1": "همین الان شروع کنید",
            "reply": "AI",
            "link": "mrchatgpt.org"
        }
    ]
}

res = httpx.post("https://imagen.nabuxai.com/v1/deck/render", json=deck_payload, headers={"X-API-Key": "YOUR_KEY"})
with open("carousel.zip", "wb") as f:
    f.write(res.content)
```

## 3. Rendering Motion Video with SFX

```python
video_payload = {
    **deck_payload,
    "slide_duration": 3.5,
    "transition": "smoothleft",
    "transition_duration": 0.5,
    "with_sfx": True,
    "fps": 30
}

res = httpx.post("https://imagen.nabuxai.com/v1/video/render", json=video_payload, headers={"X-API-Key": "YOUR_KEY"})
with open("promo.mp4", "wb") as f:
    f.write(res.content)
```
