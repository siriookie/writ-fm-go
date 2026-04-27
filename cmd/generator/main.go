package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/writ-fm/go/internal/domain"
	gen "github.com/writ-fm/go/internal/generator"
	"github.com/writ-fm/go/internal/scheduler"
)

var (
	showFlag            string
	typeFlag            string
	topicFlag           string
	countFlag           int
	minFlag             int
	focusFlag           string
	performanceModeFlag string
	debugScriptFlag     bool
	sourceFilesFlag     []string
	sourceDirsFlag      []string
	sourceRootFlag      string
	sourceTextFlag      string
	youtubeURLsFlag     []string
	youtubeURLFileFlag  string
	youtubeLangsFlag    string
	ingestOutDirFlag    string
)

var (
	loadScheduleFn   = scheduler.LoadSchedule
	buildGeneratorFn = buildGenerator
	nowFunc          = time.Now
)

var rootCmd = &cobra.Command{
	Use:   "generator",
	Short: "WRIT-FM talk segment generation tool",
}

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate talk segments for one show or the currently scheduled show",
	RunE: func(cmd *cobra.Command, args []string) error {
		if countFlag <= 0 {
			return errors.New("--count must be greater than 0")
		}
		cfg := configFromEnv()
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
		defer stop()
		sourceMaterials, err := buildSourceMaterials(ctx, sourceFilesFlag, sourceDirsForType(sourceRootFlag, typeFlag, sourceDirsFlag), sourceTextFlag, youtubeURLsFlag, youtubeLangsFlag)
		if err != nil {
			return err
		}
		return runGenerate(ctx, cfg, showFlag, typeFlag, topicFlag, sourceMaterials, countFlag)
	},
}

var ingestYouTubeCmd = &cobra.Command{
	Use:   "ingest-youtube",
	Short: "Extract YouTube transcripts into reusable source material files",
	RunE: func(cmd *cobra.Command, args []string) error {
		urls, err := collectYouTubeURLs(youtubeURLsFlag, youtubeURLFileFlag)
		if err != nil {
			return err
		}
		if len(urls) == 0 {
			return errors.New("provide at least one --youtube-url or --youtube-url-file")
		}
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
		defer stop()
		outDir := strings.TrimSpace(ingestOutDirFlag)
		if outDir == "" {
			outDir = typeSourceDir(sourceRootFlag, typeFlag)
		}
		return runIngestYouTube(ctx, urls, outDir, youtubeLangsFlag, os.Stdout)
	},
}

var generateAllCmd = &cobra.Command{
	Use:   "generate-all",
	Short: "Generate talk segments for all shows until each reaches a minimum inventory",
	RunE: func(cmd *cobra.Command, args []string) error {
		if minFlag <= 0 {
			return errors.New("--min must be greater than 0")
		}
		cfg := configFromEnv()
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
		defer stop()
		return runGenerateAll(ctx, cfg, minFlag, typeFlag)
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Print talk segment counts per show",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStatus(configFromEnv(), os.Stdout)
	},
}

var listTypesCmd = &cobra.Command{
	Use:   "list-types",
	Short: "List supported talk segment types",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runListTypes(os.Stdout)
	},
}

var listTopicsCmd = &cobra.Command{
	Use:   "list-topics",
	Short: "List topic pools for a focus area or all focus areas",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runListTopics(os.Stdout, focusFlag)
	},
}

