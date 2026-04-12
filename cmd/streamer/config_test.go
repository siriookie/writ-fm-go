package main

import (
	"testing"
)

func TestConfigFromEnv_IcecastURLRequired(t *testing.T) {
	t.Setenv("ICECAST_URL", "")
	_, err := configFromEnv()
	if err == nil {
		t.Fatal("expected error when ICECAST_URL is empty")
	}
}

func TestConfigFromEnv_Defaults(t *testing.T) {
	t.Setenv("ICECAST_URL", "icecast://source:hackme@localhost:8000/stream")
	t.Setenv("SCHEDULE_PATH", "")
	t.Setenv("TALK_SEGMENTS_DIR", "")
	t.Setenv("BUMPER_DIR", "")

	cfg, err := configFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.IcecastURL != "icecast://source:hackme@localhost:8000/stream" {
		t.Errorf("IcecastURL = %q", cfg.IcecastURL)
	}
	if cfg.SchedulePath != "config/schedule.yaml" {
		t.Errorf("SchedulePath = %q, want config/schedule.yaml", cfg.SchedulePath)
	}
	if cfg.TalkSegmentsDir != "output/talk_segments" {
		t.Errorf("TalkSegmentsDir = %q, want output/talk_segments", cfg.TalkSegmentsDir)
	}
	if cfg.BumperDir != "output/music_bumpers" {
		t.Errorf("BumperDir = %q, want output/music_bumpers", cfg.BumperDir)
	}
}

func TestConfigFromEnv_AllEnvVars(t *testing.T) {
	t.Setenv("ICECAST_URL", "icecast://source:pass@radio.example.com:8000/live")
	t.Setenv("SCHEDULE_PATH", "/etc/writ-fm/schedule.yaml")
	t.Setenv("TALK_SEGMENTS_DIR", "/var/writ-fm/talk")
	t.Setenv("BUMPER_DIR", "/var/writ-fm/bumpers")

	cfg, err := configFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.IcecastURL != "icecast://source:pass@radio.example.com:8000/live" {
		t.Errorf("IcecastURL = %q", cfg.IcecastURL)
	}
	if cfg.SchedulePath != "/etc/writ-fm/schedule.yaml" {
		t.Errorf("SchedulePath = %q", cfg.SchedulePath)
	}
	if cfg.TalkSegmentsDir != "/var/writ-fm/talk" {
		t.Errorf("TalkSegmentsDir = %q", cfg.TalkSegmentsDir)
	}
	if cfg.BumperDir != "/var/writ-fm/bumpers" {
		t.Errorf("BumperDir = %q", cfg.BumperDir)
	}
}
