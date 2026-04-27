package generator

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeTTS struct {
	mu            sync.Mutex
	calls         []ttsCall
	sleep         time.Duration
	activeCalls   atomic.Int32
	maxConcurrent atomic.Int32
}

type ttsCall struct {
	Text  string
	Voice string
}

func (f *fakeTTS) Synthesize(ctx context.Context, text, voice string, dst io.Writer) error {
	_ = ctx
	current := f.activeCalls.Add(1)
	for {
		maxSeen := f.maxConcurrent.Load()
		if current <= maxSeen || f.maxConcurrent.CompareAndSwap(maxSeen, current) {
			break
		}
	}
	defer f.activeCalls.Add(-1)

	if f.sleep > 0 {
		time.Sleep(f.sleep)
	}

	f.mu.Lock()
	f.calls = append(f.calls, ttsCall{Text: text, Voice: voice})
	f.mu.Unlock()

	_, err := dst.Write([]byte(text))
	return err
}

func (f *fakeTTS) snapshotCalls() []ttsCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]ttsCall, len(f.calls))
	copy(out, f.calls)
	return out
}

func TestPreprocessForTTS(t *testing.T) {
	t.Parallel()

	got := PreprocessForTTS(`你好**God's Plan**[pause][laugh][cough]`)
	want := "你好God's Plan……（轻笑）（轻咳）"
	if got != want {
		t.Fatalf("PreprocessForTTS() = %q, want %q", got, want)
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

	got := ParseDialogue("HOST_A:你好。\nHOST_B:晚上好。\nHOST_A:继续。")
	if len(got) != 3 {
		t.Fatalf("len(parts) = %d, want 3", len(got))
	}
	if got[1].Speaker != "HOST_B" || got[1].Text != "晚上好。" {
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
	calls := fake.snapshotCalls()
	if len(calls) != 1 {
		t.Fatalf("TTS calls = %d, want 1", len(calls))
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
	calls := fake.snapshotCalls()
	if len(calls) != 1 {
		t.Fatalf("TTS calls = %d, want 1", len(calls))
	}
	if !strings.Contains(calls[0].Text, "（深呼吸）今晚我们继续。") {
		t.Fatalf("mapped text = %q", calls[0].Text)
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
	calls := fake.snapshotCalls()
	if len(calls) != 1 {
		t.Fatalf("TTS calls = %d, want 1", len(calls))
	}
	if strings.ContainsRune(calls[0].Text, '\x00') {
		t.Fatalf("text still contains control character: %q", calls[0].Text)
	}
	if strings.Contains(calls[0].Text, "😀") {
		t.Fatalf("text still contains emoji: %q", calls[0].Text)
	}
	if !strings.Contains(calls[0].Text, "你好 世界") {
		t.Fatalf("cleaned text = %q", calls[0].Text)
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

	script := "HOST_A:你好，夜行人。\nHOST_B:晚上好，我们开始吧。"
	err := renderer.RenderMulti(context.Background(), script, map[string]string{
		"host":  "am_michael",
		"guest": "af_bella",
	}, output, PerformanceModeConstrained)
	if err != nil {
		t.Fatalf("RenderMulti() error = %v", err)
	}
	calls := fake.snapshotCalls()
	if len(calls) != 2 {
		t.Fatalf("TTS calls = %d, want 2", len(calls))
	}
	if calls[0].Voice != "am_michael" || calls[1].Voice != "af_bella" {
		t.Fatalf("voices = %#v", calls)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), "你好，夜行人。") || !strings.Contains(string(data), "晚上好，我们开始吧。") {
		t.Fatalf("unexpected output %q", string(data))
	}
}

func TestRenderPartsRendersInOrderAndConcats(t *testing.T) {
	fake := &fakeTTS{}
	renderer := NewRenderer(fake)
	renderer.commandContext = helperRendererCommandContext
	output := filepath.Join(t.TempDir(), "parts.wav")
	t.Setenv("GO_WANT_RENDERER_HELPER", "1")

	err := renderer.RenderParts(context.Background(), []ScriptPart{
		{Index: 1, Title: "One", Script: "first part", Speakers: []string{"HOST"}},
		{Index: 2, Title: "Two", Script: "HOST:second host\nGUEST:second guest", Speakers: []string{"HOST", "GUEST"}},
	}, map[string]string{
		"host":  "am_michael",
		"guest": "af_bella",
	}, output, PerformanceModeConstrained)
	if err != nil {
		t.Fatalf("RenderParts() error = %v", err)
	}
	calls := fake.snapshotCalls()
	if len(calls) != 3 {
		t.Fatalf("TTS calls = %d, want 3", len(calls))
	}
	if calls[0].Voice != "am_michael" || calls[1].Voice != "am_michael" || calls[2].Voice != "af_bella" {
		t.Fatalf("voices = %#v", calls)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "first part") || !strings.Contains(got, "second host") || !strings.Contains(got, "second guest") {
		t.Fatalf("output missing parts: %q", got)
	}
}

func TestRenderPartsReturnsErrorForEmptyParts(t *testing.T) {
	t.Parallel()

	renderer := NewRenderer(&fakeTTS{})
	err := renderer.RenderParts(context.Background(), nil, nil, filepath.Join(t.TempDir(), "empty.wav"), PerformanceModeConstrained)
	if err == nil {
		t.Fatal("RenderParts() error = nil, want error")
	}
}

func TestRenderSingleUsesBoundedChunkConcurrency(t *testing.T) {
	fake := &fakeTTS{sleep: 25 * time.Millisecond}
	renderer := NewRenderer(fake, WithSynthesisConcurrency(5), WithChunkWords(100))
	renderer.commandContext = helperRendererCommandContext
	t.Setenv("GO_WANT_RENDERER_HELPER", "1")
	output := filepath.Join(t.TempDir(), "concurrent.wav")

	script := strings.Repeat("深夜还在继续。", 120)
	if err := renderer.RenderSingle(context.Background(), script, "default_zh", output, PerformanceModeConstrained); err != nil {
		t.Fatalf("RenderSingle() error = %v", err)
	}

	if got := fake.maxConcurrent.Load(); got > 5 {
		t.Fatalf("max concurrent calls = %d, want <= 5", got)
	}
	if got := len(fake.snapshotCalls()); got < 6 {
		t.Fatalf("expected enough chunks to exercise concurrency, got %d", got)
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
	if !strings.Contains(argString, "-nostdin") || !strings.Contains(argString, "-loglevel error") {
		t.Fatalf("concat args missing hardened ffmpeg flags: %s", argString)
	}
}

func TestConcatenateAudioAppliesTinyEdgeFades(t *testing.T) {
	renderer := NewRenderer(&fakeTTS{}, WithConcatFade(0.02))
	var fadeCalls int
	renderer.commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if name == "ffmpeg" && containsArg(args, "-af") {
			fadeCalls++
			argString := strings.Join(args, " ")
			if !strings.Contains(argString, "afade=t=in:d=0.02") {
				t.Fatalf("fade args missing requested duration: %s", argString)
			}
		}
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
	if fadeCalls != 2 {
		t.Fatalf("fade calls = %d, want 2", fadeCalls)
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
		if containsArgPair(args[4:], "-f", "lavfi") {
			out := args[len(args)-1]
			_ = os.WriteFile(out, []byte("gap"), 0o644)
			os.Exit(0)
		}
		if containsArg(args[4:], "-af") {
			input := ""
			for i := 4; i < len(args)-1; i++ {
				if args[i] == "-i" && i+1 < len(args) {
					input = args[i+1]
					break
				}
			}
			if input == "" {
				os.Exit(4)
			}
			out := args[len(args)-1]
			data, _ := os.ReadFile(input)
			_ = os.WriteFile(out, data, 0o644)
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

func containsArg(args []string, value string) bool {
	for _, arg := range args {
		if arg == value {
			return true
		}
	}
	return false
}

func containsArgPair(args []string, key, value string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key && args[i+1] == value {
			return true
		}
	}
	return false
}
