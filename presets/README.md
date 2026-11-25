# Skills and templates — `presets/`

Two kinds of ready-made prompt bundles, one YAML file each, loaded at start-up
(`skills_dir` / `templates_dir` in the config; the image ships them at
`/app/presets`). Adding one is a file and a deploy, no code.

## Skills — `presets/skills/*.yaml`

A skill is an instruction bundle any chat request can switch on. It never
chooses the model; it rides whatever model or agent the request names.

```bash
curl $NABU/v1/chat/completions -H "Authorization: Bearer $NABU_KEY" -d '{
  "model": "nabu-smart",
  "nabu_skills": ["json-extract", "persian-native"],
  "messages": [{"role":"user","content":"نام، موبایل و شهر را از این متن دربیاور: ..."}]
}'
```

Or with a header: `X-Nabu-Skills: json-extract,concise`. The response echoes
the skills it used in `X-Nabu-Skills`. `nabu_skills` is removed before the body
goes upstream. At most 4 skills per request; an unknown name is a 400.

The skills' text is added as one system message **after** the caller's (or the
agent's) own system prompt. `response_format` and `temperature` from a skill
apply only when the request did not set them. Skills apply to
`/v1/chat/completions` and template runs (not to flows or `/v1/responses`).

`GET /v1/skills` lists them.

```yaml
name: json-extract              # defaults to the file name
category: data
title: { fa: "استخراج JSON", en: "JSON extraction" }
description: { fa: "...", en: "..." }
response_format: json_object    # optional: "" or json_object
temperature: 0                  # optional default
system: |
  Respond with exactly one valid JSON object ...
```

## Templates — `presets/templates/*.yaml`

A template is a complete task with blanks (`{{name}}`) to fill:

```bash
curl $NABU/v1/templates/instagram-caption/run -H "Authorization: Bearer $NABU_KEY" -d '{
  "variables": {"topic": "افتتاح شعبهٔ جدید کافه در شیراز", "goal": "فروش"}
}'
```

The run is turned into an ordinary `/v1/chat/completions` request and goes
through the same handler: the key's allow-list must permit the template's
`model`, and billing, rate limits, fallbacks and the response shape are those
of a chat completion. Optional fields: `model` (override), `skills` (added to
the template's own), `stream`, `temperature`, `max_tokens`,
`conversation_id`. A missing required variable, an unknown variable or a value
outside `options` is a 400.

`GET /v1/templates` lists the templates the key may run; `GET /v1/templates/{name}`
shows one. In the console, **Templates & skills** shows both with a form and the
exact request; admins can test-run a template there.

```yaml
name: translate
category: language
title: { fa: "...", en: "..." }
description: { fa: "...", en: "..." }
model: nabu-smart                # any chat alias or agent
skills: [concise]                # optional
temperature: 0.2                 # optional
system: |                        # optional; may use {{variables}}
  You are a professional translator ...
prompt: |
  Translate the following text into {{target}}.
  {{text}}
variables:
  - { name: text, required: true, multiline: true, label: { fa: "متن", en: "Text" } }
  - { name: target, default: "Persian", options: ["Persian", "Arabic", "English"], label: { fa: "زبان مقصد", en: "Target language" } }
```

`go test ./internal/preset` loads this directory and fails on any file the
gateway would skip, a template naming a missing skill, or a missing Persian or
English label.
