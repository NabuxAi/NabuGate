package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"unicode"
)

// The local decision engine — the "build your own decision model" trick: a
// multiple-choice question is answered in ONE forward pass by asking the model
// for a letter and reading the logprobs of the option tokens, then softmaxing
// over just those tokens (constrained decoding). Raw token probabilities are
// overconfident, so a calibration temperature (fitted elsewhere, see
// decision_calibration.go) can be applied to the logits before that softmax.
//
// It works with any OpenAI-wire provider that returns top_logprobs, which means
// every chat alias the gateway already routes to becomes a decision model —
// no Jev subscription required.

// LocalDecisionAdapter is implemented by adapters that can evaluate a
// structured decision with a plain chat model (single forward pass per
// question over the option tokens' logprobs).
type LocalDecisionAdapter interface {
	DecideLocal(ctx context.Context, req DecisionRequest) (DecisionResponse, error)
}

// decisionTemperature is the calibration temperature applied to option logits
// before the constrained softmax. Values > 1 flatten the distribution, which is
// what an overconfident model needs (the article's fit landed near 3.8).
const decisionEpsilon = 1e-9

// letterFor returns the option letter for index i (A, B, C …). More than 26
// options is a questionnaire, not a decision.
func letterFor(i int) (rune, bool) {
	if i < 0 || i >= 26 {
		return 0, false
	}
	return rune('A' + i), true
}

// firstLetterOf normalises a generated token to a bare A–Z letter: models and
// tokenizers emit "A", " A", "**A", "B:" or lowercase variants of the same
// choice. A token whose letter starts a word ("the", "Because") is prose, not
// an option, and must not be read as T or B.
func firstLetterOf(tok string) (rune, bool) {
	runes := []rune(strings.TrimSpace(tok))
	i := 0
	for i < len(runes) && !unicode.IsLetter(runes[i]) && !unicode.IsDigit(runes[i]) {
		i++
	}
	if i >= len(runes) {
		return 0, false
	}
	r := runes[i]
	if r >= 'a' && r <= 'z' {
		r = r - 'a' + 'A'
	}
	if r < 'A' || r > 'Z' {
		return 0, false
	}
	if i+1 < len(runes) && unicode.IsLetter(runes[i+1]) {
		return 0, false
	}
	return r, true
}

// decisionQuestion is one System One question: a type (choice / noul / score),
// instructions and the criteria that enumerate the options.
type decisionQuestion struct {
	Type          string          `json:"type"`
	Instructions  string          `json:"instructions"`
	Criteria      json.RawMessage `json:"criteria"`
	OptionsFormat string          `json:"options_format"`
}

// engineOption is one lettered option of a question.
type engineOption struct {
	Key  string // answer key ("spam", "true", "2" for score levels…)
	Desc string
}

// engineQuestion is a decisionQuestion resolved to an ordered option list.
type engineQuestion struct {
	ID    string
	Type  string
	Opts  []engineOption
	Inst  string
	Valid bool
	Err   string
}

