package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultOpenAITimeout = 120 * time.Second

type chatCompletionsRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionsResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// OpenAIClient is a thin OpenAI-compatible chat completions adapter.
// It works with the native OpenAI API and compatible gateways that expose
// /v1/chat/completions semantics.
type OpenAIClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewOpenAIClient returns an OpenAI-compatible client with sane defaults.
func NewOpenAIClient(baseURL, apiKey, model string) *OpenAIClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}

	return &OpenAIClient{
		baseURL: baseURL,
		apiKey:  strings.TrimSpace(apiKey),
		model:   strings.TrimSpace(model),
		httpClient: &http.Client{
			Timeout: defaultOpenAITimeout,
		},
	}
}

// Generate sends a single-user-message chat completion request.
func (c *OpenAIClient) Generate(ctx context.Context, prompt string) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", ErrEmptyResponse
	}
	if c.model == "" {
		return "", fmt.Errorf("generator/llm: model is required")
	}

	reqBody, err := json.Marshal(chatCompletionsRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		return "", fmt.Errorf("generator/llm: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("generator/llm: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("generator/llm: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("generator/llm: read response: %w", err)
	}

	var decoded chatCompletionsResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", fmt.Errorf("generator/llm: decode response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(extractAPIError(decoded, body))
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return "", fmt.Errorf("generator/llm: server returned %d: %s", resp.StatusCode, msg)
	}

	if len(decoded.Choices) == 0 {
		return "", ErrEmptyResponse
	}

	content := strings.TrimSpace(decoded.Choices[0].Message.Content)
	if content == "" {
		return "", ErrEmptyResponse
	}
	return content, nil
}

func extractAPIError(decoded chatCompletionsResponse, raw []byte) string {
	if decoded.Error != nil && decoded.Error.Message != "" {
		return decoded.Error.Message
	}
	return string(bytes.TrimSpace(raw))
}
