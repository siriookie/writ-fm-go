package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

const defaultYouTubeLangs = "en-en,en-AU,en-CA,en-IN,en-IE,en-GB,en-US,en-orig"

type sourceBlock struct {
	Name string
	Text string
}

type youtubeSource struct {
	ID               string
	Title            string
	URL              string
	Duration         float64
	TranscriptLang   string
	TranscriptSource string
	Transcript       string
}

type youtubeMetadata struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Duration   float64 `json:"duration"`
	WebpageURL string  `json:"webpage_url"`
}

type ytDLPCommandRunner func(ctx context.Context, binary string, args ...string) ([]byte, []byte, error)

type youtubeExtractor struct {
	Binary string
	Run    ytDLPCommandRunner
}

var extractYouTubeSourceMaterial = func(ctx context.Context, url string, langs []string) (sourceBlock, error) {
	extractor := youtubeExtractor{
		Binary: resolveYTDLPPath(),
		Run:    runYTDLPCommand,
	}
	source, err := extractor.Extract(ctx, url, langs)
	if err != nil {
		return sourceBlock{}, err
	}
	return sourceBlock{
		Name: "YouTube: " + fallbackString(source.Title, source.ID, url),
		Text: formatYouTubeSource(source),
	}, nil
}

func collectYouTubeURLs(urls []string, urlFile string) ([]string, error) {
	seen := make(map[string]struct{})
	var out []string
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || strings.HasPrefix(value, "#") {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	for _, url := range urls {
		add(url)
	}
	if strings.TrimSpace(urlFile) != "" {
		data, err := os.ReadFile(urlFile)
		if err != nil {
			return nil, fmt.Errorf("read youtube url file %q: %w", urlFile, err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			add(line)
		}
	}
	return out, nil
}

func runIngestYouTube(ctx context.Context, urls []string, outDir string, youtubeLangs string, w io.Writer) error {
	outDir = strings.TrimSpace(outDir)
	if outDir == "" {
		return errors.New("empty output directory")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir %q: %w", outDir, err)
	}
	langs := preferredYouTubeLangs(youtubeLangs)
	for i, url := range urls {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		block, err := extractYouTubeSourceMaterial(ctx, url, langs)
		if err != nil {
			return fmt.Errorf("extract youtube source %q: %w", url, err)
		}
		filename := fmt.Sprintf("%02d_%s.md", i+1, sanitizeFileStem(strings.TrimPrefix(block.Name, "YouTube: ")))
		path := nextAvailablePath(filepath.Join(outDir, filename))
		content := formatSourceBlock(block.Name, block.Text, 0) + "\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write youtube transcript %q: %w", path, err)
		}
		if w != nil {
			fmt.Fprintf(w, "wrote %s\n", path)
		}
	}
	return nil
}

func sanitizeFileStem(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "youtube_transcript"
	}
	name = regexp.MustCompile(`[\\/:*?"<>|]+`).ReplaceAllString(name, "_")
	name = regexp.MustCompile(`\s+`).ReplaceAllString(name, "_")
	name = strings.Trim(name, "._- ")
	runes := []rune(name)
	if len(runes) > 80 {
		name = string(runes[:80])
	}
	if name == "" {
		return "youtube_transcript"
	}
	return name
}

func nextAvailablePath(path string) string {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return path
	}
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s_%d%s", stem, i, ext)
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
}

func preferredYouTubeLangs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		raw = os.Getenv("YOUTUBE_TRANSCRIPT_LANGS")
	}
	if strings.TrimSpace(raw) == "" {
		raw = defaultYouTubeLangs
	}
	parts := strings.Split(raw, ",")
	langs := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" && !slices.Contains(langs, part) {
			langs = append(langs, part)
		}
	}
	if len(langs) == 0 {
		return strings.Split(defaultYouTubeLangs, ",")
	}
	return langs
}

