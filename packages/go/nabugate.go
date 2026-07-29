package nabugate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	BaseURL      string
	APIKey       string
	DefaultModel string
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
}

type ChatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

type Client struct {
	config Config
	http   *http.Client
}

func NewClient(apiKey string, opts ...func(*Config)) *Client {
	cfg := Config{
		BaseURL:      "https://gate.nabuxai.com/v1",
		APIKey:       apiKey,
		DefaultModel: "nabu-smart",
	}
	for _, fn := range opts {
		fn(&cfg)
	}

	return &Client{
		config: cfg,
		http: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (c *Client) Chat(messages []Message, model string, temp float64) (*ChatResponse, error) {
	if model == "" {
		model = c.config.DefaultModel
	}
	if temp == 0 {
		temp = 0.7
	}

	reqBody, err := json.Marshal(ChatRequest{
		Model:       model,
		Messages:    messages,
		Temperature: temp,
	})
	if err != nil {
		return nil, err
	}

	url := strings.TrimRight(c.config.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("NabuGate request failed (%d): %s", resp.StatusCode, string(body))
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, err
	}

	return &chatResp, nil
}

func (c *Client) CompleteText(messages []Message, model string, temp float64) (string, error) {
	resp, err := c.Chat(messages, model, temp)
	if err != nil {
		return "", err
	}
	if len(resp.Choices) > 0 {
		return strings.TrimSpace(resp.Choices[0].Message.Content), nil
	}
	return "", nil
}
