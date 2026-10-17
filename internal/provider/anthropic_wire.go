package provider

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The OpenAI chat body, translated into the Anthropic Messages API.
//
// The typed ChatRequest.Messages are text only, so a conversation with tool
// calls, tool results or images is read from Raw["messages"], the body as the
// caller sent it. Without this, Claude rungs answered text only: a LangChain
// or Agents SDK agent routed to one lost its tools, and a tool result sent
// back was dropped before it reached the model.

// openAIWireMessage is one message of an OpenAI chat body.
type openAIWireMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCallID string          `json:"tool_call_id"`
	ToolCalls  []struct {
		ID       string `json:"id"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls"`
}

// anthropicTurn is one Anthropic message: a role and its content blocks.
type anthropicTurn struct {
	Role    string           `json:"role"`
	Content []map[string]any `json:"content"`
}

// buildAnthropicBody renders req as an Anthropic Messages request body.
func buildAnthropicBody(req ChatRequest, stream bool) ([]byte, error) {
	system, turns, err := anthropicConversation(req)
	if err != nil {
		return nil, err
	}

	maxTokens := 1024
	if req.ParamStyle == ParamStyleReasoning {
		// These models think before they answer, and the thinking counts
		// against max_tokens: 1024 left a hard question with no answer.
		maxTokens = 8192
	}
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		maxTokens = *req.MaxTokens
	}

	payload := map[string]any{
		"model":      req.Model,
		"max_tokens": maxTokens,
		"messages":   turns,
	}
	if stream {
		payload["stream"] = true
	}
	if system != "" {
		payload["system"] = system
	}
	if t := anthropicSampling(req, req.Temperature); t != nil {
		payload["temperature"] = *t
	}
	if p := anthropicSampling(req, req.TopP); p != nil {
		payload["top_p"] = *p
	}
	if stops := stopToSlice(req.Stop); len(stops) > 0 {
		payload["stop_sequences"] = stops
	}
	if err := addAnthropicTools(req, payload); err != nil {
		return nil, err
	}
	addAnthropicFormat(req, payload)
	return json.Marshal(payload)
}

// anthropicConversation splits the conversation into Anthropic's top-level
// system prompt and alternating user/assistant turns.
func anthropicConversation(req ChatRequest) (string, []anthropicTurn, error) {
	var system []string
	var turns []anthropicTurn
	add := func(role string, blocks ...map[string]any) {
		if len(blocks) == 0 {
			return
		}
		// Consecutive turns of one role are one turn to Anthropic: several
		// tool results answering one parallel call must arrive together.
		if n := len(turns); n > 0 && turns[n-1].Role == role {
			turns[n-1].Content = append(turns[n-1].Content, blocks...)
			return
		}
		turns = append(turns, anthropicTurn{Role: role, Content: blocks})
	}

	raw := req.Raw["messages"]
	if len(raw) == 0 {
		for _, m := range req.Messages {
			if m.Role == "system" {
				system = append(system, m.Content)
				continue
			}
			add(m.Role, textBlock(m.Content)...)
		}
		return strings.Join(system, "\n\n"), turns, nil
	}

	var msgs []openAIWireMessage
	if err := json.Unmarshal(raw, &msgs); err != nil {
		return "", nil, fmt.Errorf("messages: %w", err)
	}
	for _, m := range msgs {
		switch m.Role {
		case "system", "developer":
			blocks, err := contentBlocks(m.Content)
			if err != nil {
				return "", nil, err
			}
			for _, b := range blocks {
				if t, ok := b["text"].(string); ok {
					system = append(system, t)
				}
			}
		case "tool":
			text := contentText(m.Content)
			add("user", map[string]any{"type": "tool_result", "tool_use_id": m.ToolCallID, "content": text})
		case "assistant":
			blocks, err := contentBlocks(m.Content)
			if err != nil {
				return "", nil, err
			}
			for _, tc := range m.ToolCalls {
				input := json.RawMessage(tc.Function.Arguments)
				if len(strings.TrimSpace(tc.Function.Arguments)) == 0 || !json.Valid(input) {
					input = json.RawMessage(`{}`)
				}
				blocks = append(blocks, map[string]any{"type": "tool_use", "id": tc.ID, "name": tc.Function.Name, "input": input})
			}
			add("assistant", blocks...)
		default:
			blocks, err := contentBlocks(m.Content)
			if err != nil {
				return "", nil, err
			}
			add("user", blocks...)
		}
	}
	return strings.Join(system, "\n\n"), turns, nil
}

func textBlock(s string) []map[string]any {
	if s == "" {
		return nil
	}
	return []map[string]any{{"type": "text", "text": s}}
}

