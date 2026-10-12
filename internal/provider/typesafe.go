package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// TypeSafeAdapter communicates with TypeSafe AI's System One API and wire-
// compatible endpoints (typesafe.ai, v1m.ir — the latter rides this adapter
// with its own base_url the way arvan/9router ride the openai one).
// It evaluates structured decisions (Noul, Choice, Score) for Jev models.
type TypeSafeAdapter struct {
	name    string
	baseURL string
	apiKey  string
}

// NewTypeSafeAdapter creates an adapter for TypeSafe AI.
func NewTypeSafeAdapter(name, baseURL, apiKey string) *TypeSafeAdapter {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.typesafe.ai/v1"
	}
	return &TypeSafeAdapter{
		name:    name,
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
	}
}

func (a *TypeSafeAdapter) Name() string { return a.name }

// Chat evaluates the input messages using Jev System One and formats a comprehensive,
// human-readable chat completion response with full token accounting.
func (a *TypeSafeAdapter) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	decReq, originalPrompt := buildDecisionRequestFromChat(req.Model, req)
	decResp, err := a.Decide(ctx, decReq)
	if err != nil {
		return ChatResponse{}, err
	}
	return formatDecisionChatResponse(decReq, decResp, originalPrompt), nil
}

// ChatStream streams the evaluated decision in progressive chunks and reports token accounting.
func (a *TypeSafeAdapter) ChatStream(ctx context.Context, req ChatRequest, onDelta DeltaFunc) (Usage, error) {
	resp, err := a.Chat(ctx, req)
	if err != nil {
		return Usage{}, err
	}
	if err := streamTextChunks(resp.Content, onDelta); err != nil {
		return resp.Usage, err
	}
	return resp.Usage, nil
}

type typeSafeResponse struct {
	Model   string          `json:"model"`
	Answers json.RawMessage `json:"answers"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	Message string `json:"message"`
}

// Decide evaluates a state against a set of typed questions.
func (a *TypeSafeAdapter) Decide(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	bodyMap := map[string]any{
		"model":     req.Model,
		"state":     req.State,
		"questions": req.Questions,
	}
	data, err := json.Marshal(bodyMap)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("failed to encode request body: %w", err)
	}

	url := a.baseURL
	if !strings.HasSuffix(url, "/systemone") && !strings.HasSuffix(url, "/decisions") {
		url += "/systemone"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if a.apiKey != "" && a.apiKey != "-" {
		httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	}

	resp, err := sharedHTTPClient.Do(httpReq)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("typesafe request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return DecisionResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp typeSafeResponse
		if json.Unmarshal(respBytes, &errResp) == nil {
			if errResp.Error != nil && errResp.Error.Message != "" {
				return DecisionResponse{}, fmt.Errorf("typesafe error (%d): %s", resp.StatusCode, errResp.Error.Message)
			}
			if errResp.Message != "" {
				return DecisionResponse{}, fmt.Errorf("typesafe error (%d): %s", resp.StatusCode, errResp.Message)
			}
			// v1m carries the message under "detail" instead of the two shapes
			// above, so the raw body would surface a JSON blob, not a reason.
			if msg := detailErrorMessage(respBytes); msg != "" {
				return DecisionResponse{}, fmt.Errorf("typesafe error (%d): %s", resp.StatusCode, msg)
			}
		}
		return DecisionResponse{}, fmt.Errorf("typesafe error (%d): %s", resp.StatusCode, string(respBytes))
	}

	var parsed typeSafeResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return DecisionResponse{}, fmt.Errorf("failed to parse typesafe response: %w", err)
	}

	return DecisionResponse{
		Model:   parsed.Model,
		Answers: parsed.Answers,
		Usage: Usage{
			PromptTokens:     parsed.Usage.InputTokens,
			CompletionTokens: parsed.Usage.OutputTokens,
			TotalTokens:      parsed.Usage.InputTokens + parsed.Usage.OutputTokens,
		},
	}, nil
}

// detailErrorMessage extracts the human-readable message from the error
// shapes that carry it under "detail" rather than "error"/"message": the v1m
// endpoint wraps runtime errors as {"detail":{"error":{"message":…}}} and
// answers invalid requests with FastAPI's validation shape
// {"detail":[{"type":…,"msg":…,"loc":[…]},…]} (the first non-empty msg is the
// one worth reading). Any other body returns "" so the caller falls back to
// the raw body rather than guessing.
func detailErrorMessage(body []byte) string {
	var d struct {
		Detail json.RawMessage `json:"detail"`
	}
	if err := json.Unmarshal(body, &d); err != nil || len(d.Detail) == 0 {
		return ""
	}
	var wrapped struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(d.Detail, &wrapped); err == nil && wrapped.Error.Message != "" {
		return wrapped.Error.Message
	}
	var items []struct {
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(d.Detail, &items); err == nil {
		for _, it := range items {
			if it.Msg != "" {
				return it.Msg
			}
		}
	}
	return ""
}

// ListModels implements ModelLister for TypeSafe.
func (a *TypeSafeAdapter) ListModels(ctx context.Context) ([]string, error) {
	url := a.baseURL + "/models"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if a.apiKey != "" && a.apiKey != "-" {
		httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	}
	resp, err := sharedHTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Fallback to standard known models if /v1/models fails
		return []string{"jev-latest", "jev-1.13.0", "jev-preview"}, nil
	}

	var data struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return []string{"jev-latest", "jev-1.13.0", "jev-preview"}, nil
	}

	var names []string
	for _, m := range data.Models {
		if m.Name != "" {
			names = append(names, m.Name)
		}
	}
	for _, m := range data.Data {
		if m.ID != "" {
			names = append(names, m.ID)
		}
	}
	if len(names) == 0 {
		names = []string{"jev-latest", "jev-1.13.0", "jev-preview"}
	}
	return names, nil
}
