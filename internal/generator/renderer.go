package generator

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	perf "github.com/writ-fm/go/internal/generator/performance"
	gentts "github.com/writ-fm/go/internal/generator/tts"
)

const (
	defaultChunkWords           = 240
	defaultGapSeconds           = 0.3
	defaultConcatFadeSeconds    = 0.012
	defaultSynthesisConcurrency = 5
)

var (
	speakerSplitRE = regexp.MustCompile(`((?:HOST_A|HOST_B|GUEST|HOST|主持人甲|主持人乙|主持人|嘉宾|主持人 [A-Z][A-Z\s.]+)(?:：|:))`)
	speakerTagRE   = regexp.MustCompile(`^(?:HOST_A|HOST_B|GUEST|HOST|主持人甲|主持人乙|主持人|嘉宾|主持人 [A-Z][A-Z\s.]+)(?:：|:)$`)
	textUnitRE     = regexp.MustCompile(`[\p{Han}\p{Hiragana}\p{Katakana}\p{Hangul}]|[\p{L}\p{N}]+(?:['_-][\p{L}\p{N}]+)*`)
)

// DialoguePart is one speaker-labelled portion of a multi-voice script.
type DialoguePart struct {
	Speaker string
	Text    string
}

// Renderer turns generated scripts into audio files through a TTS backend.
type Renderer struct {
	tts            gentts.Client
	backend        string
	debugChunkDir  string
	synthWorkers   int
	chunkWords     int
	concatFade     float64
	ffmpegBin      string
	ffprobeBin     string
	tempDir        string
	commandContext func(context.Context, string, ...string) *exec.Cmd
}

type RendererOption func(*Renderer)

func WithBackend(name string) RendererOption {
	return func(r *Renderer) {
		r.backend = strings.ToLower(strings.TrimSpace(name))
	}
}

func WithChunkDebug(dir string) RendererOption {
	return func(r *Renderer) {
		r.debugChunkDir = strings.TrimSpace(dir)
	}
}

func WithSynthesisConcurrency(n int) RendererOption {
	return func(r *Renderer) {
		if n > 0 {
			r.synthWorkers = n
		}
	}
}

func WithChunkWords(n int) RendererOption {
	return func(r *Renderer) {
		if n > 0 {
			r.chunkWords = n
		}
	}
}

func WithConcatFade(seconds float64) RendererOption {
	return func(r *Renderer) {
		if seconds >= 0 {
			r.concatFade = seconds
		}
	}
}

// NewRenderer returns a renderer with production defaults.
func NewRenderer(client gentts.Client, opts ...RendererOption) *Renderer {
	renderer := &Renderer{
		tts:            client,
		backend:        "generic",
		synthWorkers:   defaultSynthesisConcurrency,
		chunkWords:     defaultChunkWords,
		concatFade:     defaultConcatFadeSeconds,
		ffmpegBin:      "ffmpeg",
		ffprobeBin:     "ffprobe",
		commandContext: exec.CommandContext,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(renderer)
		}
	}
	return renderer
}

// PreprocessForTTS only performs final cleanup before sending text to a TTS backend.
func PreprocessForTTS(text string) string {
	replacer := strings.NewReplacer(
		"**", "",
		"__", "",
		"`", "",
		"[pause]", "……",
		"[breath]", "（轻轻吸气）",
		"[laugh]", "（轻笑）",
		"[chuckle]", "（轻笑）",
		"[cough]", "（轻咳）",
	)
	return strings.TrimSpace(replacer.Replace(text))
}

// SplitIntoChunks splits long scripts near sentence boundaries while keeping chunks near maxWords.
func SplitIntoChunks(script string, maxWords int) []string {
	script = strings.TrimSpace(script)
	if script == "" {
		return nil
	}
	if maxWords <= 0 {
		maxWords = defaultChunkWords
	}
	if countTextUnits(script) <= maxWords {
		return []string{script}
	}

	sentences := splitSentences(script)
	if len(sentences) == 0 {
		return []string{script}
	}

	var chunks []string
	var current []string
	currentWords := 0
	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" {
			continue
		}
		sentenceWords := countTextUnits(sentence)
		if sentenceWords > maxWords {
			if len(current) > 0 {
				chunks = append(chunks, strings.Join(current, " "))
				current = nil
				currentWords = 0
			}
			chunks = append(chunks, splitLongSegment(sentence, maxWords)...)
			continue
		}
		if currentWords+sentenceWords > maxWords && len(current) > 0 {
			chunks = append(chunks, strings.Join(current, " "))
			current = []string{sentence}
			currentWords = sentenceWords
			continue
		}
		current = append(current, sentence)
		currentWords += sentenceWords
	}
	if len(current) > 0 {
		chunks = append(chunks, strings.Join(current, " "))
	}
	if len(chunks) == 0 {
		return []string{script}
	}
	return chunks
}

