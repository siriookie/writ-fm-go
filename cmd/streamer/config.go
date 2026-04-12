package main

import (
	"errors"
	"os"
)

// Config holds the runtime configuration for the streamer process.
type Config struct {
	IcecastURL      string // e.g. icecast://source:hackme@localhost:8000/stream
	SchedulePath    string // path to schedule YAML, e.g. config/schedule.yaml
	TalkSegmentsDir string // path to output/talk_segments
	BumperDir       string // path to output/music_bumpers
}

// configFromEnv reads configuration from environment variables.
// ICECAST_URL is required; all others fall back to sensible defaults.
func configFromEnv() (Config, error) {
	url := os.Getenv("ICECAST_URL")
	if url == "" {
		return Config{}, errors.New("ICECAST_URL is required but not set")
	}

	schedulePath := os.Getenv("SCHEDULE_PATH")
	if schedulePath == "" {
		schedulePath = "config/schedule.yaml"
	}

	talkDir := os.Getenv("TALK_SEGMENTS_DIR")
	if talkDir == "" {
		talkDir = "output/talk_segments"
	}

	bumperDir := os.Getenv("BUMPER_DIR")
	if bumperDir == "" {
		bumperDir = "output/music_bumpers"
	}

	return Config{
		IcecastURL:      url,
		SchedulePath:    schedulePath,
		TalkSegmentsDir: talkDir,
		BumperDir:       bumperDir,
	}, nil
}
