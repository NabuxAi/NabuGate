package server

import "strings"

// Claude Desktop's gateway discovery may filter out arbitrary model IDs.
// This prefix is an API-compatible *transport alias* for a configured chat
// model, never a claim that the underlying model is Anthropic Claude.
// The actual model ID is restored before authorization, routing and billing.
const desktopTransportPrefix = "claude-compatible--"

func desktopTransportName(model string) string {
	return desktopTransportPrefix + model
}

func resolveDesktopTransportName(model string) string {
	return strings.TrimPrefix(model, desktopTransportPrefix)
}

func isClaudeNamed(model string) bool {
	name := strings.ToLower(model)
	return strings.Contains(name, "claude") || strings.Contains(name, "anthropic")
}
