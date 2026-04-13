package main

import (
	"errors"
	"os"

	streamerapp "github.com/writ-fm/go/internal/streamer"
)

// configFromEnv reads configuration from environment variables.
// ICECAST_URL is required; all others fall back to sensible defaults.
func configFromEnv() (streamerapp.Config, error) {
	url := os.Getenv("ICECAST_URL")
	if url == "" {
		return streamerapp.Config{}, errors.New("ICECAST_URL is required but not set")
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

	return streamerapp.Config{
		IcecastURL:      url,
		SchedulePath:    schedulePath,
		TalkSegmentsDir: talkDir,
		BumperDir:       bumperDir,
		ControlAddr:     os.Getenv("CONTROL_ADDR"),
		NowPlayingPath:  os.Getenv("NOW_PLAYING_PATH"),
		IcecastBaseURL:  os.Getenv("ICECAST_BASE_URL"),
		APIAddr:         os.Getenv("API_ADDR"),
		MessagesFile:    os.Getenv("MESSAGES_FILE"),
	}, nil
}
