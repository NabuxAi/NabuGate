package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"nabugate/internal/agent"
	"nabugate/internal/policy"
	"nabugate/internal/provider"
)

// Agent tool calling.
//
// An agent whose YAML declares `tools:` gets a server-side tool-call loop:
// the gateway offers the model the declared functions, executes the calls it
// comes back with (HTTP tools, run by agent.ToolExecutor), appends the results
// as role:tool messages, and calls the provider again — until the model
// answers in prose or the step cap forces it to. The caller made one ordinary
// chat request and gets one ordinary answer; the tool traffic never leaves
// the gateway.
//
// Precedence rule: a caller that sends its OWN `tools` array knows what it is
// doing, so the gateway injects nothing and runs no loop — the request passes
// through untouched, exactly as before agent tools existed. Client tools win;
// agent tools apply only to requests that did not bring their own.

// clientSuppliesTools reports whether the raw request carries its own tools.
func clientSuppliesTools(raw map[string]json.RawMessage) bool {
	return len(raw["tools"]) > 0 && string(raw["tools"]) != "null" && string(raw["tools"]) != "[]"
}

// toolLoopOutcome is what the loop settled on: the final prose answer, the
// provider that wrote it, and the usage summed over every round-trip (each
// step re-sends the growing conversation, so the tokens are all real).
type toolLoopOutcome struct {
	content      string
	finishReason string
	provider     string
	model        string
	usage        provider.Usage
	steps        int // provider round-trips
	toolCalls    int // tool executions across all steps
}

// wireToolCall is one entry of an OpenAI "tool_calls" array.
type wireToolCall struct {
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // JSON-encoded object
	} `json:"function"`
}

// completeAgentTools runs the tool-call loop for an agent request and writes
// the response, streaming-shaped when the caller asked for a stream. It is the
// handleChat branch for "agent with tools, caller brought none of its own".
func (s *Server) completeAgentTools(w http.ResponseWriter, r *http.Request, alias string, ag agent.Agent, routeModel string, req provider.ChatRequest, convID string, newTurns []provider.Message, stream bool) {
	// Every target the request might land on must carry tools: falling back
	// onto a provider that cannot would silently turn a tool agent into a
	// plain one, and "the model ignored its tools" is a miserable bug to
	// chase from the caller's side. Refuse loudly instead.
	if bad, resolved := s.router.ToolUncapableTargets(routeModel); resolved && len(bad) > 0 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf(
			"agent %q declares tools, but provider(s) %s behind model %q do not speak the OpenAI tool wire format — point the agent's `model` at an OpenAI-compatible provider (Anthropic and Gemini routes cannot carry agent tools)",
			ag.Name, strings.Join(bad, ", "), routeModel))
		return
	}

	outcome, err := s.runToolLoop(r.Context(), ag, routeModel, req)
	if err != nil {
		writeError(w, aliasErrStatus(err, "unknown model alias"), err.Error())
		return
	}

	s.record(r, outcome.provider, outcome.model, outcome.usage)
	s.saveTurn(r, convID, newTurns, outcome.content)

	w.Header().Set("X-Nabu-Provider", outcome.provider)
	w.Header().Set("X-Nabu-Model", outcome.model)
	if outcome.toolCalls > 0 {
		w.Header().Set("X-Nabu-Tool-Calls", fmt.Sprint(outcome.toolCalls))
	}

	if stream {
		s.streamToolOutcome(w, alias, outcome)
		return
	}
	s.writeToolOutcome(w, alias, outcome)
}

// runToolLoop executes the bounded loop. The conversation lives in req.Raw
// (what the OpenAI-wire adapters actually send); each round appends the
// assistant's tool_calls message and one role:tool message per result.
func (s *Server) runToolLoop(ctx context.Context, ag agent.Agent, routeModel string, req provider.ChatRequest) (toolLoopOutcome, error) {
	var outcome toolLoopOutcome

	if req.Raw == nil {
		// Internal callers can arrive Raw-less; rebuild an equivalent body so
		// the loop has somewhere to keep the growing conversation.
		req.Raw = map[string]json.RawMessage{}
		msgs, _ := json.Marshal(req.Messages)
		req.Raw["messages"] = msgs
		if req.Temperature != nil {
			req.Raw["temperature"], _ = json.Marshal(*req.Temperature)
		}
		if req.MaxTokens != nil {
			req.Raw["max_tokens"], _ = json.Marshal(*req.MaxTokens)
		}
	}
	// The loop cannot stream (a tool result is needed before the next token
	// exists), so the upstream calls are always plain completions; a caller
	// that asked for a stream gets the finished answer re-shaped as SSE by
	// streamToolOutcome.
	delete(req.Raw, "stream")
	delete(req.Raw, "stream_options")

	toolsWire, err := json.Marshal(agent.OpenAITools(ag.Tools))
	if err != nil {
		return outcome, err
	}
	req.Raw["tools"] = toolsWire

	byName := make(map[string]agent.Tool, len(ag.Tools))
	for _, t := range ag.Tools {
		byName[t.Name] = t
	}

	steps := ag.MaxSteps()
	for step := 0; ; step++ {
		if step == steps {
			// The model used its whole budget and still wants another tool:
			// take the tools away and make it answer with what it gathered.
			// This last call is not counted as a step.
			delete(req.Raw, "tools")
		}

		result, err := s.router.Chat(ctx, routeModel, req)
		if err != nil {
			return outcome, err
		}
		outcome.steps++
		outcome.provider, outcome.model = result.Provider, result.Model
		outcome.usage.PromptTokens += result.Response.Usage.PromptTokens
		outcome.usage.CompletionTokens += result.Response.Usage.CompletionTokens
		outcome.usage.TotalTokens += result.Response.Usage.TotalTokens

		var calls []wireToolCall
		if len(result.Response.ToolCalls) > 0 {
			_ = json.Unmarshal(result.Response.ToolCalls, &calls)
		}
		if len(calls) == 0 || step >= steps {
			outcome.content = result.Response.Content
			outcome.finishReason = result.Response.FinishReason
			return outcome, nil
		}

		// The provider's contract: an assistant message carrying tool_calls,
		// then one role:tool message per call, before the next model turn.
		appendRawMessage(req.Raw, map[string]any{
			"role":       "assistant",
			"content":    result.Response.Content,
			"tool_calls": result.Response.ToolCalls,
		})
		for _, call := range calls {
			outcome.toolCalls++
			content := s.executeOneTool(ctx, byName, call)
			appendRawMessage(req.Raw, map[string]any{
				"role":         "tool",
				"tool_call_id": call.ID,
				"content":      content,
			})
			s.log.Info("agent tool executed", "agent", ag.Name, "tool", call.Function.Name, "step", step+1)
		}
	}
}

