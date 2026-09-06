---
name: dadebaran
description: Dadebaran's five Persian writers (prompt, formal, insta, english, study) as NabuGate agents — how to call them and what they return.
---

# Dadebaran on NabuGate

dadebaran.ir is a Persian writing tool: a prompt engineer, a formal-letter
editor, an Instagram coach, an English coach and a study assistant. Its prompts
live in `shm379/Dadebaran` (`app/src/prompts.ts`) and are mirrored here as
agents, so any project with a NabuGate key that allows `dadebaran-*` can use
them by name — no prompt to copy, no SDK.

| agent | give it | you get |
|---|---|---|
| `dadebaran-prompt` | a rough idea (+ use: general / image / writing / code; output language; tone) | a title, the prompt in a code block, 2–3 tips |
| `dadebaran-formal` | a casual message (+ who it is for, how formal, how long) | a title, the formal rewrite, 2 tips |
| `dadebaran-insta` | a topic (+ format, tone, how many hooks) | هوک‌ها / کپشن / CTA / هشتگ‌ها / نکته‌ها |
| `dadebaran-english` | a sentence in English or Persian (+ level, focus) | نسخهٔ درست / توضیح / برای ادامه |
| `dadebaran-study` | a text or topic (+ level, which parts) | خلاصه / فلش‌کارت‌ها / سؤال‌های امتحانی / نکته‌ها |

All answer in Markdown. Settings the site collects as form fields (tone,
length, level…) are written into the user message in plain words; the agent
reads them. Nothing is JSON-wrapped here — the site's own envelope stays on
the site.

```bash
curl $NABUGATE_BASE_URL/chat/completions \
  -H "Authorization: Bearer $NABUGATE_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"model":"dadebaran-formal","messages":[{"role":"user","content":"سلام، فاکتورو کی میدی؟\n\nخواسته‌ها: برای مدیرعامل، خیلی رسمی"}]}'
```

Keys that reach them: `nabucrm` (the NabuOS console offers `os_dadebaran`)
and `dadebaran` itself. Add `dadebaran-*` to another key's allow-list to grant
it; the agents cost what `nabu-smart` costs.

When the site's prompt changes, change the agent here too — each file names
its source in the header. The two must not drift.