// ParseDialogue extracts speaker-labelled segments from a multi-voice script.
func ParseDialogue(script string) []DialoguePart {
	matches := speakerSplitRE.FindAllStringIndex(script, -1)
	if len(matches) == 0 {
		text := strings.TrimSpace(script)
		if text == "" {
			return nil
		}
		return []DialoguePart{{Speaker: "HOST", Text: text}}
	}

	parts := make([]DialoguePart, 0, len(matches))
	if prefix := strings.TrimSpace(script[:matches[0][0]]); prefix != "" {
		parts = append(parts, DialoguePart{Speaker: "HOST", Text: prefix})
	}

	for i, match := range matches {
		tag := normalizeSpeakerTag(strings.TrimSpace(script[match[0]:match[1]]))
		start := match[1]
		end := len(script)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		text := strings.TrimSpace(script[start:end])
		if text == "" {
			continue
		}
		parts = append(parts, DialoguePart{Speaker: tag, Text: text})
	}
	return parts
}

// RenderSingle renders a single-voice script to outputPath.
func (r *Renderer) RenderSingle(ctx context.Context, script, voice, outputPath string, mode PerformanceMode) error {
	return r.renderPreparedSingle(ctx, r.prepareScript(script, mode), voice, outputPath)
}

// RenderMulti renders a multi-voice script to outputPath.
func (r *Renderer) RenderMulti(ctx context.Context, script string, voices map[string]string, outputPath string, mode PerformanceMode) error {
	parts := ParseDialogue(script)
	if len(parts) == 0 {
		return r.RenderSingle(ctx, script, voices["host"], outputPath, mode)
	}

	hostVoice := firstNonEmpty(voices["host"], "am_michael")
	guestVoice := firstNonEmpty(voices["guest"], "af_bella")

	rendered := make([]string, 0, len(parts))
	for i, part := range parts {
		text := r.prepareScript(part.Text, mode)
		if text == "" {
			continue
		}
		voice := speakerVoice(part.Speaker, hostVoice, guestVoice)
		partPath := withStemSuffix(outputPath, fmt.Sprintf("_part%03d", i))
		if countTextUnits(text) > r.maxChunkWords() {
			if err := r.renderPreparedSingle(ctx, text, voice, partPath); err != nil {
				return err
			}
		} else if err := r.synthesizeToFile(ctx, text, voice, partPath); err != nil {
			return err
		}
		rendered = append(rendered, partPath)
	}
	if len(rendered) == 0 {
		return fmt.Errorf("generator/renderer: no dialogue parts rendered")
	}
	return r.concatenateAudio(ctx, rendered, outputPath, defaultGapSeconds)
}

// RenderParts renders outline-first script parts independently, then concatenates them.
func (r *Renderer) RenderParts(ctx context.Context, parts []ScriptPart, voices map[string]string, outputPath string, mode PerformanceMode) error {
	if len(parts) == 0 {
		return fmt.Errorf("generator/renderer: no script parts to render")
	}
	rendered := make([]string, 0, len(parts))
	for i, part := range parts {
		text := strings.TrimSpace(part.Script)
		if text == "" {
			continue
		}
		partPath := withStemSuffix(outputPath, fmt.Sprintf("_segment%03d", i))
		if partUsesMultipleVoices(part) || len(ParseDialogue(text)) > 1 {
			if err := r.RenderMulti(ctx, text, voices, partPath, mode); err != nil {
				r.cleanupFiles(rendered)
				return err
			}
		} else {
			voice := voiceForScriptPart(part, voices)
			if err := r.RenderSingle(ctx, text, voice, partPath, mode); err != nil {
				r.cleanupFiles(rendered)
				return err
			}
		}
		rendered = append(rendered, partPath)
	}
	if len(rendered) == 0 {
		return fmt.Errorf("generator/renderer: no script parts rendered")
	}
	return r.concatenateAudio(ctx, rendered, outputPath, defaultGapSeconds)
}