func init() {
	generateCmd.Flags().StringVar(&showFlag, "show", "", "show ID to generate for (defaults to current show)")
	generateCmd.Flags().StringVar(&typeFlag, "type", "", "specific segment type to generate")
	generateCmd.Flags().StringVar(&topicFlag, "topic", "", "specific topic to use")
	generateCmd.Flags().StringArrayVar(&sourceFilesFlag, "source-file", nil, "source material file to ground the episode; may be repeated")
	generateCmd.Flags().StringArrayVar(&sourceDirsFlag, "source-dir", nil, "directory of source material files to ground the episode; may be repeated")
	generateCmd.Flags().StringVar(&sourceRootFlag, "source-root", "", "root directory for type-specific source materials (default: GENERATOR_SOURCE_ROOT or sources)")
	generateCmd.Flags().StringVar(&sourceTextFlag, "source-text", "", "inline source material to ground the episode")
	generateCmd.Flags().StringArrayVar(&youtubeURLsFlag, "youtube-url", nil, "YouTube URL to extract transcript source material with yt-dlp; may be repeated")
	generateCmd.Flags().StringVar(&youtubeLangsFlag, "youtube-langs", "", "comma-separated YouTube transcript language priority (default: YOUTUBE_TRANSCRIPT_LANGS or en-en,en-AU,en-CA,en-IN,en-IE,en-GB,en-US,en-orig)")
	generateCmd.Flags().IntVar(&countFlag, "count", 1, "number of segments to generate")
	generateCmd.Flags().StringVar(&performanceModeFlag, "performance-mode", "constrained", "performance cue mode: constrained or expressive")
	generateCmd.Flags().BoolVar(&debugScriptFlag, "debug-script", false, "log the raw generated script before TTS rendering")

	generateAllCmd.Flags().IntVar(&minFlag, "min", 3, "minimum number of talk segments per show")
	generateAllCmd.Flags().StringVar(&typeFlag, "type", "", "specific segment type to generate for all shows")
	generateAllCmd.Flags().StringVar(&performanceModeFlag, "performance-mode", "constrained", "performance cue mode: constrained or expressive")
	generateAllCmd.Flags().BoolVar(&debugScriptFlag, "debug-script", false, "log the raw generated script before TTS rendering")

	listTopicsCmd.Flags().StringVar(&focusFlag, "focus", "", "topic focus to list")

	ingestYouTubeCmd.Flags().StringArrayVar(&youtubeURLsFlag, "youtube-url", nil, "YouTube URL to extract transcript source material with yt-dlp; may be repeated")
	ingestYouTubeCmd.Flags().StringVar(&youtubeURLFileFlag, "youtube-url-file", "", "file containing one YouTube URL per line")
	ingestYouTubeCmd.Flags().StringVar(&youtubeLangsFlag, "youtube-langs", "", "comma-separated YouTube transcript language priority (default: YOUTUBE_TRANSCRIPT_LANGS or en-en,en-AU,en-CA,en-IN,en-IE,en-GB,en-US,en-orig)")
	ingestYouTubeCmd.Flags().StringVar(&typeFlag, "type", "", "segment type source folder to write into, such as story or deep_dive")
	ingestYouTubeCmd.Flags().StringVar(&sourceRootFlag, "source-root", "", "root directory for type-specific source materials (default: GENERATOR_SOURCE_ROOT or sources)")
	ingestYouTubeCmd.Flags().StringVar(&ingestOutDirFlag, "out-dir", "", "explicit output directory for extracted transcript files")

	rootCmd.AddCommand(generateCmd, ingestYouTubeCmd, generateAllCmd, statusCmd, listTypesCmd, listTopicsCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatalf("generator: %v", err)
	}
}

func runGenerate(ctx context.Context, cfg config, showID, segmentType, topic, sourceMaterials string, count int) error {
	sched, err := loadScheduleFn(cfg.SchedulePath)
	if err != nil {
		return fmt.Errorf("load schedule: %w", err)
	}

	show, err := resolveShow(sched, showID)
	if err != nil {
		return err
	}
	service, err := buildGeneratorFn(cfg)
	if err != nil {
		return err
	}

	for i := range count {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		req, err := buildGenerateRequest(show, segmentType, topic, sourceMaterials)
		if err != nil {
			return err
		}
		log.Printf("generator: generating %d/%d for %s (type=%s)", i+1, count, show.ShowID, req.SegmentType)
		result, err := service.Generate(ctx, req)
		if err != nil {
			return fmt.Errorf("segment %d: %w", i+1, err)
		}
		if shouldDebugScript(cfg) {
			log.Printf("generator: raw script for %s/%s:\n%s", show.ShowID, req.SegmentType, result.Script)
		}
		log.Printf("generator: wrote %s (%.1fs, %d chars)", filepath.Base(result.AudioPath), result.Duration, result.WordCount)
	}
	return nil
}

func runGenerateAll(ctx context.Context, cfg config, min int, segmentType string) error {
	sched, err := loadScheduleFn(cfg.SchedulePath)
	if err != nil {
		return fmt.Errorf("load schedule: %w", err)
	}
	service, err := buildGeneratorFn(cfg)
	if err != nil {
		return err
	}

	showIDs := sortedShowIDs(sched)
	for _, showID := range showIDs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		show := toSchedulerShow(sched.Shows[showID])
		have := countSegments(cfg.TalkSegmentsDir, show.ShowID)
		need := min - have
		if need <= 0 {
			log.Printf("generator: %s already has %d segments (min=%d), skipping", show.ShowID, have, min)
			continue
		}
		log.Printf("generator: %s has %d segments, generating %d more", show.ShowID, have, need)
		for i := range need {
			req, err := buildGenerateRequest(show, segmentType, "", "")
			if err != nil {
				return err
			}
			result, err := service.Generate(ctx, req)
			if err != nil {
				log.Printf("generator: %s segment %d/%d failed: %v", show.ShowID, i+1, need, err)
				continue
			}
			if shouldDebugScript(cfg) {
				log.Printf("generator: raw script for %s/%s:\n%s", show.ShowID, req.SegmentType, result.Script)
			}
			log.Printf("generator: %s wrote %s (%.1fs, %d chars)", show.ShowID, filepath.Base(result.AudioPath), result.Duration, result.WordCount)
		}
	}
	return nil
}

