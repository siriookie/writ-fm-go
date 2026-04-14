package generator

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type fakeTTS struct {
	calls []ttsCall
}

type ttsCall struct {
	Text  string
	Voice string
}

func (f *fakeTTS) Synthesize(ctx context.Context, text, voice string, dst io.Writer) error {
	_ = ctx
	f.calls = append(f.calls, ttsCall{Text: text, Voice: voice})
	_, err := dst.Write([]byte(text))
	return err
}

func TestPreprocessForTTS(t *testing.T) {
	t.Parallel()

	got := PreprocessForTTS(`Hello [pause] "[chuckle]" [cough]`)
	if got != "Hello ... heh... ahem..." {
		t.Fatalf("PreprocessForTTS() = %q", got)
	}
}

func TestSplitIntoChunks(t *testing.T) {
	t.Parallel()

	text := "One two three four. Five six seven eight. Nine ten eleven."
	got := SplitIntoChunks(text, 4)
	if len(got) != 3 {
		t.Fatalf("len(chunks) = %d, want 3", len(got))
	}
	if got[0] != "One two three four." {
		t.Fatalf("first chunk = %q", got[0])
	}
}

func TestParseDialogue(t *testing.T) {
	t.Parallel()

	got := ParseDialogue("HOST: Hello there.\nGUEST: Hi.\nHOST_A: Again.")
	if len(got) != 3 {
		t.Fatalf("len(parts) = %d, want 3", len(got))
	}
	if got[1].Speaker != "GUEST" || got[1].Text != "Hi." {
		t.Fatalf("unexpected part %#v", got[1])
	}
}

func TestRenderSingleShort(t *testing.T) {
	t.Parallel()

	fake := &fakeTTS{}
	renderer := NewRenderer(fake)
	output := filepath.Join(t.TempDir(), "single.wav")

	if err := renderer.RenderSingle(context.Background(), "hello world", "am_michael", output); err != nil {
		t.Fatalf("RenderSingle() error = %v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("TTS calls = %d, want 1", len(fake.calls))
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "hello world" {
		t.Fatalf("output = %q", string(data))
	}
}

func TestRenderMultiUsesSpeakerVoicesAndConcat(t *testing.T) {
	fake := &fakeTTS{}
	renderer := NewRenderer(fake)
	renderer.commandContext = helperRendererCommandContext
	output := filepath.Join(t.TempDir(), "multi.wav")

	t.Setenv("GO_WANT_RENDERER_HELPER", "1")

	script := "HOST: hello there.\nGUEST: general kenobi."
	err := renderer.RenderMulti(context.Background(), script, map[string]string{
		"host":  "am_michael",
		"guest": "af_bella",
	}, output)
	if err != nil {
		t.Fatalf("RenderMulti() error = %v", err)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("TTS calls = %d, want 2", len(fake.calls))
	}
	if fake.calls[0].Voice != "am_michael" || fake.calls[1].Voice != "af_bella" {
		t.Fatalf("voices = %#v", fake.calls)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), "hello there.") || !strings.Contains(string(data), "general kenobi.") {
		t.Fatalf("unexpected output %q", string(data))
	}
}

func TestDuration(t *testing.T) {
	renderer := NewRenderer(&fakeTTS{})
	renderer.commandContext = helperRendererCommandContext
	t.Setenv("GO_WANT_RENDERER_HELPER", "1")
	t.Setenv("GO_HELPER_FFPROBE_STDOUT", "12.5")

	seconds, err := renderer.Duration(context.Background(), "dummy.wav")
	if err != nil {
		t.Fatalf("Duration() error = %v", err)
	}
	if seconds != 12.5 {
		t.Fatalf("Duration() = %v", seconds)
	}
}

func helperRendererCommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cs := append([]string{"-test.run=TestHelperRendererProcess", "--", name}, args...)
	return exec.CommandContext(ctx, os.Args[0], cs...)
}

func TestHelperRendererProcess(t *testing.T) {
	if os.Getenv("GO_WANT_RENDERER_HELPER") != "1" {
		return
	}
	args := os.Args
	if len(args) < 4 {
		os.Exit(2)
	}
	name := args[3]
	switch name {
	case "ffprobe":
		_, _ = os.Stdout.WriteString(os.Getenv("GO_HELPER_FFPROBE_STDOUT"))
		os.Exit(0)
	case "ffmpeg":
		if len(args) >= 11 && args[4] == "-y" && args[5] == "-f" && args[6] == "lavfi" {
			out := args[len(args)-1]
			_ = os.WriteFile(out, []byte("gap"), 0o644)
			os.Exit(0)
		}
		listFile := ""
		for i := 4; i < len(args)-1; i++ {
			if args[i] == "-i" && i+1 < len(args) {
				listFile = args[i+1]
				break
			}
		}
		if listFile == "" {
			os.Exit(3)
		}
		out := args[len(args)-1]
		listData, _ := os.ReadFile(listFile)
		lines := strings.Split(strings.TrimSpace(string(listData)), "\n")
		var combined bytes.Buffer
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			path := strings.TrimPrefix(line, "file '")
			path = strings.TrimSuffix(path, "'")
			chunk, _ := os.ReadFile(path)
			combined.Write(chunk)
		}
		_ = os.WriteFile(out, combined.Bytes(), 0o644)
		os.Exit(0)
	}
	os.Exit(1)
}
