#!/usr/bin/env bash
#
# Daily NabuGate health report, delivered to Telegram.
#
# It answers the question the console cannot answer from config alone: which
# aliases actually respond right now, and which provider accounts still have
# credit. Config states intent; only a live call states fact — and the outage
# this script exists to catch was precisely that gap. Every rung of nabu-smart
# resolved to Parspack, Parspack ran out of credit, and the gateway returned 502
# for every message for as long as nobody happened to send one by hand.
#
# Probes cost a token or two each: max_tokens=1 against ~10 aliases, once a day.
#
# Secrets are never written into this file. Provider keys are read at run time
# out of the running gateway container, and the Telegram credentials come from
# an env file the installer creates:
#
#   /etc/nabu-status.env   (chmod 600)
#     TELEGRAM_BOT_TOKEN=...
#     TELEGRAM_CHAT_ID=...
#
# Install: scripts/install-status-report.sh on the host.

set -uo pipefail

ENV_FILE="${NABU_STATUS_ENV:-/etc/nabu-status.env}"
[ -r "$ENV_FILE" ] && . "$ENV_FILE"

# DRY_RUN=1 prints the report instead of sending it — the only way to read what
# a change produced without spending a message on the owner's phone.
DRY_RUN="${DRY_RUN:-}"

if [ -z "$DRY_RUN" ]; then
  : "${TELEGRAM_BOT_TOKEN:?TELEGRAM_BOT_TOKEN is required (see $ENV_FILE)}"
  : "${TELEGRAM_CHAT_ID:?TELEGRAM_CHAT_ID is required (see $ENV_FILE)}"
fi

GATEWAY="${NABUGATE_BASE_URL:-https://gate.nabuxai.com/v1}"

# Container names move on every Coolify redeploy, so match on the stable
# project suffix rather than pinning the full name.
find_container() { docker ps --format '{{.Names}}' | grep -m1 -- "$1"; }

GATE_CTR="$(find_container 'nabugate-')"
CHAT_CTR="$(find_container 'dashboard-g41lt2s2fbq6fbozh71yyjo1')"
CHAT_DB_CTR="$(find_container 'postgres-g41lt2s2fbq6fbozh71yyjo1')"

# env_of reads one variable out of a running container without needing a shell
# inside it — the gateway image is distroless and has none.
env_of() {
  local ctr="$1" key="$2"
  [ -n "$ctr" ] || return 0
  docker inspect "$ctr" --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null \
    | grep -m1 "^${key}=" | cut -d= -f2-
}

ADMIN_KEY="$(env_of "$GATE_CTR" NABU_API_KEY)"
OR_KEY_1="$(env_of "$GATE_CTR" OPENROUTER_API_KEY)"
OR_KEY_2="$(env_of "$GATE_CTR" OPENROUTER_API_KEY_2)"
GEMINI_KEY="$(env_of "$GATE_CTR" GEMINI_API_KEY)"
GAMMA_KEY="$(env_of "$GATE_CTR" GAMMA_API_KEY)"

OUT=""
say() { OUT="${OUT}$1"$'\n'; }

# ---------------------------------------------------------------- aliases ----
# A 200 with empty content is a failure, not a success: an upstream can close a
# stream having emitted a role delta, a stop and nothing else, and reporting
# that as healthy is how a dead alias stays dead.
probe_alias() {
  local alias="$1"
  local body
  body="$(curl -s --max-time 45 \
    -H "Authorization: Bearer $ADMIN_KEY" \
    -H 'Content-Type: application/json' \
    -d "{\"model\":\"$alias\",\"messages\":[{\"role\":\"user\",\"content\":\"ok\"}],\"max_tokens\":1}" \
    "$GATEWAY/chat/completions")"

  ALIAS_STATE="$(printf '%s' "$body" | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
except Exception:
    print("DOWN|unreadable response"); raise SystemExit
if "error" in d:
    msg = str(d["error"].get("message", ""))
    # The router concatenates every failed rung; the first one names the cause.
    print("DOWN|" + msg.split("; ")[0][:110]); raise SystemExit
try:
    c = d["choices"][0]["message"].get("content") or ""
except Exception:
    print("DOWN|no choices"); raise SystemExit
print(("UP|" + (d.get("model") or "")) if c.strip() else "EMPTY|200 with no content")
')"
}

say "🤖 <b>NabuGate — گزارش روزانه</b>"
say "$(date '+%Y-%m-%d %H:%M %Z')"
say ""
say "<b>Aliasها</b>"

for alias in nabu-smart nabu-fast nabu-cheap nabu-vision nabu-kimi nabu-minimax nabu-9router; do
  probe_alias "$alias"
  case "${ALIAS_STATE%%|*}" in
    UP)    say "✅ <code>$alias</code>" ;;
    EMPTY) say "⚠️ <code>$alias</code> — ${ALIAS_STATE#*|}" ;;
    *)     say "❌ <code>$alias</code> — ${ALIAS_STATE#*|}" ;;
  esac
done

# ------------------------------------------------------------- embeddings ----
EMBED="$(curl -s --max-time 45 \
  -H "Authorization: Bearer $ADMIN_KEY" -H 'Content-Type: application/json' \
  -d '{"model":"chat-embed","input":"ok","dimensions":1536}' \
  "$GATEWAY/embeddings" | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
    if "error" in d:
        print("❌ " + str(d["error"].get("message",""))[:110]); raise SystemExit
    print("✅ %d بعد" % len(d["data"][0]["embedding"]))