func runStatus(cfg config, w io.Writer) error {
	sched, err := loadScheduleFn(cfg.SchedulePath)
	if err != nil {
		return fmt.Errorf("load schedule: %w", err)
	}

	fmt.Fprintf(w, "%-25s %-12s %-20s %s\n", "SHOW", "SEGMENTS", "HOST", "FOCUS")
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 78))
	for _, showID := range sortedShowIDs(sched) {
		show := sched.Shows[showID]
		n := countSegments(cfg.TalkSegmentsDir, show.ShowID)
		fmt.Fprintf(w, "%-25s %-12d %-20s %s\n", show.ShowID, n, show.Host, show.TopicFocus)
	}
	return nil
}

func runListTypes(w io.Writer) error {
	keys := make([]string, 0, len(gen.SegmentLengthTargets))
	for key := range gen.SegmentLengthTargets {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	fmt.Fprintf(w, "%-20s %s\n", "SEGMENT TYPE", "TARGET CHARS")
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 40))
	for _, key := range keys {
		target := gen.SegmentLengthTargets[key]
		fmt.Fprintf(w, "%-20s %d-%d\n", key, target.Min, target.Max)
	}
	return nil
}

func runListTopics(w io.Writer, focus string) error {
	if strings.TrimSpace(focus) != "" {
		pool, ok := gen.TopicPools[focus]
		if !ok {
			return fmt.Errorf("unknown focus %q (available: %v)", focus, sortedTopicFocuses())
		}
		fmt.Fprintf(w, "[%s]\n", focus)
		for _, topic := range pool {
			fmt.Fprintf(w, "- %s\n", topic)
		}
		return nil
	}

	for _, key := range sortedTopicFocuses() {
		fmt.Fprintf(w, "[%s]\n", key)
		for _, topic := range gen.TopicPools[key] {
			fmt.Fprintf(w, "- %s\n", topic)
		}
	}
	return nil
}

func resolveShow(sched *scheduler.StationSchedule, showID string) (*schedulerShow, error) {
	if strings.TrimSpace(showID) != "" {
		show, ok := sched.Shows[showID]
		if !ok {
			return nil, fmt.Errorf("show %q not found in schedule", showID)
		}
		return toSchedulerShow(show), nil
	}

	resolved, err := sched.Resolve(nowFunc())
	if err != nil {
		return nil, fmt.Errorf("resolve current show: %w", err)
	}
	return &schedulerShow{
		ShowID:       resolved.ShowID,
		Name:         resolved.Name,
		Description:  resolved.Description,
		Host:         resolved.Host,
		TopicFocus:   resolved.TopicFocus,
		SegmentTypes: append([]string(nil), resolved.SegmentTypes...),
		Voices:       cloneVoices(resolved.Voices),
	}, nil
}

func toSchedulerShow(show *domain.Show) *schedulerShow {
	return &schedulerShow{
		ShowID:       show.ShowID,
		Name:         show.Name,
		Description:  show.Description,
		Host:         show.Host,
		TopicFocus:   show.TopicFocus,
		SegmentTypes: append([]string(nil), show.SegmentTypes...),
		Voices:       cloneVoices(show.Voices),
	}
}

type schedulerShow struct {
	ShowID       string
	Name         string
	Description  string
	Host         string
	TopicFocus   string
	SegmentTypes []string
	Voices       map[string]string
}

func buildGenerateRequest(show *schedulerShow, segmentType, topic, sourceMaterials string) (gen.GenerateRequest, error) {
	segmentType = strings.TrimSpace(segmentType)
	if segmentType == "" {
		if len(show.SegmentTypes) == 0 {
			return gen.GenerateRequest{}, fmt.Errorf("show %q has no segment types", show.ShowID)
		}
		segmentType = show.SegmentTypes[rand.Intn(len(show.SegmentTypes))]
	}
	return gen.GenerateRequest{
		ShowID:          show.ShowID,
		ShowName:        show.Name,
		ShowDescription: show.Description,
		HostID:          show.Host,
		TopicFocus:      show.TopicFocus,
		SegmentType:     segmentType,
		Topic:           topic,
		SourceMaterials: strings.TrimSpace(sourceMaterials),
		Voices:          cloneVoices(show.Voices),
		PerformanceMode: gen.NormalizePerformanceMode(gen.PerformanceMode(performanceModeFlag)),
	}, nil
}

