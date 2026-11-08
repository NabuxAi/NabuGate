package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"nabugate/internal/provider"
)

// Anthropic-compatible Messages API adapter for clients which require the
// Messages wire protocol (including Claude Desktop third-party inference).
// Billing, policy and BYOK all run through the existing router and auth layer.
// This is not a Claude.ai OAuth implementation and must not collect session
// cookies or Claude.ai subscription credentials.
type nabuAnthropicRequest struct {
	Model         string            `json:"model"`
	MaxTokens     int               `json:"max_tokens"`
	Messages      []nabuAnthTurn    `json:"messages"`
	System        json.RawMessage   `json:"system"`
	Stream        bool              `json:"stream"`
	Temperature   *float64          `json:"temperature"`
	TopP          *float64          `json:"top_p"`
	StopSequences []string          `json:"stop_sequences"`
	Tools         []nabuAnthTool    `json:"tools"`
	ToolChoice    json.RawMessage   `json:"tool_choice"`
}
type nabuAnthTurn struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}
type nabuAnthTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}
type nabuAnthBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	Source    struct {
		Type      string `json:"type"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
		URL       string `json:"url"`
	} `json:"source"`
}

func nabuAnthError(w http.ResponseWriter, status int, kind, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "error", "error": map[string]any{"type": kind, "message": message},
	})
}

func nabuAnthText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil
	}
	var blocks []nabuAnthBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", fmt.Errorf("expected a string or content blocks")
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Type != "text" {
			return "", fmt.Errorf("unsupported nested block type %q", block.Type)
		}
		parts = append(parts, block.Text)
	}
	return strings.Join(parts, "\n"), nil
}

func nabuAnthConvert(in nabuAnthropicRequest) (provider.ChatRequest, error) {
	msgs := make([]map[string]any, 0, len(in.Messages)+1)
	raw := map[string]json.RawMessage{}
	typed := []provider.Message{}
	addText := func(role, content string) {
		if content == "" {
			return
		}
		msgs = append(msgs, map[string]any{"role": role, "content": content})
		typed = append(typed, provider.Message{Role: role, Content: content})
	}
	if len(in.System) > 0 {
		system, err := nabuAnthText(in.System)
		if err != nil {
			return provider.ChatRequest{}, fmt.Errorf("system: %w", err)
		}
		addText("system", system)
	}
	for i, msg := range in.Messages {
		if msg.Role != "user" && msg.Role != "assistant" {
			return provider.ChatRequest{}, fmt.Errorf("messages[%d]: role must be user or assistant", i)
		}
		var simple string
		if err := json.Unmarshal(msg.Content, &simple); err == nil {
			addText(msg.Role, simple)
			continue
		}
		var blocks []nabuAnthBlock
		if err := json.Unmarshal(msg.Content, &blocks); err != nil {
			return provider.ChatRequest{}, fmt.Errorf("messages[%d]: invalid content", i)
		}
		if msg.Role == "assistant" {
			parts := []map[string]any{}
			calls := []map[string]any{}
			for _, b := range blocks {
				switch b.Type {
				case "text":
					parts = append(parts, map[string]any{"type": "text", "text": b.Text})
				case "tool_use":
					if b.ID == "" || b.Name == "" {
						return provider.ChatRequest{}, fmt.Errorf("messages[%d]: tool_use needs id and name", i)
					}
					args := string(b.Input)
					if args == "" || !json.Valid(b.Input) {
						args = "{}"
					}
					calls = append(calls, map[string]any{"id": b.ID, "type": "function",
						"function": map[string]any{"name": b.Name, "arguments": args}})
				default:
					return provider.ChatRequest{}, fmt.Errorf("messages[%d]: unsupported assistant block %q", i, b.Type)
				}
			}
			converted := map[string]any{"role": "assistant", "content": parts}
			if len(calls) > 0 {
				converted["tool_calls"] = calls
			}
			msgs = append(msgs, converted)
			// The native adapters read Raw to preserve tools and structured blocks.
			typed = append(typed, provider.Message{Role: "assistant"})
			continue
		}
		parts := []map[string]any{}
		flushParts := func() {
			if len(parts) > 0 {
				msgs = append(msgs, map[string]any{"role": "user", "content": parts})
				typed = append(typed, provider.Message{Role: "user"})
				parts = nil
			}
		}
		for _, b := range blocks {
			switch b.Type {
			case "text":
				parts = append(parts, map[string]any{"type": "text", "text": b.Text})
			case "image":
				var url string
				switch b.Source.Type {
				case "base64":
					if !strings.HasPrefix(b.Source.MediaType, "image/") || b.Source.Data == "" {
						return provider.ChatRequest{}, fmt.Errorf("messages[%d]: invalid image", i)
					}
					url = "data:" + b.Source.MediaType + ";base64," + b.Source.Data
				case "url":
					if !strings.HasPrefix(b.Source.URL, "https://") {
						return provider.ChatRequest{}, fmt.Errorf("messages[%d]: image URL must use HTTPS", i)
					}
					url = b.Source.URL
				default:
					return provider.ChatRequest{}, fmt.Errorf("messages[%d]: unsupported image source", i)
				}
				parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": url}})
			case "tool_result":
				flushParts()
				if b.ToolUseID == "" {
					return provider.ChatRequest{}, fmt.Errorf("messages[%d]: tool_result requires tool_use_id", i)
				}
				value, err := nabuAnthText(b.Content)
				if err != nil {
					return provider.ChatRequest{}, fmt.Errorf("messages[%d] tool_result: %w", i, err)
				}
				msgs = append(msgs, map[string]any{"role": "tool", "tool_call_id": b.ToolUseID, "content": value})
				typed = append(typed, provider.Message{Role: "user", Content: value})
			default:
				return provider.ChatRequest{}, fmt.Errorf("messages[%d]: unsupported user block %q", i, b.Type)
			}
		}
		flushParts()
	}
	if len(msgs) == 0 {
		return provider.ChatRequest{}, fmt.Errorf("messages must not be empty")
	}
	encode := func(key string, val any) error {
		data, err := json.Marshal(val)
		if err == nil {
			raw[key] = data
		}
		return err
	}
	if err := encode("model", in.Model); err != nil {
		return provider.ChatRequest{}, err
	}
	if err := encode("messages", msgs); err != nil {
		return provider.ChatRequest{}, err
	}
	_ = encode("max_tokens", in.MaxTokens)
	if in.Temperature != nil {
		_ = encode("temperature", *in.Temperature)
	}
	if in.TopP != nil {
		_ = encode("top_p", *in.TopP)
	}
	if len(in.StopSequences) > 0 {
		_ = encode("stop", in.StopSequences)
	}
	if len(in.Tools) > 0 {
		tools := make([]map[string]any, 0, len(in.Tools))
		for _, tool := range in.Tools {
			if tool.Name == "" {
				return provider.ChatRequest{}, fmt.Errorf("tools: name required")
			}
			schema := tool.InputSchema
			if len(schema) == 0 || string(schema) == "null" {
				schema = json.RawMessage("{\"type\":\"object\",\"properties\":{}}")
			}
			if !json.Valid(schema) {
				return provider.ChatRequest{}, fmt.Errorf("tools: invalid input_schema")
			}
			tools = append(tools, map[string]any{"type": "function", "function": map[string]any{
				"name": tool.Name, "description": tool.Description, "parameters": schema,
			}})
		}
		_ = encode("tools", tools)
	}
	if len(in.ToolChoice) > 0 && string(in.ToolChoice) != "null" {
		var choice struct {
			Type string `json:"type"`
			Name string `json:"name"`
			DisableParallelToolUse bool `json:"disable_parallel_tool_use"`
		}
		if json.Unmarshal(in.ToolChoice, &choice) != nil {
			return provider.ChatRequest{}, fmt.Errorf("invalid tool_choice")
		}
		switch choice.Type {
		case "none", "auto":
			_ = encode("tool_choice", choice.Type)
		case "any":
			_ = encode("tool_choice", "required")
		case "tool":
			if choice.Name == "" {
				return provider.ChatRequest{}, fmt.Errorf("tool_choice name required")
			}
			_ = encode("tool_choice", map[string]any{"type": "function", "function": map[string]any{"name": choice.Name}})
		default:
			return provider.ChatRequest{}, fmt.Errorf("unsupported tool_choice type")
		}
		if choice.DisableParallelToolUse {
			_ = encode("parallel_tool_calls", false)
		}
	}
	if len(msgs) > 0 {
		// Typed fields enable the native Anthropic/Gemini adapters. Tools and
		// images remain intact in the raw OpenAI-wire messages.
	}
	return provider.ChatRequest{
		Messages: typed, Temperature: in.Temperature, TopP: in.TopP,
		MaxTokens: &in.MaxTokens, Stop: raw["stop"], Raw: raw,
	}, nil
}

func nabuAnthOutputBlocks(text string, calls json.RawMessage) ([]map[string]any, string) {
	blocks := make([]map[string]any, 0, 3)
	if text != "" {
		blocks = append(blocks, map[string]any{"type": "text", "text": text})
	}
	reason := "end_turn"
	var tools []struct {
		ID string `json:"id"`
		Function struct {
			Name string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	}
	if json.Unmarshal(calls, &tools) == nil {
		for _, call := range tools {
			if call.ID == "" || call.Function.Name == "" {
				continue
			}
			input := json.RawMessage(call.Function.Arguments)
			if !json.Valid(input) {
				input = json.RawMessage("{}")
			}
			blocks = append(blocks, map[string]any{
				"type": "tool_use", "id": call.ID, "name": call.Function.Name, "input": input,
			})
			reason = "tool_use"
		}
	}
	if len(blocks) == 0 {
		blocks = append(blocks, map[string]any{"type": "text", "text": ""})
	}
	return blocks, reason
}

func (s *Server) handleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	var incoming nabuAnthropicRequest
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		nabuAnthError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
		return
	}
	if incoming.Model == "" || incoming.MaxTokens < 1 || len(incoming.Messages) == 0 {
		nabuAnthError(w, http.StatusBadRequest, "invalid_request_error", "model, max_tokens and nonempty messages are required")
		return
	}
	if !s.aliasAllowed(w, r, incoming.Model) {
		return
	}
	request, err := nabuAnthConvert(incoming)
	if err != nil {
		nabuAnthError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if incoming.Stream {
		s.streamAnthropicMessages(w, r, incoming, request)
		return
	}
	result, err := s.router.Chat(r.Context(), incoming.Model, request)
	if err != nil {
		nabuAnthError(w, aliasErrStatus(err, "unknown model alias"), "api_error", err.Error())
		return
	}
	cost := s.record(r, result.Provider, result.Model, result.Response.Usage)
	w.Header().Set("X-Nabu-Provider", result.Provider)
	w.Header().Set("X-Nabu-Model", result.Model)
	w.Header().Set(costHeader, formatCost(cost))
	blocks, reason := nabuAnthOutputBlocks(result.Response.Content, result.Response.ToolCalls)
	if reason == "end_turn" && result.Response.FinishReason == "length" {
		reason = "max_tokens"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": "msg_nabu_" + fmt.Sprint(time.Now().UnixNano()),
		"type": "message", "role": "assistant", "content": blocks,
		"model": incoming.Model, "stop_reason": reason, "stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens": result.Response.Usage.PromptTokens,
			"output_tokens": result.Response.Usage.CompletionTokens,
		},
	})
}

type nabuAnthToolAccumulator struct {
	id, name, arguments string
}
func (s *Server) streamAnthropicMessages(w http.ResponseWriter, r *http.Request, in nabuAnthropicRequest, request provider.ChatRequest) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		nabuAnthError(w, http.StatusInternalServerError, "api_error", "streaming not supported")
		return
	}
	id := "msg_nabu_" + fmt.Sprint(time.Now().UnixNano())
	headersWritten := false
	textOpen := false
	blockIndex := 0
	var chosenProvider, chosenModel string
	tools := make(map[int]*nabuAnthToolAccumulator)
	sent := func(event string, data any) {
		raw, _ := json.Marshal(data)
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, raw)
		flusher.Flush()
	}
	start := func() {
		if headersWritten {
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		headersWritten = true
		sent("message_start", map[string]any{"type": "message_start", "message": map[string]any{
			"id": id, "type": "message", "role": "assistant",
			"content": []any{}, "model": in.Model, "stop_reason": nil,
			"stop_sequence": nil, "usage": map[string]any{"input_tokens": 0, "output_tokens": 0},
		}})
	}
	ctx := provider.WithToolDeltas(r.Context(), func(raw json.RawMessage) error {
		var delta []struct {
			Index int `json:"index"`
			ID string `json:"id"`
			Function struct {
				Name string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		}
		if err := json.Unmarshal(raw, &delta); err != nil {
			return err
		}
		for _, part := range delta {
			item := tools[part.Index]
			if item == nil {
				item = &nabuAnthToolAccumulator{}
				tools[part.Index] = item
			}
			if part.ID != "" {
				item.id = part.ID
			}
			if part.Function.Name != "" {
				item.name += part.Function.Name
			}
			item.arguments += part.Function.Arguments
		}
		return nil
	})
	result, err := s.router.ChatStream(ctx, in.Model, request,
		func(p, m string) { chosenProvider, chosenModel = p, m },
		func(delta string) error {
			start()
			if !textOpen {
				sent("content_block_start", map[string]any{
					"type": "content_block_start", "index": blockIndex,
					"content_block": map[string]any{"type": "text", "text": ""},
				})
				textOpen = true
			}
			sent("content_block_delta", map[string]any{
				"type": "content_block_delta", "index": blockIndex,
				"delta": map[string]any{"type": "text_delta", "text": delta},
			})
			return nil
		},
	)
	if err != nil && !headersWritten {
		if chosenProvider != "" {
			s.recordBrokenUsage(r, result.Provider, result.Model, result.Usage)
		}
		nabuAnthError(w, aliasErrStatus(err, "unknown model alias"), "api_error", err.Error())
		return
	}
	start()
	if textOpen {
		sent("content_block_stop", map[string]any{"type": "content_block_stop", "index": blockIndex})
		blockIndex++
	}
	if err != nil {
		s.recordBrokenUsage(r, result.Provider, result.Model, result.Usage)
		sent("error", map[string]any{"type": "error", "error": map[string]any{
			"type": "api_error", "message": err.Error(),
		}})
		return
	}
	indices := make([]int, 0, len(tools))
	for i := range tools {
		indices = append(indices, i)
	}
	sort.Ints(indices)
	reason := "end_turn"
	for _, i := range indices {
		tool := tools[i]
		if tool.id == "" || tool.name == "" {
			continue
		}
		args := tool.arguments
		if !json.Valid([]byte(args)) {
			args = "{}"
		}
		sent("content_block_start", map[string]any{
			"type": "content_block_start", "index": blockIndex,
			"content_block": map[string]any{"type": "tool_use", "id": tool.id, "name": tool.name, "input": map[string]any{}},
		})
		sent("content_block_delta", map[string]any{
			"type": "content_block_delta", "index": blockIndex,
			"delta": map[string]any{"type": "input_json_delta", "partial_json": args},
		})
		sent("content_block_stop", map[string]any{"type": "content_block_stop", "index": blockIndex})
		blockIndex++
		reason = "tool_use"
	}
	_ = s.record(r, result.Provider, result.Model, result.Usage)
	sent("message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{"stop_reason": reason, "stop_sequence": nil},
		"usage": map[string]any{"input_tokens": result.Usage.PromptTokens, "output_tokens": result.Usage.CompletionTokens},
	})
	sent("message_stop", map[string]any{"type": "message_stop"})
}