// Duration returns the media duration in seconds using ffprobe.
func (r *Renderer) Duration(ctx context.Context, path string) (float64, error) {
	commandContext := r.commandContext
	if commandContext == nil {
		commandContext = exec.CommandContext
	}
	cmd := commandContext(ctx, r.ffprobeBin, "-v", "quiet", "-show_entries", "format=duration", "-of", "csv=p=0", path)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("generator/renderer: ffprobe failed: %s: %w", strings.TrimSpace(stderr.String()), err)
	}
	value := strings.TrimSpace(stdout.String())
	if value == "" {
		return 0, fmt.Errorf("generator/renderer: ffprobe returned empty duration")
	}
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("generator/renderer: parse duration: %w", err)
	}
	return seconds, nil
}

func (r *Renderer) prepareScript(script string, mode PerformanceMode) string {
	normalized := perf.NormalizePerformanceCues(script, perf.Mode(NormalizePerformanceMode(mode)), r.backend)
	rendered := perf.RenderPerformanceForBackend(normalized, r.backend)
	return PreprocessForTTS(rendered)
}

func (r *Renderer) renderPreparedSingle(ctx context.Context, script, voice, outputPath string) error {
	chunks := SplitIntoChunks(script, r.maxChunkWords())
	if len(chunks) == 0 {
		return fmt.Errorf("generator/renderer: no text to render")
	}

	chunkFiles, err := r.renderChunks(ctx, chunks, voice, outputPath, "chunk")
	if err != nil {
		return err
	}
	return r.concatenateAudio(ctx, chunkFiles, outputPath, 0)
}

func (r *Renderer) renderChunks(ctx context.Context, chunks []string, voice, outputPath, label string) ([]string, error) {
	files := make([]string, len(chunks))
	for i := range chunks {
		files[i] = withStemSuffix(outputPath, fmt.Sprintf("_%s%03d", label, i))
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	limit := r.synthWorkers
	if limit <= 0 {
		limit = defaultSynthesisConcurrency
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	var once sync.Once
	var firstErr error

	for i, chunk := range chunks {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, chunk string) {
			defer wg.Done()
			defer func() { <-sem }()

			chunkPath := files[i]
			if err := r.synthesizeToFile(ctx, chunk, voice, chunkPath); err != nil {
				once.Do(func() {
					firstErr = err
					cancel()
				})
				return
			}
			if err := r.debugDumpChunk(outputPath, label, i, voice, chunk, chunkPath); err != nil {
				once.Do(func() {
					firstErr = err
					cancel()
				})
				return
			}
		}(i, chunk)
	}

	wg.Wait()
	if firstErr != nil {
		r.cleanupFiles(files)
		return nil, firstErr
	}
	return files, nil
}

func (r *Renderer) synthesizeToFile(ctx context.Context, text, voice, path string) error {
	if r.tts == nil {
		return fmt.Errorf("generator/renderer: TTS client is required")
	}
	text = gentts.CleanText(text)
	if text == "" {
		return gentts.ErrEmptyText
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("generator/renderer: create output dir: %w", err)
		}
		file, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("generator/renderer: create chunk file: %w", err)
		}
		err = r.tts.Synthesize(ctx, text, voice, file)
		closeErr := file.Close()
		if err == nil && closeErr != nil {
			err = closeErr
		}
		if err == nil {
			return nil
		}
		_ = os.Remove(path)
		if attempt == 1 {
			return fmt.Errorf("generator/renderer: synthesize %s: %w", path, err)
		}
		time.Sleep(2 * time.Second)
	}
	return nil
}