// executeOneTool runs a single tool call and renders the outcome — result or
// failure — as the tool message content. A failure is information the model
// can act on (retry, apologise, answer from general knowledge), so it goes
// back into the conversation rather than failing the whole request; only the
// provider calls themselves can do that.
func (s *Server) executeOneTool(ctx context.Context, byName map[string]agent.Tool, call wireToolCall) string {
	tool, ok := byName[call.Function.Name]
	if !ok {
		return fmt.Sprintf("error: unknown tool %q — call one of the declared functions", call.Function.Name)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return fmt.Sprintf("error: could not parse arguments as a JSON object: %v", err)
	}
	result, err := s.toolExec.Execute(ctx, tool, args)
	if err != nil {
		s.log.Warn("agent tool failed", "tool", tool.Name, "error", err.Error())
		return "error: " + err.Error()
	}
	return result
}

// appendRawMessage appends one message object to the raw body's messages array.
func appendRawMessage(raw map[string]json.RawMessage, msg map[string]any) {
	var msgs []json.RawMessage
	if len(raw["messages"]) > 0 {
		_ = json.Unmarshal(raw["messages"], &msgs)
	}
	encoded, err := json.Marshal(msg)
	if err != nil {
		return
	}
	raw["messages"], _ = json.Marshal(append(msgs, encoded))
}

// writeToolOutcome renders the loop result as an ordinary chat.completion —
// the same shape handleChat produces, so tool-using agents are invisible to
// existing clients.
func (s *Server) writeToolOutcome(w http.ResponseWriter, alias string, outcome toolLoopOutcome) {
	finish := outcome.finishReason
	if finish == "" {
		finish = "stop"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":             "nabu-" + fmt.Sprint(time.Now().UnixNano()),
		"object":         "chat.completion",
		"created":        time.Now().Unix(),
		"model":          alias,
		"provider":       outcome.provider,
		"upstream_model": outcome.model,
		"choices": []map[string]any{{
			"index":         0,
			"finish_reason": finish,
			"message":       map[string]any{"role": "assistant", "content": outcome.content},
		}},
		"usage": map[string]int{
			"prompt_tokens":     outcome.usage.PromptTokens,
			"completion_tokens": outcome.usage.CompletionTokens,
			"total_tokens":      outcome.usage.TotalTokens,
		},
	})
}

// streamToolOutcome re-shapes a finished loop result as an SSE stream: one
// content delta carrying the whole answer. Tool calling needs the full
// exchange before the answer exists, so true token streaming is impossible
// here; a stream-shaped response keeps SSE clients working without a special
// case on their side, at the cost of the answer arriving all at once.
func (s *Server) streamToolOutcome(w http.ResponseWriter, alias string, outcome toolLoopOutcome) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeToolOutcome(w, alias, outcome)
		return
	}
	id := "nabu-" + fmt.Sprint(time.Now().UnixNano())
	created := time.Now().Unix()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	writeSSE := func(v any) {
		payload, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", payload)
		flusher.Flush()
	}
	writeSSE(streamChunk(id, created, alias, outcome.provider, outcome.model, map[string]any{"role": "assistant"}, nil))
	writeSSE(streamChunk(id, created, alias, outcome.provider, outcome.model, map[string]any{"content": outcome.content}, nil))
	finish := outcome.finishReason
	if finish == "" {
		finish = "stop"
	}
	writeSSE(streamChunk(id, created, alias, outcome.provider, outcome.model, map[string]any{}, &finish))
	fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// handleAgents lists the configured sub-agents and — the part a caller cannot
// learn from /v1/models — which tools each one carries. Scoped to the calling
// key's allow-list like every other catalogue endpoint.
func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	pol, hasPol := r.Context().Value(policyCtxKey{}).(policy.Policy)
	scoped := s.policy.Enabled() && hasPol

	type agentInfo struct {
		Name         string   `json:"name"`
		Description  string   `json:"description,omitempty"`
		Model        string   `json:"model"`
		Tools        []string `json:"tools,omitempty"`
		MaxToolSteps int      `json:"max_tool_steps,omitempty"`
	}

	data := make([]agentInfo, 0, s.agents.Len())
	for _, name := range s.agents.Names() {
		if scoped && !pol.Allows(name) {
			continue
		}
		ag, ok := s.agents.Lookup(name)
		if !ok {
			continue
		}
		info := agentInfo{Name: ag.Name, Description: ag.Description, Model: ag.Model}
		if len(ag.Tools) > 0 {
			info.Tools = agent.ToolNames(ag.Tools)
			info.MaxToolSteps = ag.MaxSteps()
		}
		data = append(data, info)
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}