func resolveYTDLPPath() string {
	if path := strings.TrimSpace(os.Getenv("YTDLP_PATH")); path != "" {
		return path
	}
	if path, err := exec.LookPath("yt-dlp"); err == nil {
		return path
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidate := filepath.Join(home, "yt-dlp", "yt-dlp.exe")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "yt-dlp"
}

func (e youtubeExtractor) Extract(ctx context.Context, url string, langs []string) (youtubeSource, error) {
	if strings.TrimSpace(url) == "" {
		return youtubeSource{}, errors.New("empty youtube url")
	}
	if len(langs) == 0 {
		langs = preferredYouTubeLangs("")
	}
	run := e.Run
	if run == nil {
		run = runYTDLPCommand
	}
	binary := strings.TrimSpace(e.Binary)
	if binary == "" {
		binary = resolveYTDLPPath()
	}

	meta, err := e.fetchMetadata(ctx, run, binary, url)
	if err != nil {
		return youtubeSource{}, err
	}
	dir, err := os.MkdirTemp("", "writ-fm-youtube-*")
	if err != nil {
		return youtubeSource{}, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	transcript, lang, err := e.fetchTranscript(ctx, run, binary, dir, url, meta.ID, langs, "manual")
	sourceKind := "manual"
	if err != nil || strings.TrimSpace(transcript) == "" {
		transcript, lang, err = e.fetchTranscript(ctx, run, binary, dir, url, meta.ID, langs, "auto")
		sourceKind = "auto"
	}
	if err != nil {
		return youtubeSource{}, err
	}
	if strings.TrimSpace(transcript) == "" {
		return youtubeSource{}, fmt.Errorf("no usable transcript found for preferred languages %s", strings.Join(langs, ","))
	}

	return youtubeSource{
		ID:               meta.ID,
		Title:            meta.Title,
		URL:              fallbackString(meta.WebpageURL, url),
		Duration:         meta.Duration,
		TranscriptLang:   lang,
		TranscriptSource: sourceKind,
		Transcript:       transcript,
	}, nil
}

func (e youtubeExtractor) fetchMetadata(ctx context.Context, run ytDLPCommandRunner, binary, url string) (youtubeMetadata, error) {
	stdout, stderr, err := runYTDLPWithRetry(ctx, run, binary, "--dump-single-json", "--skip-download", "--no-warnings", url)
	if err != nil {
		return youtubeMetadata{}, fmt.Errorf("yt-dlp metadata: %w%s", err, formatStderr(stderr))
	}
	var meta youtubeMetadata
	if err := json.Unmarshal(bytes.TrimSpace(stdout), &meta); err != nil {
		return youtubeMetadata{}, fmt.Errorf("parse yt-dlp metadata JSON: %w%s", err, formatStderr(stderr))
	}
	if strings.TrimSpace(meta.ID) == "" && strings.TrimSpace(meta.Title) == "" {
		return youtubeMetadata{}, errors.New("yt-dlp metadata missing id and title")
	}
	return meta, nil
}

func (e youtubeExtractor) fetchTranscript(ctx context.Context, run ytDLPCommandRunner, binary, dir, url, videoID string, langs []string, kind string) (string, string, error) {
	args := []string{
		"--skip-download",
		"--sub-langs", strings.Join(langs, ","),
		"--sub-format", "vtt",
		"--convert-subs", "vtt",
		"-P", dir,
		"-o", "%(id)s.%(ext)s",
		"--no-warnings",
	}
	switch kind {
	case "manual":
		args = append(args, "--write-subs")
	case "auto":
		args = append(args, "--write-auto-subs")
	default:
		return "", "", fmt.Errorf("unknown transcript kind %q", kind)
	}
	args = append(args, url)

	before := existingVTTFiles(dir)
	_, stderr, err := runYTDLPWithRetry(ctx, run, binary, args...)
	if err != nil {
		return "", "", fmt.Errorf("yt-dlp %s subtitles: %w%s", kind, err, formatStderr(stderr))
	}
	file, lang, err := pickSubtitleFile(dir, before, videoID, langs)
	if err != nil {
		return "", "", err
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return "", "", fmt.Errorf("read subtitle file: %w", err)
	}
	return cleanVTTTranscript(string(data)), lang, nil
}

func runYTDLPCommand(ctx context.Context, binary string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	return stdout, stderr.Bytes(), err
}

func runYTDLPWithRetry(ctx context.Context, run ytDLPCommandRunner, binary string, args ...string) ([]byte, []byte, error) {
	var stdout, stderr []byte
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		stdout, stderr, err = run(ctx, binary, args...)
		if err == nil {
			return stdout, stderr, nil
		}
		if attempt < 3 {
			timer := time.NewTimer(time.Duration(attempt) * 300 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, nil, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return stdout, stderr, err
}

func existingVTTFiles(dir string) map[string]struct{} {
	files := make(map[string]struct{})
	matches, _ := filepath.Glob(filepath.Join(dir, "*.vtt"))
	for _, match := range matches {
		files[match] = struct{}{}
	}
	return files
}

func pickSubtitleFile(dir string, before map[string]struct{}, videoID string, langs []string) (string, string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.vtt"))
	if err != nil {
		return "", "", fmt.Errorf("list subtitle files: %w", err)
	}
	type candidate struct {
		path string
		lang string
	}
	var candidates []candidate
	for _, match := range matches {
		if _, existed := before[match]; existed {
			continue
		}
		lang := subtitleLang(filepath.Base(match), videoID)
		candidates = append(candidates, candidate{path: match, lang: lang})
	}
	if len(candidates) == 0 {
		return "", "", errors.New("no subtitle file produced by yt-dlp")
	}
	for _, lang := range langs {
		for _, c := range candidates {
			if strings.EqualFold(c.lang, lang) {
				return c.path, c.lang, nil
			}
		}
	}
	return candidates[0].path, candidates[0].lang, nil
}

func subtitleLang(filename, videoID string) string {
	name := strings.TrimSuffix(filename, filepath.Ext(filename))
	prefix := strings.TrimSpace(videoID) + "."
	if strings.TrimSpace(videoID) != "" && strings.HasPrefix(name, prefix) {
		return strings.TrimPrefix(name, prefix)
	}
	if idx := strings.LastIndex(name, "."); idx >= 0 && idx < len(name)-1 {
		return name[idx+1:]
	}
	return ""
}

var (
	vttTimestampLine = regexp.MustCompile(`\d{2}:\d{2}:\d{2}\.\d{3}\s+-->\s+\d{2}:\d{2}:\d{2}\.\d{3}`)
	vttCueIndexLine  = regexp.MustCompile(`^\d+$`)
	vttTag           = regexp.MustCompile(`<[^>]+>`)
	vttWhitespace    = regexp.MustCompile(`\s+`)
)

func cleanVTTTranscript(raw string) string {
	var lines []string
	last := ""
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" ||
			strings.HasPrefix(line, "WEBVTT") ||
			strings.HasPrefix(line, "Kind:") ||
			strings.HasPrefix(line, "Language:") ||
			strings.HasPrefix(line, "NOTE") ||
			vttTimestampLine.MatchString(line) ||
			vttCueIndexLine.MatchString(line) {
			continue
		}
		line = html.UnescapeString(vttTag.ReplaceAllString(line, ""))
		line = strings.Join(strings.Fields(vttWhitespace.ReplaceAllString(line, " ")), " ")
		if line == "" {
			continue
		}
		key := strings.ToLower(line)
		if key == last {
			continue
		}
		lines = append(lines, line)
		last = key
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func formatYouTubeSource(source youtubeSource) string {
	var b strings.Builder
	if strings.TrimSpace(source.Title) != "" {
		fmt.Fprintf(&b, "标题：%s\n", source.Title)
	}
	if strings.TrimSpace(source.URL) != "" {
		fmt.Fprintf(&b, "链接：%s\n", source.URL)
	}
	if strings.TrimSpace(source.ID) != "" {
		fmt.Fprintf(&b, "Video ID：%s\n", source.ID)
	}
	if source.Duration > 0 {
		fmt.Fprintf(&b, "时长：%.0f 秒\n", source.Duration)
	}
	if strings.TrimSpace(source.TranscriptLang) != "" {
		fmt.Fprintf(&b, "字幕语言：%s\n", source.TranscriptLang)
	}
	if strings.TrimSpace(source.TranscriptSource) != "" {
		fmt.Fprintf(&b, "字幕来源：%s\n", source.TranscriptSource)
	}
	fmt.Fprintf(&b, "\n转写正文：\n%s", strings.TrimSpace(source.Transcript))
	return strings.TrimSpace(b.String())
}

func fallbackString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func formatStderr(stderr []byte) string {
	text := strings.TrimSpace(string(stderr))
	if text == "" {
		return ""
	}
	return ": " + text
}
