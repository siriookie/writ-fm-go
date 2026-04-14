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
	"time"

	gentts "github.com/writ-fm/go/internal/generator/tts"
)

const (
	defaultChunkWords = 100
	defaultGapSeconds = 0.3
)

var (
	sentenceExtractRE = regexp.MustCompile(`[^.!?]+[.!?]?`)
	speakerSplitRE    = regexp.MustCompile(`((?:HOST_A|HOST_B|GUEST|HOST|[A-Z][A-Z\s.]+):)`)
	speakerTagRE      = regexp.MustCompile(`^(?:HOST_A|HOST_B|GUEST|HOST|[A-Z][A-Z\s.]+):$`)
)

// DialoguePart is one speaker-labelled portion of a multi-voice script.
type DialoguePart struct {
	Speaker string
	Text    string
}

// Renderer turns generated scripts into audio files through a TTS backend.
type Renderer struct {
	tts            gentts.Client
	ffmpegBin      string
	ffprobeBin     string
	tempDir        string
	commandContext func(context.Context, string, ...string) *exec.Cmd
}

// NewRenderer returns a renderer with production defaults.
func NewRenderer(client gentts.Client) *Renderer {
	return &Renderer{
		tts:            client,
		ffmpegBin:      "ffmpeg",
		ffprobeBin:     "ffprobe",
		commandContext: exec.CommandContext,
	}
}

// PreprocessForTTS normalizes generator markup into spoken text.
func PreprocessForTTS(text string) string {
	replacer := strings.NewReplacer(
		"[pause]", "...",
		"[chuckle]", "heh...",
		`"`, "",
	)
	text = replacer.Replace(text)
	text = strings.ReplaceAll(text, "[cough]", "ahem...")
	return strings.TrimSpace(text)
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
	words := strings.Fields(script)
	if len(words) <= maxWords {
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
		sentenceWords := len(strings.Fields(sentence))
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
		tag := strings.TrimSuffix(strings.TrimSpace(script[match[0]:match[1]]), ":")
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
func (r *Renderer) RenderSingle(ctx context.Context, script, voice, outputPath string) error {
	script = PreprocessForTTS(script)
	chunks := SplitIntoChunks(script, defaultChunkWords)
	if len(chunks) == 0 {
		return fmt.Errorf("generator/renderer: no text to render")
	}

	chunkFiles, err := r.renderChunks(ctx, chunks, voice, outputPath, "chunk")
	if err != nil {
		return err
	}
	return r.concatenateAudio(ctx, chunkFiles, outputPath, 0)
}

// RenderMulti renders a multi-voice script to outputPath.
func (r *Renderer) RenderMulti(ctx context.Context, script string, voices map[string]string, outputPath string) error {
	parts := ParseDialogue(script)
	if len(parts) == 0 {
		return r.RenderSingle(ctx, script, voices["host"], outputPath)
	}

	hostVoice := firstNonEmpty(voices["host"], "am_michael")
	guestVoice := firstNonEmpty(voices["guest"], "af_bella")

	rendered := make([]string, 0, len(parts))
	for i, part := range parts {
		text := PreprocessForTTS(part.Text)
		if text == "" {
			continue
		}
		voice := speakerVoice(part.Speaker, hostVoice, guestVoice)
		partPath := withStemSuffix(outputPath, fmt.Sprintf("_part%03d", i))
		if len(strings.Fields(text)) > defaultChunkWords {
			if err := r.RenderSingle(ctx, text, voice, partPath); err != nil {
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

func (r *Renderer) renderChunks(ctx context.Context, chunks []string, voice, outputPath, label string) ([]string, error) {
	files := make([]string, 0, len(chunks))
	for i, chunk := range chunks {
		chunkPath := withStemSuffix(outputPath, fmt.Sprintf("_%s%03d", label, i))
		if err := r.synthesizeToFile(ctx, chunk, voice, chunkPath); err != nil {
			r.cleanupFiles(files)
			return nil, err
		}
		files = append(files, chunkPath)
	}
	return files, nil
}

func (r *Renderer) synthesizeToFile(ctx context.Context, text, voice, path string) error {
	if r.tts == nil {
		return fmt.Errorf("generator/renderer: TTS client is required")
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

	files := append([]string(nil), chunkFiles...)
	if gapSeconds > 0 && len(chunkFiles) > 1 {
		silencePath := withStemSuffix(outputPath, "_gap")
		if err := r.generateSilence(ctx, silencePath, gapSeconds); err != nil {
			r.cleanupFiles(chunkFiles)
			return err
		}
		var expanded []string
		for i, file := range chunkFiles {
			expanded = append(expanded, file)
			if i < len(chunkFiles)-1 {
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
	cmd := commandContext(ctx, r.ffmpegBin, "-y", "-f", "concat", "-safe", "0", "-i", listFile, "-c", "copy", outputPath)
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
		builder.WriteString("file '")
		builder.WriteString(strings.ReplaceAll(file, "'", "'\\''"))
		builder.WriteString("'\n")
	}
	if err := os.WriteFile(listFile, []byte(builder.String()), 0o644); err != nil {
		return "", fmt.Errorf("generator/renderer: write concat list: %w", err)
	}
	return listFile, nil
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

func (r *Renderer) cleanupFiles(paths []string) {
	for _, path := range paths {
		_ = os.Remove(path)
	}
}

func speakerVoice(speaker, hostVoice, guestVoice string) string {
	switch speaker {
	case "HOST", "HOST_A":
		return hostVoice
	case "GUEST", "HOST_B":
		return guestVoice
	default:
		return hostVoice
	}
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
	matches := sentenceExtractRE.FindAllString(text, -1)
	sentences := make([]string, 0, len(matches))
	for _, match := range matches {
		match = strings.TrimSpace(match)
		if match != "" {
			sentences = append(sentences, match)
		}
	}
	return sentences
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
