#!/usr/bin/env bash
#
# Installs the daily status report on the host as a systemd timer.
#
# Run on the server, as root:
#   TELEGRAM_BOT_TOKEN=... TELEGRAM_CHAT_ID=... ./install-status-report.sh
#
# A timer rather than a container: the report reads provider keys straight out
# of the running gateway container, so it needs the host's docker socket and
# nothing else. Putting it in the compose would mean either mounting that socket
# into a distroless image or duplicating every secret a second time.

set -euo pipefail

: "${TELEGRAM_BOT_TOKEN:?set TELEGRAM_BOT_TOKEN}"
: "${TELEGRAM_CHAT_ID:?set TELEGRAM_CHAT_ID}"

AT="${REPORT_AT:-08:00}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

install -m 0755 "$SCRIPT_DIR/status-report.sh" /usr/local/bin/nabu-status-report

umask 077
cat >/etc/nabu-status.env <<EOF
TELEGRAM_BOT_TOKEN=$TELEGRAM_BOT_TOKEN
TELEGRAM_CHAT_ID=$TELEGRAM_CHAT_ID
EOF
chmod 600 /etc/nabu-status.env

cat >/etc/systemd/system/nabu-status-report.service <<'EOF'
[Unit]
Description=NabuGate daily status report to Telegram
After=docker.service
Wants=docker.service

[Service]
Type=oneshot
ExecStart=/usr/local/bin/nabu-status-report
EOF

cat >/etc/systemd/system/nabu-status-report.timer <<EOF
[Unit]
Description=Run the NabuGate status report once a day

[Timer]
OnCalendar=*-*-* ${AT}:00
# The box runs ~146 containers and its nightly dump has pushed load past 700.
# A fixed minute would land every timer on the hour together.
RandomizedDelaySec=600
Persistent=true

[Install]
WantedBy=timers.target
EOF

systemctl daemon-reload
systemctl enable --now nabu-status-report.timer
systemctl list-timers nabu-status-report.timer --no-pager