// resolveQuestion turns the wire shape into lettered options. Deterministic
// order matters: the same question must always get the same letters, and a map
// in Go iterates in random order, so criteria keys are sorted.
func resolveQuestion(id string, q decisionQuestion) engineQuestion {
	eq := engineQuestion{ID: id, Type: q.Type, Inst: q.Instructions}
	switch q.Type {
	case "choice":
		var crit map[string]string
		if err := json.Unmarshal(q.Criteria, &crit); err != nil || len(crit) == 0 {
			eq.Err = "choice question needs criteria as a non-empty object of option → description"
			return eq
		}
		for _, k := range sortedKeys(crit) {
			eq.Opts = append(eq.Opts, engineOption{Key: k, Desc: crit[k]})
		}
	case "noul":
		// Binary by definition. Custom descriptions are honored, extra keys ignored.
		crit := map[string]string{}
		if len(q.Criteria) > 0 {
			_ = json.Unmarshal(q.Criteria, &crit)
		}
		t, f := crit["true"], crit["false"]
		if t == "" {
			t = "Condition is satisfied, affirmed, or true"
		}
		if f == "" {
			f = "Condition is not satisfied, denied, or false"
		}
		eq.Opts = []engineOption{{Key: "true", Desc: t}, {Key: "false", Desc: f}}
	case "score":
		var crit []string
		if err := json.Unmarshal(q.Criteria, &crit); err != nil || len(crit) < 2 {
			eq.Err = "score question needs criteria as an array of at least two levels"
			return eq
		}
		for i, lvl := range crit {
			eq.Opts = append(eq.Opts, engineOption{Key: fmt.Sprintf("%d", i), Desc: lvl})
		}
	default:
		eq.Err = fmt.Sprintf("unsupported question type %q (use choice, noul or score)", q.Type)
		return eq
	}
	if len(eq.Opts) > 26 {
		eq.Err = "too many options (max 26)"
		return eq
	}
	eq.Valid = true
	return eq
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// stateString renders the decision state for the prompt. A string state goes
// in as-is; anything else (object, array…) goes in as indented JSON so the
// model sees the structure it was sent.
func stateString(state json.RawMessage) string {
	s := strings.TrimSpace(string(state))
	if len(s) == 0 {
		return ""
	}
	if s[0] == '"' {
		var str string
		if err := json.Unmarshal(state, &str); err == nil {
			return str
		}
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, state, "", "  "); err == nil {
		return pretty.String()
	}
	return s
}

// renderQuestion builds the single question the model answers with one letter.
func renderQuestion(state string, q engineQuestion) string {
	var sb strings.Builder
	if state != "" {
		sb.WriteString("State:\n")
		sb.WriteString(state)
		sb.WriteString("\n\n")
	}
	if q.Inst != "" {
		sb.WriteString("Question: ")
		sb.WriteString(q.Inst)
		sb.WriteString("\n\n")
	}
	sb.WriteString("Options:\n")
	for i, opt := range q.Opts {
		letter, _ := letterFor(i)
		if opt.Desc != "" {
			fmt.Fprintf(&sb, "%c. %s — %s\n", letter, opt.Key, opt.Desc)
		} else {
			fmt.Fprintf(&sb, "%c. %s\n", letter, opt.Key)
		}
	}
	sb.WriteString("\nAnswer with the letter of the best option only. No explanation, no other text.")
	return sb.String()
}

// chatLogprobResponse is the slice of the OpenAI chat wire format the engine
// needs: the top logprobs of the first generated token.
type chatLogprobResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Logprobs *struct {
			Content []struct {
				TopLogprobs []struct {
					Token   string  `json:"token"`
					Logprob float64 `json:"logprob"`
				} `json:"top_logprobs"`
			} `json:"content"`
		} `json:"logprobs"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// DecideLocal implements the decision engine on any OpenAI-wire endpoint that
// returns top_logprobs: one forward pass per question, letters only.
func (a *OpenAIAdapter) DecideLocal(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	var questions map[string]decisionQuestion
	if err := json.Unmarshal(req.Questions, &questions); err != nil || len(questions) == 0 {
		return DecisionResponse{}, fmt.Errorf("%s: questions must be a non-empty object of question definitions", a.name)
	}
	if len(questions) > 20 {
		return DecisionResponse{}, fmt.Errorf("%s: too many questions in one decision (max 20)", a.name)
	}

	resolved := make([]engineQuestion, 0, len(questions))
	for _, id := range sortedQuestionIDs(questions) {
		resolved = append(resolved, resolveQuestion(id, questions[id]))
	}

	state := stateString(req.State)
	temperature := 1.0
	if req.Temperature != nil && *req.Temperature > 0 {
		temperature = *req.Temperature
	}

	answers := make(map[string]any, len(resolved))
	var usage Usage
	var failure error

	for _, q := range resolved {
		if !q.Valid {
			answers[q.ID] = map[string]any{"type": q.Type, "error": q.Err}
			continue
		}
		probs, promptTokens, completionTokens, err := a.optionProbabilities(ctx, req.Model, renderQuestion(state, q), len(q.Opts), temperature)
		usage.PromptTokens += promptTokens
		usage.CompletionTokens += completionTokens
		if err != nil {
			// One unanswerable question must not throw away the rest.
			answers[q.ID] = map[string]any{"type": q.Type, "error": err.Error()}
			failure = err
			continue
		}

		raw := make(map[string]float64, len(q.Opts))
		bestKey, bestP := "", -1.0
		for i, opt := range q.Opts {
			p := probs[i]
			raw[opt.Key] = p
			if p > bestP {
				bestP, bestKey = p, opt.Key
			}
		}
		answer := map[string]any{
			"type":          q.Type,
			"probabilities": raw,
			"confidence":    bestP,
		}
		if temperature != 1.0 {
			answer["temperature"] = temperature
		}
		switch q.Type {
		case "choice":
			answer["choice"] = bestKey
		case "noul":
			answer["noul"] = raw["true"]
		case "score":
			legend := map[string]string{}
			score := 0.0
			for i, opt := range q.Opts {
				legend[opt.Key] = opt.Desc
				idx := float64(i)
				score += raw[opt.Key] * idx
			}
			answer["score"] = score
			answer["legend"] = legend
		}
		answers[q.ID] = answer
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens

	raw, _ := json.Marshal(answers)
	resp := DecisionResponse{Model: req.Model, Answers: raw, Usage: usage}
	// Every question failed: surface the underlying error rather than a page
	// of per-question error objects.
	if failure != nil && len(answers) > 0 && allFailed(answers) {
		return resp, failure
	}
	return resp, nil
}

func allFailed(answers map[string]any) bool {
	for _, v := range answers {
		if m, ok := v.(map[string]any); ok {
			if _, hasErr := m["error"]; !hasErr {
				return false
			}
		}
	}
	return true
}

func sortedQuestionIDs(qs map[string]decisionQuestion) []string {
	ids := make([]string, 0, len(qs))
	for id := range qs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// optionProbabilities runs the single forward pass and returns the constrained
// softmax over the option letters — the renormalised probabilities of only the
// A–N… tokens, with the calibration temperature applied to the logits first.
func (a *OpenAIAdapter) optionProbabilities(ctx context.Context, model, prompt string, numOptions int, temperature float64) ([]float64, int, int, error) {
	body := map[string]any{
		"model":        model,
		"temperature":  0,
		"max_tokens":   1,
		"logprobs":     true,
		"top_logprobs": 20,
		"messages": []map[string]string{
			{"role": "system", "content": "You are a decision model. You answer multiple-choice questions with a single letter and nothing else."},
			{"role": "user", "content": prompt},
		},
	}
	raw, status, err := a.postJSON(ctx, "/chat/completions", body)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("%s: decision pass failed: %w", a.name, err)
	}
	var parsed chatLogprobResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, 0, 0, fmt.Errorf("%s: invalid chat response (status %d): %s", a.name, status, truncate(raw))
	}
	if status >= 400 {
		msg := http.StatusText(status)
		if parsed.Error != nil && parsed.Error.Message != "" {
			msg = parsed.Error.Message
		}
		return nil, 0, 0, fmt.Errorf("%s: upstream error (status %d): %s", a.name, status, msg)
	}
	if len(parsed.Choices) == 0 || parsed.Choices[0].Logprobs == nil || len(parsed.Choices[0].Logprobs.Content) == 0 {
		return nil, 0, 0, fmt.Errorf("%s: no logprobs returned (provider must support logprobs=true)", a.name)
	}

	// Best logprob seen per option index; a letter may appear as several token
	// variants ("A" and " A") and the top-k cutoff may drop it entirely.
	optLogprob := make([]float64, numOptions)
	found := make([]bool, numOptions)
	for _, lp := range parsed.Choices[0].Logprobs.Content[0].TopLogprobs {
		letter, ok := firstLetterOf(lp.Token)
		if !ok {
			continue
		}
		idx := int(letter - 'A')
		if idx >= numOptions {
			continue
		}
		if !found[idx] || lp.Logprob > optLogprob[idx] {
			optLogprob[idx] = lp.Logprob
			found[idx] = true
		}
	}
	if noneFound(found) {
		return nil, 0, 0, fmt.Errorf("%s: model did not emit any option letter in its top tokens", a.name)
	}

	// Constrained softmax over the option tokens only, with the calibration
	// temperature on the logits (T>1 flattens an overconfident distribution).
	probs := make([]float64, numOptions)
	var sum float64
	for i := range probs {
		if found[i] {
			probs[i] = math.Exp(optLogprob[i] / temperature)
			sum += probs[i]
		}
	}
	for i := range probs {
		probs[i] /= sum
	}
	return probs, parsed.Usage.PromptTokens, parsed.Usage.CompletionTokens, nil
}

func noneFound(found []bool) bool {
	for _, f := range found {
		if f {
			return false
		}
	}
	return true
}