// contentText flattens an OpenAI content value (a string or parts) to text.
func contentText(raw json.RawMessage) string {
	blocks, _ := contentBlocks(raw)
	var b strings.Builder
	for _, bl := range blocks {
		if t, ok := bl["text"].(string); ok {
			b.WriteString(t)
		}
	}
	return b.String()
}

// contentBlocks converts an OpenAI content value — a string, null, or an array
// of text and image_url parts — into Anthropic content blocks.
func contentBlocks(raw json.RawMessage) ([]map[string]any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return textBlock(s), nil
	}
	var parts []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, fmt.Errorf("message content: %w", err)
	}
	var out []map[string]any
	for _, p := range parts {
		switch p.Type {
		case "text":
			out = append(out, textBlock(p.Text)...)
		case "image_url":
			out = append(out, imageBlock(p.ImageURL.URL))
		}
	}
	return out, nil
}

// imageBlock turns an image_url (a data: URL or a remote one) into an
// Anthropic image block.
func imageBlock(u string) map[string]any {
	if rest, ok := strings.CutPrefix(u, "data:"); ok {
		if meta, data, found := strings.Cut(rest, ","); found {
			media := strings.TrimSuffix(meta, ";base64")
			return map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": media, "data": data}}
		}
	}
	return map[string]any{"type": "image", "source": map[string]any{"type": "url", "url": u}}
}

// addAnthropicTools maps OpenAI tools, tool_choice and parallel_tool_calls.
func addAnthropicTools(req ChatRequest, payload map[string]any) error {
	raw := req.Raw["tools"]
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var tools []struct {
		Type     string `json:"type"`
		Function struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Parameters  json.RawMessage `json:"parameters"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &tools); err != nil {
		return fmt.Errorf("tools: %w", err)
	}
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		if t.Type != "" && t.Type != "function" {
			continue
		}
		schema := t.Function.Parameters
		if len(schema) == 0 || string(schema) == "null" {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		tool := map[string]any{"name": t.Function.Name, "input_schema": schema}
		if t.Function.Description != "" {
			tool["description"] = t.Function.Description
		}
		out = append(out, tool)
	}
	if len(out) == 0 {
		return nil
	}
	payload["tools"] = out

	choice := map[string]any{"type": "auto"}
	if rc := req.Raw["tool_choice"]; len(rc) > 0 && string(rc) != "null" {
		var s string
		var named struct {
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		}
		switch {
		case json.Unmarshal(rc, &s) == nil:
			switch s {
			case "none":
				choice = map[string]any{"type": "none"}
			case "required":
				choice = map[string]any{"type": "any"}
			}
		case json.Unmarshal(rc, &named) == nil && named.Function.Name != "":
			choice = map[string]any{"type": "tool", "name": named.Function.Name}
		}
	}
	// The current Claude generation refuses forced tool use with a 400. The
	// call still offers the tools; the model decides, which is what a prompt
	// asking for the tool gets anyway, and far better than every such request
	// failing on these rungs.
	if req.ParamStyle == ParamStyleReasoning && (choice["type"] == "any" || choice["type"] == "tool") {
		choice = map[string]any{"type": "auto"}
	}
	if pc := req.Raw["parallel_tool_calls"]; string(pc) == "false" && choice["type"] != "none" {
		choice["disable_parallel_tool_use"] = true
	}
	payload["tool_choice"] = choice
	return nil
}

// addAnthropicFormat maps response_format json_schema onto Anthropic's
// structured outputs, so with_structured_output and friends get JSON back.
func addAnthropicFormat(req ChatRequest, payload map[string]any) {
	raw := req.Raw["response_format"]
	if len(raw) == 0 {
		return
	}
	var rf struct {
		Type       string `json:"type"`
		JSONSchema struct {
			Schema json.RawMessage `json:"schema"`
		} `json:"json_schema"`
	}
	if json.Unmarshal(raw, &rf) != nil || rf.Type != "json_schema" || len(rf.JSONSchema.Schema) == 0 {
		return
	}
	payload["output_config"] = map[string]any{"format": map[string]any{"type": "json_schema", "schema": rf.JSONSchema.Schema}}
}

// anthropicBlock is one content block of an Anthropic response.
type anthropicBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// openAIToolCalls renders Anthropic tool_use blocks as an OpenAI tool_calls
// array, or nil when there are none.
func openAIToolCalls(blocks []anthropicBlock) json.RawMessage {
	var calls []map[string]any
	for _, b := range blocks {
		if b.Type != "tool_use" {
			continue
		}
		args := string(b.Input)
		if args == "" || args == "null" {
			args = "{}"
		}
		calls = append(calls, map[string]any{
			"id": b.ID, "type": "function",
			"function": map[string]any{"name": b.Name, "arguments": args},
		})
	}
	if len(calls) == 0 {
		return nil
	}
	out, _ := json.Marshal(calls)
	return out
}
