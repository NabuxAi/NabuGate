package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"nabugate/internal/flow"
	"nabugate/internal/provider"
)

// maxFlowDepth caps how deep a flow may call other flows.
//
// A step may name another flow, which is what "a team built from teams" means.
// Cycles are caught by name before the cap is reached, so the cap is only for
// the honestly deep chain — and a chain deeper than this is a runaway prompt
// bill nobody meant to authorise.
const maxFlowDepth = 4

// stepOutcome is what one link produced, kept so the caller can see the whole
// chain rather than only its last sentence. A flow that returns a good answer
// for the wrong reason is indistinguishable from one that does not unless the
// middle is visible.
type stepOutcome struct {
	Name     string         `json:"name"`
	Agent    string         `json:"agent"`
	Output   string         `json:"output"`
	Provider string         `json:"provider"`
	Model    string         `json:"model"`
	Usage    provider.Usage `json:"usage"`
	Skipped  bool           `json:"skipped,omitempty"`
	Error    string         `json:"error,omitempty"`
}

// flowResult is one whole chain.
type flowResult struct {
	Steps  []stepOutcome
	Output string
	Usage  provider.Usage
}

// lastUserText is what the caller actually asked, for the {{input}} placeholder.
//
// The last user message rather than all of them: a step late in a chain that
// wants to check the draft against the brief means the brief, not the whole
// transcript including its own earlier steps.
func lastUserText(messages []provider.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}

	return ""
}

// runFlow executes every step in order, handing each what the one before it
// produced.
//
// The caller's own messages are passed to the first step unchanged, so a flow
// invoked with a conversation behaves like the agent it replaces. Later steps
// get a single user message built from their template, because a reviewer needs
// the draft, not a replay of everything that led to it — and re-sending the
// history at every rung multiplies the token bill by the number of steps.
func (s *Server) runFlow(ctx context.Context, fl flow.Flow, req provider.ChatRequest, depth int, seen map[string]bool) (flowResult, error) {
	if depth > maxFlowDepth {
		return flowResult{}, fmt.Errorf("flow %q nests deeper than %d levels", fl.Name, maxFlowDepth)
	}
	if seen[fl.Name] {
		// Named rather than counted: "flow x calls itself" is a definition the
		// operator can go and fix; "too deep" is a symptom they would have to
		// diagnose.
		return flowResult{}, fmt.Errorf("flow %q is part of a cycle", fl.Name)
	}

	seen = withSeen(seen, fl.Name)

	input := lastUserText(req.Messages)

	result := flowResult{Steps: make([]stepOutcome, 0, len(fl.Steps))}
	previous := ""

	for i, step := range fl.Steps {
		stepReq := req

		if i > 0 || strings.TrimSpace(step.Input) != "" {
			// Everything after the first step — and the first step too when it
			// has been given a template — talks to its agent in one message.
			stepReq.Messages = []provider.Message{{
				Role:    "user",
				Content: step.Render(previous, input),
			}}
			stepReq.Raw = nil
		}

		outcome, err := s.runStep(ctx, step, stepReq, depth, seen)

		if err != nil {
			if !step.Optional {
				// One member failing ends the chain. A reviewer handed nothing
				// writes a review of nothing, and it reads exactly as
				// confidently as a real one.
				return result, fmt.Errorf("flow %q step %q: %w", fl.Name, step.DisplayName(), err)
			}

			outcome.Skipped = true
			outcome.Error = err.Error()
			result.Steps = append(result.Steps, outcome)

			continue
		}

		result.Steps = append(result.Steps, outcome)
		result.Usage = addUsage(result.Usage, outcome.Usage)
		previous = outcome.Output
	}

	if previous == "" {
		return result, fmt.Errorf("flow %q produced no output", fl.Name)
	}

	result.Output = previous

	return result, nil
}

