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
	showFlag  string
	typeFlag  string
	topicFlag string
	countFlag int
	minFlag   int
	focusFlag string
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
		return runGenerate(ctx, cfg, showFlag, typeFlag, topicFlag, countFlag)
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
	generateCmd.Flags().IntVar(&countFlag, "count", 1, "number of segments to generate")

	generateAllCmd.Flags().IntVar(&minFlag, "min", 3, "minimum number of talk segments per show")
	generateAllCmd.Flags().StringVar(&typeFlag, "type", "", "specific segment type to generate for all shows")

	listTopicsCmd.Flags().StringVar(&focusFlag, "focus", "", "topic focus to list")

	rootCmd.AddCommand(generateCmd, generateAllCmd, statusCmd, listTypesCmd, listTopicsCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatalf("generator: %v", err)
	}
}

func runGenerate(ctx context.Context, cfg config, showID, segmentType, topic string, count int) error {
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
		req, err := buildGenerateRequest(show, segmentType, topic)
		if err != nil {
			return err
		}
		log.Printf("generator: generating %d/%d for %s (type=%s)", i+1, count, show.ShowID, req.SegmentType)
		result, err := service.Generate(ctx, req)
		if err != nil {
			return fmt.Errorf("segment %d: %w", i+1, err)
		}
		log.Printf("generator: wrote %s (%.1fs, %d words)", filepath.Base(result.AudioPath), result.Duration, result.WordCount)
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
			req, err := buildGenerateRequest(show, segmentType, "")
			if err != nil {
				return err
			}
			result, err := service.Generate(ctx, req)
			if err != nil {
				log.Printf("generator: %s segment %d/%d failed: %v", show.ShowID, i+1, need, err)
				continue
			}
			log.Printf("generator: %s wrote %s (%.1fs, %d words)", show.ShowID, filepath.Base(result.AudioPath), result.Duration, result.WordCount)
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
	keys := make([]string, 0, len(gen.SegmentWordTargets))
	for key := range gen.SegmentWordTargets {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	fmt.Fprintf(w, "%-20s %s\n", "SEGMENT TYPE", "TARGET WORDS")
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 40))
	for _, key := range keys {
		target := gen.SegmentWordTargets[key]
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

func buildGenerateRequest(show *schedulerShow, segmentType, topic string) (gen.GenerateRequest, error) {
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
		Voices:          cloneVoices(show.Voices),
	}, nil
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
