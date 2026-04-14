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

const (
	defaultLiteLLMTimeout = 120 * time.Second
	defaultLiteLLMPath    = "/chat/completions"
)

type liteLLMRequest struct {
	Model    string         `json:"model"`
	Messages []chatMessage  `json:"messages"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// LiteLLMClient is a dedicated adapter for LiteLLM proxy deployments.
// It preserves LiteLLM-specific request semantics such as metadata/tags while
// still satisfying the generic LLM client interface.
type LiteLLMClient struct {
	baseURL    string
	apiKey     string
	model      string
	path       string
	metadata   map[string]any
	httpClient *http.Client
}

// NewLiteLLMClient returns a LiteLLM client with default endpoint settings.
func NewLiteLLMClient(baseURL, apiKey, model string, metadata map[string]any) *LiteLLMClient {
	return &LiteLLMClient{
		baseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:   strings.TrimSpace(apiKey),
		model:    strings.TrimSpace(model),
		path:     defaultLiteLLMPath,
		metadata: cloneMetadata(metadata),
		httpClient: &http.Client{
			Timeout: defaultLiteLLMTimeout,
		},
	}
}

// Generate sends a single-message completion request to LiteLLM.
func (c *LiteLLMClient) Generate(ctx context.Context, prompt string) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", ErrEmptyResponse
	}
	if c.model == "" {
		return "", fmt.Errorf("generator/llm: model is required")
	}
	if c.baseURL == "" {
		return "", fmt.Errorf("generator/llm: base URL is required")
	}

	reqBody, err := json.Marshal(liteLLMRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
		Metadata: cloneMetadata(c.metadata),
	})
	if err != nil {
		return "", fmt.Errorf("generator/llm: marshal LiteLLM request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+c.path, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("generator/llm: create LiteLLM request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("generator/llm: LiteLLM request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("generator/llm: read LiteLLM response: %w", err)
	}

	var decoded chatCompletionsResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", fmt.Errorf("generator/llm: decode LiteLLM response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(extractAPIError(decoded, body))
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return "", fmt.Errorf("generator/llm: LiteLLM returned %d: %s", resp.StatusCode, msg)
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

func cloneMetadata(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}

	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