// runStep resolves one step's target and runs it: another flow, a sub-agent, or
// a plain alias — resolved in that order, matching how handleChat resolves a
// requested model, so a name means the same thing wherever it is written.
func (s *Server) runStep(ctx context.Context, step flow.Step, req provider.ChatRequest, depth int, seen map[string]bool) (stepOutcome, error) {
	outcome := stepOutcome{Name: step.DisplayName(), Agent: step.Agent}

	if nested, ok := s.flows.Lookup(step.Agent); ok {
		inner, err := s.runFlow(ctx, nested, req, depth+1, seen)
		if err != nil {
			return outcome, err
		}

		outcome.Output = inner.Output
		outcome.Usage = inner.Usage
		outcome.Model = nested.Name

		return outcome, nil
	}

	routeModel := step.Agent
	if ag, ok := s.agents.Lookup(step.Agent); ok {
		applyAgentToChat(ag, &req)
		routeModel = ag.Model
	}

	res, err := s.router.Chat(ctx, routeModel, req)
	if err != nil {
		return outcome, err
	}

	outcome.Output = res.Response.Content
	outcome.Provider = res.Provider
	outcome.Model = res.Model
	outcome.Usage = res.Response.Usage

	return outcome, nil
}

// withSeen copies the visited set so sibling branches do not see each other's
// names. Two steps of one flow may legitimately call the same sub-flow.
func withSeen(seen map[string]bool, name string) map[string]bool {
	next := make(map[string]bool, len(seen)+1)
	for k := range seen {
		next[k] = true
	}
	next[name] = true

	return next
}

// completeFlow runs a whole chain and answers with an ordinary chat completion.
//
// Shaped exactly like a single-model reply so every existing client — the ones
// that only know `choices[0].message.content` — works against a flow with no
// change at all. The trace rides along in an extra `flow` field, which those
// clients ignore and a console can read.
func (s *Server) completeFlow(w http.ResponseWriter, r *http.Request, fl flow.Flow, req provider.ChatRequest) {
	result, err := s.runFlow(r.Context(), fl, req, 1, nil)
	if err != nil {
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "cycle") || strings.Contains(err.Error(), "nests deeper") {
			// A flow that calls itself is a definition the operator wrote, not
			// an upstream having a bad minute.
			status = http.StatusBadRequest
		}

		// Whatever did run is still worth recording: a chain that died at step
		// three already spent steps one and two.
		s.recordFlow(r, fl, result)
		writeError(w, status, err.Error())

		return
	}

	s.recordFlow(r, fl, result)

	w.Header().Set("X-Nabu-Flow", fl.Name)
	w.Header().Set("X-Nabu-Flow-Steps", fmt.Sprint(len(result.Steps)))

	writeJSON(w, http.StatusOK, map[string]any{
		"id":      "nabu-" + fmt.Sprint(time.Now().UnixNano()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   fl.Name,
		"choices": []map[string]any{{
			"index":         0,
			"finish_reason": "stop",
			"message":       map[string]any{"role": "assistant", "content": result.Output},
		}},
		"usage": map[string]int{
			"prompt_tokens":     result.Usage.PromptTokens,
			"completion_tokens": result.Usage.CompletionTokens,
			"total_tokens":      result.Usage.TotalTokens,
		},
		"flow": map[string]any{"name": fl.Name, "steps": result.Steps},
	})
}

// recordFlow bills every step that actually called a provider.
//
// Per step rather than once for the flow: the whole reason to run four models
// instead of one is that they are different models, and a bill that hid which
// of them the money went to would make the choice unreviewable.
func (s *Server) recordFlow(r *http.Request, fl flow.Flow, result flowResult) {
	for _, step := range result.Steps {
		if step.Provider == "" {
			continue
		}
		s.record(r, step.Provider, step.Model, step.Usage)
	}
	_ = fl
}

func addUsage(a, b provider.Usage) provider.Usage {
	return provider.Usage{
		PromptTokens:     a.PromptTokens + b.PromptTokens,
		CompletionTokens: a.CompletionTokens + b.CompletionTokens,
		TotalTokens:      a.TotalTokens + b.TotalTokens,
	}
}