func (r *Renderer) concatenateAudio(ctx context.Context, chunkFiles []string, outputPath string, gapSeconds float64) error {
	if len(chunkFiles) == 0 {
		return fmt.Errorf("generator/renderer: no chunk files to concatenate")
	}
	if len(chunkFiles) == 1 && gapSeconds <= 0 {
		if err := moveFile(chunkFiles[0], outputPath); err != nil {
			return fmt.Errorf("generator/renderer: finalize single chunk: %w", err)
		}
		return nil
	}

	inputFiles := append([]string(nil), chunkFiles...)
	fadedFiles, err := r.applyConcatFades(ctx, chunkFiles, outputPath)
	if err != nil {
		r.cleanupFiles(chunkFiles)
		return err
	}
	if len(fadedFiles) > 0 {
		inputFiles = fadedFiles
		defer r.cleanupFiles(fadedFiles)
	}

	files := append([]string(nil), inputFiles...)
	if gapSeconds > 0 && len(chunkFiles) > 1 {
		silencePath := withStemSuffix(outputPath, "_gap")
		if err := r.generateSilence(ctx, silencePath, gapSeconds); err != nil {
			r.cleanupFiles(chunkFiles)
			return err
		}
		var expanded []string
		for i, file := range inputFiles {
			expanded = append(expanded, file)
			if i < len(inputFiles)-1 {
				expanded = append(expanded, silencePath)
			}
		}
		files = expanded
		defer os.Remove(silencePath)
	}

	listFile, err := r.writeConcatList(outputPath, files)
	if err != nil {
		r.cleanupFiles(chunkFiles)
		return err
	}
	defer os.Remove(listFile)

	commandContext := r.commandContext
	if commandContext == nil {
		commandContext = exec.CommandContext
	}
	cmd := commandContext(
		ctx,
		r.ffmpegBin,
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-nostdin",
		"-f", "concat",
		"-safe", "0",
		"-i", listFile,
		"-vn",
		"-ac", "1",
		"-ar", "24000",
		"-c:a", "pcm_s16le",
		outputPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		r.cleanupFiles(chunkFiles)
		return fmt.Errorf("generator/renderer: ffmpeg concat failed: %s: %w", strings.TrimSpace(stderr.String()), err)
	}
	r.cleanupFiles(chunkFiles)
	return nil
}

func (r *Renderer) writeConcatList(outputPath string, files []string) (string, error) {
	listFile := withSuffix(outputPath, ".concat.txt")
	var builder strings.Builder
	for _, file := range files {
		abs, err := filepath.Abs(file)
		if err != nil {
			return "", fmt.Errorf("generator/renderer: resolve path %s: %w", file, err)
		}
		abs = filepath.ToSlash(abs)
		builder.WriteString("file '")
		builder.WriteString(strings.ReplaceAll(abs, "'", "\\'"))
		builder.WriteString("'\n")
	}
	if err := os.WriteFile(listFile, []byte(builder.String()), 0o644); err != nil {
		return "", fmt.Errorf("generator/renderer: write concat list: %w", err)
	}
	return listFile, nil
}

func (r *Renderer) applyConcatFades(ctx context.Context, files []string, outputPath string) ([]string, error) {
	if r.concatFade <= 0 || len(files) < 2 {
		return nil, nil
	}
	faded := make([]string, len(files))
	for i, file := range files {
		fadedPath := withStemSuffix(outputPath, fmt.Sprintf("_fade%03d", i))
		if err := r.applyEdgeFade(ctx, file, fadedPath, r.concatFade); err != nil {
			r.cleanupFiles(faded)
			return nil, err
		}
		faded[i] = fadedPath
	}
	return faded, nil
}