except SystemExit:
    raise
except Exception:
    print("❌ unreadable response")
')"
say ""
say "<b>Embedding</b>"
say "<code>chat-embed</code> — $EMBED"

# ----------------------------------------------------------------- credit ----
say ""
say "<b>اعتبار</b>"

openrouter_credit() {
  local key="$1" label="$2"
  [ -n "$key" ] || { say "➖ $label — کلید تنظیم نشده"; return; }
  local line
  line="$(curl -s --max-time 25 -H "Authorization: Bearer $key" \
    https://openrouter.ai/api/v1/key | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)["data"]
except Exception:
    print("پاسخ نامعتبر"); raise SystemExit
limit = d.get("limit")
tier = "رایگان" if d.get("is_free_tier") else "پولی"
used = d.get("usage", 0)
if limit is None:
    print("%s · مصرف %.4f$ · بدون سقف" % (tier, used))
else:
    print("%s · مصرف %.4f$ از %.2f$" % (tier, used, limit))
')"
  say "• $label — $line"
}

openrouter_credit "$OR_KEY_1" "OpenRouter #1"
openrouter_credit "$OR_KEY_2" "OpenRouter #2"

# Parspack publishes no balance endpoint — every /credits, /balance, /me path
# answers 404 — so the only honest reading is what a real call returns. 402 is
# the empty-wallet signal.
PARS="$(curl -s --max-time 40 -H "Authorization: Bearer $ADMIN_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"model":"nabu-parspack","messages":[{"role":"user","content":"ok"}],"max_tokens":1}' \
  "$GATEWAY/chat/completions" | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
except Exception:
    print("پاسخ نامعتبر"); raise SystemExit
m = str(d.get("error", {}).get("message", ""))
if not m:
    print("✅ اعتبار دارد")
elif "402" in m or "insufficient" in m.lower():
    print("❌ اعتبار تمام شده")
else:
    print("⚠️ " + m.split("; ")[0][:90])
')"
say "• Parspack — $PARS"

# Gemini has no balance either; the free tier simply starts answering 429.
GEM="$(curl -s --max-time 30 -X POST \
  "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=$GEMINI_KEY" \
  -H 'Content-Type: application/json' -d '{"contents":[{"parts":[{"text":"ok"}]}]}' | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
except Exception:
    print("پاسخ نامعتبر"); raise SystemExit
if "error" in d:
    code = d["error"].get("code")
    print("❌ سهمیه تمام شده (429)" if code == 429 else "❌ خطای %s" % code)
else:
    print("✅ در سهمیه")
')"
say "• Gemini — $GEM"

if [ -n "$GAMMA_KEY" ]; then
  say "• Gamma — ✅ کلید تنظیم شده"
else
  say "• Gamma — ➖ کلید تنظیم نشده (nabu-slides غیرفعال)"
fi

# --------------------------------------------------------------- nabuchat ----
if [ -n "$CHAT_DB_CTR" ]; then
  CHAT_STATS="$(docker exec "$CHAT_DB_CTR" psql -U nabuchat -d nabuchat -t -A -F'|' -c "
    SELECT
      (SELECT count(*) FROM agents WHERE hidden = false),
      (SELECT count(*) FROM data_sources WHERE status = 'synched'),
      (SELECT count(*) FROM data_sources WHERE status = 'error'),
      (SELECT coalesce(sum(nb_agent_queries), 0) FROM usages);
  " 2>/dev/null | tr -d ' ')"
  if [ -n "$CHAT_STATS" ]; then
    IFS='|' read -r N_AGENTS N_OK N_ERR N_Q <<<"$CHAT_STATS"
    say ""
    say "<b>NabuChat</b>"
    say "• ایجنت فعال: $N_AGENTS"
    say "• منابع ایندکس‌شده: $N_OK  ·  خطا: $N_ERR"
    say "• کوئری این ماه: $N_Q"
  fi
fi

# ------------------------------------------------------------- containers ----
say ""
say "<b>کانتینرها</b>"
for pair in "NabuGate:$GATE_CTR" "NabuChat:$CHAT_CTR"; do
  label="${pair%%:*}"; ctr="${pair#*:}"
  if [ -z "$ctr" ]; then
    say "❌ $label — بالا نیست"
  else
    say "✅ $label — $(docker inspect -f '{{.State.Status}}' "$ctr" 2>/dev/null)"
  fi
done

# ----------------------------------------------------------------- deliver ----
# Telegram rejects anything over 4096 characters, and a report that grows past
# it must not vanish silently.
send_chunk() {
  curl -s --max-time 30 -o /dev/null \
    -X POST "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/sendMessage" \
    --data-urlencode "chat_id=${TELEGRAM_CHAT_ID}" \
    --data-urlencode "text=$1" \
    -d parse_mode=HTML \
    -d disable_web_page_preview=true
}

if [ -n "$DRY_RUN" ]; then
  printf '%s' "$OUT"
  exit 0
fi

printf '%s' "$OUT" | awk '
  { if (length(buf) + length($0) + 1 > 3500) { printf "%s\f", buf; buf = "" }
    buf = buf $0 "\n" }
  END { if (length(buf)) printf "%s", buf }
' | while IFS= read -r -d $'\f' chunk || [ -n "$chunk" ]; do
  send_chunk "$chunk"
done
