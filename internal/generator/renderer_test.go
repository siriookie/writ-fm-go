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

	got := PreprocessForTTS(`你好**God's Plan**[pause][laugh][cough]`)
	if got != "你好God's Plan……（轻笑）（轻咳）" {
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

func TestSplitIntoChunksChinese(t *testing.T) {
	t.Parallel()

	text := "深夜电台还亮着。城市已经睡了，但是还有人醒着。我们继续说下去。"
	got := SplitIntoChunks(text, 12)
	if len(got) < 2 {
		t.Fatalf("len(chunks) = %d, want >= 2; chunks=%#v", len(got), got)
	}
	if got[0] != "深夜电台还亮着。" {
		t.Fatalf("first chunk = %q", got[0])
	}
}

func TestSplitIntoChunksChineseWithoutSpaces(t *testing.T) {
	t.Parallel()

	text := "这是一个没有空格但是非常长的中文段落它应该在超过阈值之后被切开否则渲染时会把整段一次性丢给TTS后端"
	got := SplitIntoChunks(text, 10)
	if len(got) < 2 {
		t.Fatalf("len(chunks) = %d, want >= 2; chunks=%#v", len(got), got)
	}
}

func TestParseDialogue(t *testing.T) {
	t.Parallel()

	got := ParseDialogue("主持人：你好。\n嘉宾：也向你问好。\n主持人甲：继续。")
	if len(got) != 3 {
		t.Fatalf("len(parts) = %d, want 3", len(got))
	}
	if got[1].Speaker != "嘉宾" || got[1].Text != "也向你问好。" {
		t.Fatalf("unexpected part %#v", got[1])
	}
}

func TestRenderSingleShort(t *testing.T) {
	t.Parallel()

	fake := &fakeTTS{}
	renderer := NewRenderer(fake)
	output := filepath.Join(t.TempDir(), "single.wav")

	if err := renderer.RenderSingle(context.Background(), "hello world", "am_michael", output, PerformanceModeConstrained); err != nil {
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

func TestRenderSingleExpressiveMimoMapsCue(t *testing.T) {
	t.Parallel()

	fake := &fakeTTS{}
	renderer := NewRenderer(fake, WithBackend("mimo"))
	output := filepath.Join(t.TempDir(), "single.wav")

	if err := renderer.RenderSingle(context.Background(), "（深呼吸）今晚我们继续。", "default_zh", output, PerformanceModeExpressive); err != nil {
		t.Fatalf("RenderSingle() error = %v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("TTS calls = %d, want 1", len(fake.calls))
	}
	if !strings.Contains(fake.calls[0].Text, "（深呼吸）今晚我们继续。") {
		t.Fatalf("mapped text = %q", fake.calls[0].Text)
	}
}

func TestRenderSingleCleansTextBeforeTTS(t *testing.T) {
	t.Parallel()

	fake := &fakeTTS{}
	renderer := NewRenderer(fake)
	output := filepath.Join(t.TempDir(), "single.wav")

	if err := renderer.RenderSingle(context.Background(), "你好\x00  世界 😀 [pause]", "default_zh", output, PerformanceModeConstrained); err != nil {
		t.Fatalf("RenderSingle() error = %v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("TTS calls = %d, want 1", len(fake.calls))
	}
	if strings.ContainsRune(fake.calls[0].Text, '\x00') {
		t.Fatalf("text still contains control character: %q", fake.calls[0].Text)
	}
	if strings.Contains(fake.calls[0].Text, "😀") {
		t.Fatalf("text still contains emoji: %q", fake.calls[0].Text)
	}
	if !strings.Contains(fake.calls[0].Text, "你好 世界") {
		t.Fatalf("cleaned text = %q", fake.calls[0].Text)
	}
}

func TestRenderSingleWritesChunkDebugArtifacts(t *testing.T) {
	fake := &fakeTTS{}
	debugDir := t.TempDir()
	renderer := NewRenderer(fake, WithChunkDebug(debugDir))
	renderer.commandContext = helperRendererCommandContext
	t.Setenv("GO_WANT_RENDERER_HELPER", "1")
	output := filepath.Join(t.TempDir(), "single.wav")

	if err := renderer.RenderSingle(context.Background(), strings.Repeat("深夜", 80)+"。再继续一点。", "default_zh", output, PerformanceModeConstrained); err != nil {
		t.Fatalf("RenderSingle() error = %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(debugDir, "single"))
	if err != nil {
		t.Fatalf("ReadDir(debug) error = %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected chunk debug artifacts")
	}

	var foundText, foundAudio bool
	for _, entry := range entries {
		switch filepath.Ext(entry.Name()) {
		case ".txt":
			foundText = true
		case ".wav":
			foundAudio = true
		}
	}
	if !foundText || !foundAudio {
		t.Fatalf("expected both text and audio debug files, got %#v", entries)
	}
}

func TestRenderMultiUsesSpeakerVoicesAndConcat(t *testing.T) {
	fake := &fakeTTS{}
	renderer := NewRenderer(fake)
	renderer.commandContext = helperRendererCommandContext
	output := filepath.Join(t.TempDir(), "multi.wav")

	t.Setenv("GO_WANT_RENDERER_HELPER", "1")

	script := "主持人：你好，夜行人。\n嘉宾：晚上好，我们开始吧。"
	err := renderer.RenderMulti(context.Background(), script, map[string]string{
		"host":  "am_michael",
		"guest": "af_bella",
	}, output, PerformanceModeConstrained)
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
	if !strings.Contains(string(data), "你好，夜行人。") || !strings.Contains(string(data), "晚上好，我们开始吧。") {
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

func TestConcatenateAudioReencodesWAVChunks(t *testing.T) {
	renderer := NewRenderer(&fakeTTS{})
	var gotName string
	var gotArgs []string
	renderer.commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return helperRendererCommandContext(ctx, name, args...)
	}
	t.Setenv("GO_WANT_RENDERER_HELPER", "1")

	dir := t.TempDir()
	chunkA := filepath.Join(dir, "a.wav")
	chunkB := filepath.Join(dir, "b.wav")
	if err := os.WriteFile(chunkA, []byte("chunk-a"), 0o644); err != nil {
		t.Fatalf("WriteFile(chunkA) error = %v", err)
	}
	if err := os.WriteFile(chunkB, []byte("chunk-b"), 0o644); err != nil {
		t.Fatalf("WriteFile(chunkB) error = %v", err)
	}
	output := filepath.Join(dir, "out.wav")

	if err := renderer.concatenateAudio(context.Background(), []string{chunkA, chunkB}, output, 0); err != nil {
		t.Fatalf("concatenateAudio() error = %v", err)
	}
	if gotName != "ffmpeg" {
		t.Fatalf("command name = %q, want ffmpeg", gotName)
	}
	argString := strings.Join(gotArgs, " ")
	if !strings.Contains(argString, "-c:a pcm_s16le") {
		t.Fatalf("concat args missing pcm_s16le reencode: %s", argString)
	}
	if strings.Contains(argString, "-c copy") {
		t.Fatalf("concat args should not use stream copy: %s", argString)
	}
}

func TestCountTextUnitsMixedLanguage(t *testing.T) {
	t.Parallel()

	if got := countTextUnits("hello world"); got != 2 {
		t.Fatalf("countTextUnits(english) = %d, want 2", got)
	}
	if got := countTextUnits("你好世界"); got != 4 {
		t.Fatalf("countTextUnits(chinese) = %d, want 4", got)
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
