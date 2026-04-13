package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/writ-fm/go/internal/musicgen"
	"github.com/writ-fm/go/internal/scheduler"
)

type config struct {
	MusicGenURL  string
	OutputDir    string
	SchedulePath string
}

func configFromEnv() config {
	url := os.Getenv("MUSIC_GEN_URL")
	if url == "" {
		url = "http://localhost:4009"
	}
	dir := os.Getenv("BUMPER_DIR")
	if dir == "" {
		dir = "output/music_bumpers"
	}
	sched := os.Getenv("SCHEDULE_PATH")
	if sched == "" {
		sched = "config/schedule.yaml"
	}
	return config{MusicGenURL: url, OutputDir: dir, SchedulePath: sched}
}

func main() {
	showFlag := flag.String("show", "", "show ID to generate bumpers for")
	countFlag := flag.Int("count", 1, "number of bumpers to generate")
	allFlag := flag.Bool("all", false, "generate bumpers for all shows")
	minFlag := flag.Int("min", 5, "minimum bumpers per show (used with --all)")
	statusFlag := flag.Bool("status", false, "print bumper counts per show and exit")
	flag.Parse()

	cfg := configFromEnv()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	var err error
	switch {
	case *statusFlag:
		err = runStatus(cfg, os.Stdout)
	case *allFlag:
		err = runGenerateAll(ctx, cfg, *minFlag)
	case *showFlag != "":
		err = runGenerate(ctx, cfg, *showFlag, *countFlag)
	default:
		fmt.Fprintln(os.Stderr, "usage: musicgen --show <id> [--count n] | --all [--min n] | --status")
		os.Exit(1)
	}

	if err != nil {
		log.Fatalf("musicgen: %v", err)
	}
}

// runGenerate generates count bumpers for a single show.
func runGenerate(ctx context.Context, cfg config, showID string, count int) error {
	style, err := bumperStyleForShow(cfg.SchedulePath, showID)
	if err != nil {
		return err
	}

	client := musicgen.NewClient(cfg.MusicGenURL)
	gen := musicgen.NewBumperGenerator(client, cfg.OutputDir)

	for i := range count {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		log.Printf("musicgen: generating bumper %d/%d for %s (style=%s)", i+1, count, showID, style)
		meta, err := gen.Generate(ctx, showID, style)
		if err != nil {
			return fmt.Errorf("bumper %d: %w", i+1, err)
		}
		log.Printf("musicgen: wrote %s (%.0fs generation)", meta.Filename, meta.GenerationSeconds)
	}
	return nil
}

// runGenerateAll generates bumpers for every show until each has at least min.
func runGenerateAll(ctx context.Context, cfg config, min int) error {
	sched, err := scheduler.LoadSchedule(cfg.SchedulePath)
	if err != nil {
		return fmt.Errorf("load schedule: %w", err)
	}
	shows := sched.Shows

	client := musicgen.NewClient(cfg.MusicGenURL)
	gen := musicgen.NewBumperGenerator(client, cfg.OutputDir)

	for _, show := range shows {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		have := countBumpers(cfg.OutputDir, show.ShowID)
		need := min - have
		if need <= 0 {
			log.Printf("musicgen: %s already has %d bumpers (min=%d), skipping", show.ShowID, have, min)
			continue
		}
		log.Printf("musicgen: %s has %d bumpers, generating %d more (target=%d)", show.ShowID, have, need, min)
		for i := range need {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			meta, err := gen.Generate(ctx, show.ShowID, show.BumperStyle)
			if err != nil {
				log.Printf("musicgen: %s bumper %d/%d failed: %v", show.ShowID, i+1, need, err)
				continue
			}
			log.Printf("musicgen: %s wrote %s (%.0fs)", show.ShowID, meta.Filename, meta.GenerationSeconds)
		}
	}
	return nil
}

// runStatus prints bumper counts per show to w.
func runStatus(cfg config, w io.Writer) error {
	sched, err := scheduler.LoadSchedule(cfg.SchedulePath)
	if err != nil {
		return fmt.Errorf("load schedule: %w", err)
	}
	shows := sched.Shows

	fmt.Fprintf(w, "%-25s %-12s %s\n", "SHOW", "BUMPERS", "STYLE")
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 50))
	for _, show := range shows {
		n := countBumpers(cfg.OutputDir, show.ShowID)
		fmt.Fprintf(w, "%-25s %-12d %s\n", show.ShowID, n, show.BumperStyle)
	}
	return nil
}

// bumperStyleForShow looks up the bumper style for showID in the schedule.
func bumperStyleForShow(schedulePath, showID string) (string, error) {
	sched, err := scheduler.LoadSchedule(schedulePath)
	if err != nil {
		return "", fmt.Errorf("load schedule: %w", err)
	}
	for _, show := range sched.Shows {
		if show.ShowID == showID {
			return show.BumperStyle, nil
		}
	}
	return "", fmt.Errorf("show %q not found in schedule", showID)
}

// countBumpers returns the number of audio files in the show's bumper directory.
func countBumpers(outputDir, showID string) int {
	entries, err := os.ReadDir(filepath.Join(outputDir, showID))
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	n := 0
	for _, e := range entries {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == ".flac" || ext == ".mp3" || ext == ".wav" {
			n++
		}
	}
	return n
}
