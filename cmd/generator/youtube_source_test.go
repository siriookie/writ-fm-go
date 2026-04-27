package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPreferredYouTubeLangs(t *testing.T) {
	t.Setenv("YOUTUBE_TRANSCRIPT_LANGS", "en, zh, en")

	got := preferredYouTubeLangs("")
	want := []string{"en", "zh"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("preferredYouTubeLangs() = %#v, want %#v", got, want)
	}

	got = preferredYouTubeLangs("ja,ko")
	want = []string{"ja", "ko"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("preferredYouTubeLangs(explicit) = %#v, want %#v", got, want)
	}
}

func TestPreferredYouTubeLangsDefaultUsesEnglishVariants(t *testing.T) {
	t.Setenv("YOUTUBE_TRANSCRIPT_LANGS", "")

	got := preferredYouTubeLangs("")
	want := []string{"en-en", "en-AU", "en-CA", "en-IN", "en-IE", "en-GB", "en-US", "en-orig"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("preferredYouTubeLangs(default) = %#v, want %#v", got, want)
	}
}

func TestCleanVTTTranscript(t *testing.T) {
	raw := `WEBVTT
Kind: captions
Language: en

1
00:00:01.000 --> 00:00:03.000
<c>Hello&nbsp;world</c>

2
00:00:03.000 --> 00:00:05.000
Hello world

3
00:00:05.000 --> 00:00:07.000
<00:00:05.500>下一句 <b>开始</b>
`
	got := cleanVTTTranscript(raw)
	want := "Hello world\n下一句 开始"
	if got != want {
		t.Fatalf("cleanVTTTranscript() = %q, want %q", got, want)
	}
}

func TestYouTubeExtractorPrefersManualSubtitle(t *testing.T) {
	dir := ""
	var commands [][]string
	run := func(ctx context.Context, binary string, args ...string) ([]byte, []byte, error) {
		_ = ctx
		_ = binary
		commands = append(commands, append([]string(nil), args...))
		if len(args) > 0 && args[0] == "--dump-single-json" {
			return []byte(`{"id":"abc123def45","title":"A careful lecture","duration":123,"webpage_url":"https://youtu.be/abc123def45"}`), nil, nil
		}
		for i, arg := range args {
			if arg == "-P" && i+1 < len(args) {
				dir = args[i+1]
				break
			}
		}
		if slicesContains(args, "--write-subs") {
			return nil, nil, os.WriteFile(filepath.Join(dir, "abc123def45.zh-Hans.vtt"), []byte("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\n人工字幕\n"), 0o644)
		}
		t.Fatalf("unexpected auto subtitle fallback: %#v", args)
		return nil, nil, nil
	}

	got, err := youtubeExtractor{Binary: "yt-dlp", Run: run}.Extract(context.Background(), "https://youtu.be/abc123def45", []string{"zh-Hans", "en"})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got.Transcript != "人工字幕" || got.TranscriptSource != "manual" || got.TranscriptLang != "zh-Hans" {
		t.Fatalf("unexpected source: %#v", got)
	}
	if len(commands) != 2 {
		t.Fatalf("commands = %d, want metadata + manual subtitle", len(commands))
	}
}

func TestYouTubeExtractorFallsBackToAutoSubtitle(t *testing.T) {
	run := func(ctx context.Context, binary string, args ...string) ([]byte, []byte, error) {
		_ = ctx
		_ = binary
		if len(args) > 0 && args[0] == "--dump-single-json" {
			return []byte(`{"id":"abc123def45","title":"Auto only"}`), nil, nil
		}
		dir := ""
		for i, arg := range args {
			if arg == "-P" && i+1 < len(args) {
				dir = args[i+1]
				break
			}
		}
		if slicesContains(args, "--write-auto-subs") {
			return nil, nil, os.WriteFile(filepath.Join(dir, "abc123def45.en.vtt"), []byte("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nAuto transcript\n"), 0o644)
		}
		return nil, nil, nil
	}

	got, err := youtubeExtractor{Binary: "yt-dlp", Run: run}.Extract(context.Background(), "https://youtu.be/abc123def45", []string{"zh-Hans", "en"})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got.Transcript != "Auto transcript" || got.TranscriptSource != "auto" || got.TranscriptLang != "en" {
		t.Fatalf("unexpected source: %#v", got)
	}
}

func TestBuildSourceMaterialsIncludesYouTubeTranscript(t *testing.T) {
	orig := extractYouTubeSourceMaterial
	extractYouTubeSourceMaterial = func(ctx context.Context, url string, langs []string) (sourceBlock, error) {
		_ = ctx
		if url != "https://youtu.be/abc123def45" {
			t.Fatalf("url = %q", url)
		}
		if !reflect.DeepEqual(langs, []string{"zh", "en"}) {
			t.Fatalf("langs = %#v", langs)
		}
		return sourceBlock{Name: "YouTube: 示例视频", Text: "标题：示例视频\n\n转写正文：这里是完整转写。"}, nil
	}
	defer func() { extractYouTubeSourceMaterial = orig }()

	got, err := buildSourceMaterials(context.Background(), nil, nil, "", []string{"https://youtu.be/abc123def45"}, "zh,en")
	if err != nil {
		t.Fatalf("buildSourceMaterials() error = %v", err)
	}
	for _, want := range []string{"## YouTube: 示例视频", "转写正文", "这里是完整转写"} {
		if !strings.Contains(got, want) {
			t.Fatalf("source materials missing %q:\n%s", want, got)
		}
	}
}

func TestRunIngestYouTubeWritesTranscriptFiles(t *testing.T) {
	orig := extractYouTubeSourceMaterial
	extractYouTubeSourceMaterial = func(ctx context.Context, url string, langs []string) (sourceBlock, error) {
		_ = ctx
		if url != "https://youtu.be/abc123def45" {
			t.Fatalf("url = %q", url)
		}
		if !reflect.DeepEqual(langs, []string{"zh", "en"}) {
			t.Fatalf("langs = %#v", langs)
		}
		return sourceBlock{Name: "YouTube: Bad/File:Name", Text: "标题：Bad/File:Name\n\n转写正文：内容"}, nil
	}
	defer func() { extractYouTubeSourceMaterial = orig }()

	dir := t.TempDir()
	if err := runIngestYouTube(context.Background(), []string{"https://youtu.be/abc123def45"}, dir, "zh,en", nil); err != nil {
		t.Fatalf("runIngestYouTube() error = %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if strings.ContainsAny(entries[0].Name(), `\/:*?"<>|`) {
		t.Fatalf("filename was not sanitized: %q", entries[0].Name())
	}
	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), "## YouTube: Bad/File:Name") || !strings.Contains(string(data), "转写正文：内容") {
		t.Fatalf("unexpected file content:\n%s", string(data))
	}
}

func TestCollectYouTubeURLsDeduplicatesFileAndFlags(t *testing.T) {
	file := filepath.Join(t.TempDir(), "urls.txt")
	if err := os.WriteFile(file, []byte("https://youtu.be/a\n# comment\nhttps://youtu.be/b\nhttps://youtu.be/a\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	got, err := collectYouTubeURLs([]string{"https://youtu.be/a", "https://youtu.be/c"}, file)
	if err != nil {
		t.Fatalf("collectYouTubeURLs() error = %v", err)
	}
	want := []string{"https://youtu.be/a", "https://youtu.be/c", "https://youtu.be/b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("collectYouTubeURLs() = %#v, want %#v", got, want)
	}
}

func slicesContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