type sourceDirSpec struct {
	Path    string
	PickOne bool
}

func buildSourceMaterials(ctx context.Context, files []string, dirs []sourceDirSpec, inline string, youtubeURLs []string, youtubeLangs string) (string, error) {
	var blocks []string
	if trimmed := strings.TrimSpace(inline); trimmed != "" {
		blocks = append(blocks, formatSourceBlock("inline", trimmed, 20000))
	}
	for _, file := range files {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("read source file %q: %w", file, err)
		}
		blocks = append(blocks, formatSourceBlock(filepath.Base(file), string(data), 30000))
	}
	dirFiles, err := sourceFilesFromDirs(dirs)
	if err != nil {
		return "", err
	}
	for _, file := range dirFiles {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("read source file %q: %w", file, err)
		}
		blocks = append(blocks, formatSourceBlock(filepath.Base(file), string(data), 30000))
	}
	langs := preferredYouTubeLangs(youtubeLangs)
	for _, rawURL := range youtubeURLs {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			continue
		}
		block, err := extractYouTubeSourceMaterial(ctx, rawURL, langs)
		if err != nil {
			return "", fmt.Errorf("extract youtube source %q: %w", rawURL, err)
		}
		blocks = append(blocks, formatSourceBlock(block.Name, block.Text, 50000))
	}
	return strings.TrimSpace(strings.Join(blocks, "\n\n")), nil
}

func sourceFilesFromDirs(dirs []sourceDirSpec) ([]string, error) {
	seen := make(map[string]struct{})
	var files []string
	for _, spec := range dirs {
		dir := strings.TrimSpace(spec.Path)
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read source dir %q: %w", dir, err)
		}
		var dirFiles []string
		for _, entry := range entries {
			if entry.IsDir() || !isSourceMaterialFile(entry.Name()) {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			dirFiles = append(dirFiles, path)
		}
		slices.Sort(dirFiles)
		if spec.PickOne && len(dirFiles) > 1 {
			dirFiles = []string{dirFiles[rand.Intn(len(dirFiles))]}
		}
		for _, path := range dirFiles {
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			files = append(files, path)
		}
	}
	slices.Sort(files)
	return files, nil
}

func isSourceMaterialFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown", ".txt", ".text":
		return true
	default:
		return false
	}
}

func sourceDirsForType(root, segmentType string, explicit []string) []sourceDirSpec {
	dirs := make([]sourceDirSpec, 0, len(explicit)+2)
	for _, dir := range explicit {
		dirs = append(dirs, sourceDirSpec{Path: dir, PickOne: true})
	}
	resolvedRoot := sourceRoot(root)
	if resolvedRoot == "" {
		return dirs
	}
	dirs = append(dirs, sourceDirSpec{Path: filepath.Join(resolvedRoot, "common"), PickOne: false})
	if strings.TrimSpace(segmentType) != "" {
		dirs = append(dirs, sourceDirSpec{Path: typeSourceDir(resolvedRoot, segmentType), PickOne: true})
	}
	return dirs
}

func sourceRoot(raw string) string {
	if strings.TrimSpace(raw) != "" {
		return strings.TrimSpace(raw)
	}
	if env := strings.TrimSpace(os.Getenv("GENERATOR_SOURCE_ROOT")); env != "" {
		return env
	}
	return "sources"
}

func typeSourceDir(root, segmentType string) string {
	root = sourceRoot(root)
	segmentType = strings.TrimSpace(segmentType)
	if segmentType == "" {
		segmentType = "youtube"
	}
	return filepath.Join(root, segmentType)
}

func formatSourceBlock(name, text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	runes := []rune(text)
	truncated := false
	if maxRunes > 0 && len(runes) > maxRunes {
		text = string(runes[:maxRunes])
		truncated = true
	}
	if truncated {
		text += "..."
	}
	return fmt.Sprintf("## %s\n%s", name, text)
}

func countSegments(outputDir, showID string) int {
	entries, err := os.ReadDir(filepath.Join(outputDir, showID))
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	n := 0
	for _, e := range entries {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == ".wav" || ext == ".mp3" || ext == ".flac" {
			n++
		}
	}
	return n
}

func sortedShowIDs(sched *scheduler.StationSchedule) []string {
	ids := make([]string, 0, len(sched.Shows))
	for id := range sched.Shows {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func sortedTopicFocuses() []string {
	keys := make([]string, 0, len(gen.TopicPools))
	for key := range gen.TopicPools {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func cloneVoices(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func shouldDebugScript(cfg config) bool {
	return debugScriptFlag || cfg.DebugScript
}