func (r *Renderer) applyEdgeFade(ctx context.Context, inputPath, outputPath string, seconds float64) error {
	commandContext := r.commandContext
	if commandContext == nil {
		commandContext = exec.CommandContext
	}
	fade := strconv.FormatFloat(seconds, 'f', -1, 64)
	cmd := commandContext(
		ctx,
		r.ffmpegBin,
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-nostdin",
		"-i", inputPath,
		"-af", "afade=t=in:d="+fade+",areverse,afade=t=in:d="+fade+",areverse",
		"-vn",
		"-ac", "1",
		"-ar", "24000",
		"-c:a", "pcm_s16le",
		outputPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("generator/renderer: ffmpeg fade failed: %s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return nil
}

func (r *Renderer) generateSilence(ctx context.Context, outputPath string, seconds float64) error {
	commandContext := r.commandContext
	if commandContext == nil {
		commandContext = exec.CommandContext
	}
	cmd := commandContext(
		ctx,
		r.ffmpegBin,
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-nostdin",
		"-f", "lavfi",
		"-i", "anullsrc=r=24000:cl=mono",
		"-t", strconv.FormatFloat(seconds, 'f', -1, 64),
		"-c:a", "pcm_s16le",
		outputPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("generator/renderer: generate silence failed: %s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return nil
}

func (r *Renderer) maxChunkWords() int {
	if r.chunkWords > 0 {
		return r.chunkWords
	}
	return defaultChunkWords
}

func (r *Renderer) cleanupFiles(paths []string) {
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		_ = os.Remove(path)
	}
}

func (r *Renderer) debugDumpChunk(outputPath, label string, index int, voice, text, audioPath string) error {
	if strings.TrimSpace(r.debugChunkDir) == "" {
		return nil
	}

	stem := strings.TrimSuffix(filepath.Base(outputPath), filepath.Ext(outputPath))
	dir := filepath.Join(r.debugChunkDir, stem)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("generator/renderer: create chunk debug dir: %w", err)
	}

	base := fmt.Sprintf("%s_%03d", label, index)
	metaPath := filepath.Join(dir, base+".txt")
	audioDebugPath := filepath.Join(dir, base+filepath.Ext(audioPath))

	var builder strings.Builder
	builder.WriteString("voice=")
	builder.WriteString(voice)
	builder.WriteString("\n")
	builder.WriteString("text=\n")
	builder.WriteString(text)
	builder.WriteString("\n")

	if err := os.WriteFile(metaPath, []byte(builder.String()), 0o644); err != nil {
		return fmt.Errorf("generator/renderer: write chunk debug text: %w", err)
	}

	audio, err := os.ReadFile(audioPath)
	if err != nil {
		return fmt.Errorf("generator/renderer: read chunk debug audio: %w", err)
	}
	if err := os.WriteFile(audioDebugPath, audio, 0o644); err != nil {
		return fmt.Errorf("generator/renderer: write chunk debug audio: %w", err)
	}
	return nil
}

func speakerVoice(speaker, hostVoice, guestVoice string) string {
	switch speaker {
	case "HOST", "HOST_A", "主持人", "主持人甲":
		return hostVoice
	case "GUEST", "HOST_B", "嘉宾", "主持人乙":
		return guestVoice
	default:
		return hostVoice
	}
}

func partUsesMultipleVoices(part ScriptPart) bool {
	seen := map[string]bool{}
	for _, speaker := range part.Speakers {
		speaker = strings.ToUpper(strings.TrimSpace(speaker))
		if speaker == "" {
			continue
		}
		seen[speaker] = true
	}
	return len(seen) > 1
}

func voiceForScriptPart(part ScriptPart, voices map[string]string) string {
	hostVoice := firstNonEmpty(voices["host"], "am_michael")
	guestVoice := firstNonEmpty(voices["guest"], "af_bella")
	if len(part.Speakers) == 0 {
		return hostVoice
	}
	return speakerVoice(strings.ToUpper(strings.TrimSpace(part.Speakers[0])), hostVoice, guestVoice)
}

func withStemSuffix(path, suffix string) string {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	return base + suffix + ext
}

func withSuffix(path, suffix string) string {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	return base + suffix
}

func moveFile(src, dst string) error {
	_ = os.Remove(dst)
	return os.Rename(src, dst)
}

func splitSentences(text string) []string {
	var sentences []string
	var current strings.Builder
	for _, r := range text {
		current.WriteRune(r)
		if isSentenceBoundary(r) {
			segment := strings.TrimSpace(current.String())
			if segment != "" {
				sentences = append(sentences, segment)
			}
			current.Reset()
		}
	}
	if tail := strings.TrimSpace(current.String()); tail != "" {
		sentences = append(sentences, tail)
	}
	return sentences
}

func splitLongSegment(text string, maxWords int) []string {
	indexes := textUnitRE.FindAllStringIndex(text, -1)
	if len(indexes) == 0 || len(indexes) <= maxWords {
		return []string{strings.TrimSpace(text)}
	}

	chunks := make([]string, 0, (len(indexes)+maxWords-1)/maxWords)
	start := 0
	for i := maxWords; i < len(indexes); i += maxWords {
		end := indexes[i-1][1]
		chunk := strings.TrimSpace(text[start:end])
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		start = end
	}
	if tail := strings.TrimSpace(text[start:]); tail != "" {
		chunks = append(chunks, tail)
	}
	return chunks
}

func countTextUnits(text string) int {
	return len(textUnitRE.FindAllString(text, -1))
}

func isSentenceBoundary(r rune) bool {
	switch r {
	case '.', '!', '?', ';', '。', '！', '？', '；', '\n':
		return true
	default:
		return false
	}
}

func normalizeSpeakerTag(tag string) string {
	tag = strings.TrimSpace(tag)
	tag = strings.TrimSuffix(tag, ":")
	tag = strings.TrimSuffix(tag, "：")
	if speakerTagRE.MatchString(tag + ":") {
		return strings.TrimSuffix(tag, ":")
	}
	return tag
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
