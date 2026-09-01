package opsless

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"
)

var (
	// Mute duplicates for 5 minutes
	lastSent   = make(map[string]time.Time)
	lastSentMu sync.Mutex
)

// NotifySelfHealing sends a Zero-UI Telegram alert when NabuGate performs an autonomous fallback.
// It fires and forgets (async) to not block the AI request.
func NotifySelfHealing(logger *slog.Logger, project, originalProvider, originalModel, fallbackProvider, fallbackModel, errReason string) {
	token := os.Getenv("OPSLESS_TELEGRAM_BOT_TOKEN")
	chatId := os.Getenv("OPSLESS_ADMIN_CHAT_ID")

	if token == "" || chatId == "" {
		return
	}

	key := fmt.Sprintf("%s-%s", originalProvider, fallbackProvider)
	lastSentMu.Lock()
	if time.Since(lastSent[key]) < 5*time.Minute {
		lastSentMu.Unlock()
		return // Rate limited
	}
	lastSent[key] = time.Now()
	lastSentMu.Unlock()

	go func() {
		msg := fmt.Sprintf("🛡 **Opsless NabuGate (Self-Healing)**\n\n"+
			"پروژه: `%s`\n"+
			"ارائه‌دهنده اصلی `%s` (`%s`) دچار مشکل شد.\n"+
			"خطا: `%s`\n\n"+
			"✅ ترافیک **بدون قطعی (Zero Downtime)** به `%s` (`%s`) هدایت شد.",
			project, originalProvider, originalModel, errReason, fallbackProvider, fallbackModel)

		payload := map[string]interface{}{
			"chat_id":    chatId,
			"text":       msg,
			"parse_mode": "Markdown",
		}

		body, _ := json.Marshal(payload)
		resp, err := http.Post(fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token), "application/json", bytes.NewReader(body))
		if err != nil {
			logger.Warn("Failed to send opsless self-healing alert", "error", err)
			return
		}
		defer resp.Body.Close()
	}()
}
