package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	streamerapp "github.com/writ-fm/go/internal/streamer"
)

func main() {
	cfg, err := configFromEnv()
	if err != nil {
		log.Fatalf("streamer: config: %v", err)
	}

	// Trap SIGTERM and SIGINT. When either arrives, ctx is cancelled and run()
	// finishes the current audio chunk, closes the encoder, and exits cleanly.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	log.Printf("streamer: starting (Icecast=%s, schedule=%s)", cfg.IcecastURL, cfg.SchedulePath)
	streamerapp.Run(ctx, cfg)
	log.Printf("streamer: shutdown complete")
}
