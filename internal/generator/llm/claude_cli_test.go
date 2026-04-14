package llm

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestClaudeCLIGenerate(t *testing.T) {
	tests := []struct {
		name        string
		envOutput   string
		envStderr   string
		envExitCode string
		prompt      string
		want        string
		wantErr     error
	}{
		{
			name:      "cleans markdown and wrapping quotes",
			envOutput: "```text\n\"*hello* _world_\"\n```",
			prompt:    "test prompt",
			want:      "hello world",
		},
		{
			name:      "empty output becomes ErrEmptyResponse",
			envOutput: "   ",
			prompt:    "test prompt",
			wantErr:   ErrEmptyResponse,
		},
		{
			name:        "non zero exit returns wrapped error",
			envStderr:   "boom",
			envExitCode: "7",
			prompt:      "test prompt",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GO_WANT_HELPER_PROCESS", "1")
			t.Setenv("GO_HELPER_STDOUT", tt.envOutput)
			t.Setenv("GO_HELPER_STDERR", tt.envStderr)
			t.Setenv("GO_HELPER_EXIT_CODE", tt.envExitCode)

			client := NewClaudeCLI("claude-test")
			client.commandContext = helperCommandContext
			client.shortPromptTimeout = time.Second
			client.longPromptTimeout = time.Second

			got, err := client.Generate(context.Background(), tt.prompt)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Generate() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if tt.envExitCode != "" {
				if err == nil || !strings.Contains(err.Error(), tt.envStderr) {
					t.Fatalf("Generate() error = %v, want stderr %q", err, tt.envStderr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Generate() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("Generate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClaudeCLIGenerate_TimeoutPropagatesDeadlineExceeded(t *testing.T) {
	commandContext := func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess", "--", name)
	}

	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	t.Setenv("GO_HELPER_SLEEP_MS", "200")
	t.Setenv("GO_HELPER_EXIT_CODE", "0")

	client := NewClaudeCLI("")
	client.commandContext = commandContext
	client.shortPromptTimeout = 20 * time.Millisecond
	client.longPromptTimeout = 20 * time.Millisecond

	_, err := client.Generate(context.Background(), "timeout")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Generate() error = %v, want context.DeadlineExceeded", err)
	}
}

func TestClaudeCLITimeoutForPrompt(t *testing.T) {
	t.Parallel()

	client := NewClaudeCLI("")
	client.shortPromptTimeout = time.Second
	client.longPromptTimeout = 2 * time.Second
	client.longPromptThreshold = 10

	if got := client.timeoutForPrompt("short"); got != time.Second {
		t.Fatalf("timeoutForPrompt(short) = %v, want %v", got, time.Second)
	}
	if got := client.timeoutForPrompt(strings.Repeat("x", 10)); got != 2*time.Second {
		t.Fatalf("timeoutForPrompt(long) = %v, want %v", got, 2*time.Second)
	}
}

func helperCommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cs := append([]string{"-test.run=TestHelperProcess", "--", name}, args...)
	return exec.CommandContext(ctx, os.Args[0], cs...)
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	if sleep := os.Getenv("GO_HELPER_SLEEP_MS"); sleep != "" {
		d, err := time.ParseDuration(sleep + "ms")
		if err == nil {
			time.Sleep(d)
		}
	}

	if stderr := os.Getenv("GO_HELPER_STDERR"); stderr != "" {
		_, _ = os.Stderr.WriteString(stderr)
	}
	if stdout := os.Getenv("GO_HELPER_STDOUT"); stdout != "" {
		_, _ = os.Stdout.WriteString(stdout)
	}

	code := 0
	if raw := os.Getenv("GO_HELPER_EXIT_CODE"); raw != "" {
		switch raw {
		case "0":
			code = 0
		default:
			code = 1
		}
	}
	os.Exit(code)
}
