package provider

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// defaultDecisionQuestions defines standard System One questions when chatting
// in free-form natural language with a decision model.
var defaultDecisionQuestions = map[string]any{
	"intent": map[string]any{
		"type":         "choice",
		"instructions": "What is the primary intent, topic, or category of this input?",
		"criteria": map[string]any{
			"inquiry_or_analysis": "Analysis, question, classification, or evaluation request",
			"security_guardrail":  "Checking safety, jailbreak, prompt injection, or malicious payload",
			"operational_action":  "Requesting an operational action, transaction, or workflow step",
			"general_chat":        "General conversational or informational message",
		},
	},
	"verdict": map[string]any{
		"type":         "noul",
		"instructions": "Based on the input context and criteria, is the primary premise, condition, or question satisfied/true?",
		"criteria": map[string]any{
			"true":  "Condition is satisfied, affirmed, or true",
			"false": "Condition is not satisfied, denied, or false",
		},
	},
	"severity": map[string]any{
		"type":         "score",
		"instructions": "What is the severity, priority, or urgency level of this request?",
		"criteria":     []string{"Low", "Medium", "High", "Critical"},
	},
}

// buildDecisionRequestFromChat extracts state and questions from chat messages.
func buildDecisionRequestFromChat(model string, req ChatRequest) (DecisionRequest, string) {
	var userPrompt string
	var contextParts []string

	for _, m := range req.Messages {
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		if m.Role == "user" {
			userPrompt = content
		}
		contextParts = append(contextParts, fmt.Sprintf("[%s]: %s", m.Role, content))
	}

	stateText := strings.Join(contextParts, "\n\n")
	if stateText == "" {
		stateText = userPrompt
	}

	// If the user supplied a JSON body specifying "questions" and/or "state", honor it.
	var customBody struct {
		State     any            `json:"state"`
		Questions map[string]any `json:"questions"`
	}
	if err := json.Unmarshal([]byte(userPrompt), &customBody); err == nil && len(customBody.Questions) > 0 {
		var stateRaw json.RawMessage
		if customBody.State != nil {
			stateRaw, _ = json.Marshal(customBody.State)
		} else {
			stateRaw, _ = json.Marshal(stateText)
		}
		questionsRaw, _ := json.Marshal(customBody.Questions)
		return DecisionRequest{
			Model:     model,
			State:     stateRaw,
			Questions: questionsRaw,
		}, userPrompt
	}

	// Free-form natural language chat: evaluate standard System One primitives.
	stateRaw, _ := json.Marshal(stateText)
	questionsRaw, _ := json.Marshal(defaultDecisionQuestions)
	return DecisionRequest{
		Model:     model,
		State:     stateRaw,
		Questions: questionsRaw,
	}, userPrompt
}

// formatDecisionChatResponse converts a DecisionResponse into an OpenAI ChatResponse.
func formatDecisionChatResponse(decResp DecisionResponse, originalPrompt string) ChatResponse {
	var answers map[string]struct {
		Type          string             `json:"type"`
		Noul          *float64           `json:"noul"`
		Choice        *string            `json:"choice"`
		Probabilities map[string]float64 `json:"probabilities"`
		Confidence    *float64           `json:"confidence"`
		Score         *float64           `json:"score"`
		Legend        map[string]string  `json:"legend"`
	}
	_ = json.Unmarshal(decResp.Answers, &answers)

	var sb strings.Builder
	sb.WriteString("### 🧠 TypeSafe Jev (System 1 Decision Engine)\n")
	sb.WriteString(fmt.Sprintf("**Model:** `%s`\n\n", decResp.Model))
	sb.WriteString("**نتیجه تحلیل و ارزیابی ساختاریافته (Decision Verdict):**\n\n")

	if len(answers) > 0 {
		sb.WriteString("| شناسه پرسش | نوع | مقدار منتخب / ارزیابی | ضریب اطمینان (Confidence) |\n")
		sb.WriteString("| :--- | :--- | :--- | :--- |\n")

		keys := make([]string, 0, len(answers))
		for k := range answers {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			a := answers[k]
			switch a.Type {
			case "noul":
				val := 0.0
				if a.Noul != nil {
					val = *a.Noul
				}
				verdict := "خیر (False)"
				if val >= 0.5 {
					verdict = fmt.Sprintf("بله (True: %.1f%%)", val*100)
				} else {
					verdict = fmt.Sprintf("خیر (False: %.1f%%)", (1.0-val)*100)
				}
				sb.WriteString(fmt.Sprintf("| `%s` | Noul (بله/خیر) | %s | احتمال: `%.2f` |\n", k, verdict, val))

			case "choice":
				ch := "-"
				if a.Choice != nil {
					ch = fmt.Sprintf("`%s`", *a.Choice)
				}
				conf := "-"
				if a.Confidence != nil {
					conf = fmt.Sprintf("%.1f%%", *a.Confidence*100)
				}
				sb.WriteString(fmt.Sprintf("| `%s` | Choice (دسته‌بندی) | %s | %s |\n", k, ch, conf))

			case "score":
				sc := 0.0
				if a.Score != nil {
					sc = *a.Score
				}
				conf := "-"
				if a.Confidence != nil {
					conf = fmt.Sprintf("%.1f%%", *a.Confidence*100)
				}
				levelDesc := ""
				idxStr := fmt.Sprintf("%d", int(sc+0.5))
				if desc, ok := a.Legend[idxStr]; ok {
					levelDesc = fmt.Sprintf(" (%s)", desc)
				}
				sb.WriteString(fmt.Sprintf("| `%s` | Score (امتیاز) | `%.2f`%s | %s |\n", k, sc, levelDesc, conf))

			default:
				sb.WriteString(fmt.Sprintf("| `%s` | %s | انجام شد | - |\n", k, a.Type))
			}
		}
	}

	sb.WriteString("\n---\n")
	sb.WriteString("#### 📊 خروجی خام ساختاریافته (JSON Payload):\n")
	sb.WriteString("```json\n")
	var pretty json.RawMessage
	if b, err := json.MarshalIndent(decResp.Answers, "", "  "); err == nil {
		pretty = b
	} else {
		pretty = decResp.Answers
	}
	sb.WriteString(string(pretty))
	sb.WriteString("\n```\n")

	content := sb.String()

	// Calculate tokens accurately.
	pTokens := decResp.Usage.PromptTokens
	if pTokens <= 0 {
		pTokens = (len(originalPrompt) + 3) / 4
		if pTokens < 1 {
			pTokens = 1
		}
	}

	cTokens := decResp.Usage.CompletionTokens
	if cTokens <= 0 {
		cTokens = (len(content) + 3) / 4
		if cTokens < 1 {
			cTokens = 1
		}
	}

	usage := Usage{
		PromptTokens:     pTokens,
		CompletionTokens: cTokens,
		TotalTokens:      pTokens + cTokens,
	}

	return ChatResponse{
		Content:      content,
		FinishReason: "stop",
		Usage:        usage,
	}
}

// streamTextChunks sends content in progressive lines/chunks to the delta callback.
func streamTextChunks(content string, onDelta DeltaFunc) error {
	lines := strings.SplitAfter(content, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		if err := onDelta(line); err != nil {
			return err
		}
	}
	return nil
}
