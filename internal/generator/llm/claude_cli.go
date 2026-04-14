package llm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	defaultClaudeBinary        = "claude"
	defaultShortPromptTimeout  = 120 * time.Second
	defaultLongPromptTimeout   = 300 * time.Second
	defaultLongPromptThreshold = 2000
)

// ClaudeCLI calls the Claude CLI as an LLM backend.
type ClaudeCLI struct {
	binary              string
	model               string
	commandContext      func(context.Context, string, ...string) *exec.Cmd
	shortPromptTimeout  time.Duration
	longPromptTimeout   time.Duration
	longPromptThreshold int
}

// NewClaudeCLI returns a Claude CLI client with production defaults.
func NewClaudeCLI(model string) *ClaudeCLI {
	return &ClaudeCLI{
		binary:              defaultClaudeBinary,
		model:               model,
		commandContext:      exec.CommandContext,
		shortPromptTimeout:  defaultShortPromptTimeout,
		longPromptTimeout:   defaultLongPromptTimeout,
		longPromptThreshold: defaultLongPromptThreshold,
	}
}

// Generate runs `claude -p <prompt>` and returns cleaned script text.
func (c *ClaudeCLI) Generate(ctx context.Context, prompt string) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", ErrEmptyResponse
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeoutForPrompt(prompt))
	defer cancel()

	args := []string{"-p", prompt}
	if c.model != "" {
		args = append(args, "--model", c.model)
	}

	commandContext := c.commandContext
	if commandContext == nil {
		commandContext = exec.CommandContext
	}
	cmd := commandContext(ctx, c.binary, args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", context.DeadlineExceeded
		}

		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return "", fmt.Errorf("generator/llm: claude cli failed: %s: %w", msg, err)
		}
		return "", fmt.Errorf("generator/llm: claude cli failed: %w", err)
	}

	cleaned := cleanClaudeOutput(stdout.String())
	if cleaned == "" {
		return "", ErrEmptyResponse
	}
	return cleaned, nil
}

func (c *ClaudeCLI) timeoutForPrompt(prompt string) time.Duration {
	if len(prompt) >= c.longPromptThreshold {
		return c.longPromptTimeout
	}
	return c.shortPromptTimeout
}

func cleanClaudeOutput(text string) string {
	cleaned := strings.TrimSpace(text)

	if strings.HasPrefix(cleaned, "```") {
		lines := strings.Split(cleaned, "\n")
		if len(lines) >= 3 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
			cleaned = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	cleaned = strings.ReplaceAll(cleaned, "*", "")
	cleaned = strings.ReplaceAll(cleaned, "_", "")
	cleaned = strings.TrimSpace(cleaned)

	if len(cleaned) >= 2 && cleaned[0] == '"' && cleaned[len(cleaned)-1] == '"' {
		cleaned = strings.TrimSpace(cleaned[1 : len(cleaned)-1])
	}

	return cleaned
}
