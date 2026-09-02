# Getting started as a customer

One base URL, one key, every model. Anything that speaks the OpenAI API works
with NabuGate by changing its base URL.

## 1. Account

Sign up at `/panel/` with an email and password, or with a Google or Nabu
account.

## 2. Credit

NabuGate has no monthly plan. The account holds a USD wallet and every request
deducts exactly the model's real price. Credit never expires.

Open **خرید و شارژ**, pick a package or any amount from $1 to $5,000, and pay
at the bank's page. The bank charges in toman at the day's rate; the wallet is
credited the USD amount chosen. Card details never reach NabuGate.

The balance is credited only after the bank confirms. If you return to the
panel before that, the Payments page shows the invoice as pending and asks the
gateway again each time it is opened. See `docs/payments.md` for every failure
and what to do.

## 3. Key

Open **کلیدهای API**, create one key **per application**. The full key is shown
once; only its hash is stored. Each key can be limited to a model glob
(`nabu-*`), to browser origins, to a provider, and to a rate limit.

## 4. Connect

```bash
export OPENAI_BASE_URL="https://gate.nabuxai.com/v1"
export OPENAI_API_KEY="ng_xxxxxxxxxxxxxxxxxxxx"
```

Then use any alias from `GET /v1/models`:

| Name | Use |
|---|---|
| `nabu-fast` | cheap and quick; chat, summaries, classification |
| `nabu-smart` | strongest reasoning; coding assistants |
| `nabu-embed` | vectors for search |
| `write-embed` | vectors for a **stored** index (no fallback, fixed width) |
| `openai/gpt-…`, `anthropic/claude-…` | one specific model, no fallback |

Tool-specific guides (Cursor, Cline, Claude Code, Codex, VS Code, SDKs) are on
the public docs page at `/docs`.

## Watching your balance

Every metered response carries:

```
X-Nabu-Balance-USD: 4.1837
X-Nabu-Balance-Warning: low      # only below $1
```

At zero the key answers `402 Payment Required` until any top-up; no new key is
needed.

## Errors

| Code | Meaning |
|---|---|
| 401 | wrong, deleted or disabled key; missing `Authorization: Bearer` |
| 402 | balance is zero |
| 403 | origin not allowed for this key, or model outside its allow-list |
| 404 | model name not in `GET /v1/models` |
| 429 | key's rate limit |
| 502 | every provider in the alias chain failed; retry or use a direct model |

**درخواست‌های اخیر** in the panel lists each refused call with its reason.
