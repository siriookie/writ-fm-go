package tts

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPythonBinForGOOS(t *testing.T) {
	t.Parallel()

	base := filepath.Join("tmp", "kokoro")
	if got := pythonBinForGOOS(base, "windows"); got != filepath.Join(base, ".venv", "Scripts", "python.exe") {
		t.Fatalf("windows python path = %q", got)
	}
	if got := pythonBinForGOOS(base, "darwin"); got != filepath.Join(base, ".venv", "bin", "python") {
		t.Fatalf("unix python path = %q", got)
	}
}

func TestKokoroSynthesize(t *testing.T) {
	t.Setenv("GO_WANT_KOKORO_HELPER", "1")
	t.Setenv("GO_HELPER_KOKORO_WAV", "wav-bytes")

	client := newTestKokoro(t)
	var dst bytes.Buffer

	if err := client.Synthesize(context.Background(), "hello world", "am_michael", &dst); err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if got := dst.String(); got != "wav-bytes" {
		t.Fatalf("Synthesize() wrote %q, want %q", got, "wav-bytes")
	}
}

func TestKokoroSynthesize_EmptyText(t *testing.T) {
	t.Parallel()

	client := &KokoroTTS{}
	var dst bytes.Buffer
	err := client.Synthesize(context.Background(), "   ", "am_michael", &dst)
	if !errors.Is(err, ErrEmptyText) {
		t.Fatalf("Synthesize() error = %v, want ErrEmptyText", err)
	}
}

func TestKokoroSynthesize_ReportsCommandFailure(t *testing.T) {
	t.Setenv("GO_WANT_KOKORO_HELPER", "1")
	t.Setenv("GO_HELPER_KOKORO_STDERR", "boom")
	t.Setenv("GO_HELPER_KOKORO_EXIT_CODE", "4")

	client := newTestKokoro(t)
	var dst bytes.Buffer

	err := client.Synthesize(context.Background(), "hello world", "am_michael", &dst)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Synthesize() error = %v, want stderr in message", err)
	}
}

func TestKokoroSynthesize_Timeout(t *testing.T) {
	t.Setenv("GO_WANT_KOKORO_HELPER", "1")
	t.Setenv("GO_HELPER_KOKORO_SLEEP_MS", "200")

	client := newTestKokoro(t)
	client.timeout = 20 * time.Millisecond

	var dst bytes.Buffer
	err := client.Synthesize(context.Background(), "hello world", "am_michael", &dst)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Synthesize() error = %v, want context.DeadlineExceeded", err)
	}
}

func newTestKokoro(t *testing.T) *KokoroTTS {
	t.Helper()

	root := t.TempDir()
	pythonPath := pythonBinForGOOS(root, runtime.GOOS)
	if err := os.MkdirAll(filepath.Dir(pythonPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(pythonPath, []byte("helper"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := NewKokoroTTS(root)
	client.commandContext = helperKokoroCommandContext
	client.tempDir = root
	return client
}

func helperKokoroCommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cs := append([]string{"-test.run=TestHelperKokoroProcess", "--", name}, args...)
	return exec.CommandContext(ctx, os.Args[0], cs...)
}

func TestHelperKokoroProcess(t *testing.T) {
	if os.Getenv("GO_WANT_KOKORO_HELPER") != "1" {
		return
	}

	if sleep := os.Getenv("GO_HELPER_KOKORO_SLEEP_MS"); sleep != "" {
		d, err := time.ParseDuration(sleep + "ms")
		if err == nil {
			time.Sleep(d)
		}
	}

	args := os.Args
	if len(args) < 6 {
		os.Exit(2)
	}

	script := args[len(args)-1]
	marker := `sf.write("`
	start := strings.Index(script, marker)
	if start >= 0 {
		start += len(marker)
		end := strings.Index(script[start:], `"`)
		if end >= 0 {
			path := script[start : start+end]
			_ = os.WriteFile(path, []byte(os.Getenv("GO_HELPER_KOKORO_WAV")), 0o644)
		}
	}

	if stderr := os.Getenv("GO_HELPER_KOKORO_STDERR"); stderr != "" {
		_, _ = os.Stderr.WriteString(stderr)
	}

	if os.Getenv("GO_HELPER_KOKORO_EXIT_CODE") != "" {
		os.Exit(1)
	}
	_, _ = os.Stdout.WriteString("SUCCESS")
	os.Exit(0)
}
