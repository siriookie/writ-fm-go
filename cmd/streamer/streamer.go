package main

import (
	"context"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/writ-fm/go/internal/audio"
	"github.com/writ-fm/go/internal/control"
	"github.com/writ-fm/go/internal/icecast"
	"github.com/writ-fm/go/internal/nowplaying"
	"github.com/writ-fm/go/internal/scheduler"
	"github.com/writ-fm/go/internal/store"
)

const (
	encoderReadyTimeout  = 300 * time.Millisecond // ffmpeg exits within ~100 ms if Icecast unreachable
	encoderRestartDelay  = 2 * time.Second
	emptyQueueDelay      = 30 * time.Second
	bumpersPerTalk       = 3 // minimum bumpers between talk segments; rand adds 0 or 1 more
	silenceChunkInterval = 100 * time.Millisecond
	silenceChunkSize     = 44100 * 2 * 2 / 10 // 17640 bytes = 0.1 s of s16le stereo 44100 Hz
)

// silencePiper is the subset of *audio.Encoder used by pipeSilence.
// Extracted as an interface so tests can inject a mock without a real ffmpeg process.
type silencePiper interface {
	io.Writer
	Alive() bool
}

// pipeSilence writes zero-filled PCM chunks to w for duration d, keeping the
// Icecast source connection alive when the talk queue is empty. Returns early
// if ctx is cancelled or w.Alive() returns false.
func pipeSilence(ctx context.Context, w silencePiper, d time.Duration) {
	chunk := make([]byte, silenceChunkSize) // zero-filled by Go runtime
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil || !w.Alive() {
			return
		}
		if _, err := w.Write(chunk); err != nil {
			return
		}
		contextSleep(ctx, silenceChunkInterval)
	}
}

// contextSleep sleeps for d, returning early if ctx is cancelled.
func contextSleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}

// run is the outer encoder loop. It starts a persistent ffmpeg encoder that
// streams to Icecast, runs the inner playback loop, and restarts on failure.
// It returns only when ctx is cancelled (SIGTERM / SIGINT).
func run(ctx context.Context, cfg Config) {
	sched, err := scheduler.LoadSchedule(cfg.SchedulePath)
	if err != nil {
		log.Fatalf("streamer: load schedule: %v", err)
	}

	talks := store.NewTalkStore(cfg.TalkSegmentsDir)
	bumpers := store.NewBumperStore(cfg.BumperDir)

	ctrl := control.NewController()
	if cfg.ControlAddr != "" {
		srv := &http.Server{Addr: cfg.ControlAddr, Handler: ctrl}
		go func() {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("streamer: control server: %v", err)
			}
		}()
		defer func() {
			shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutCtx)
		}()
		log.Printf("streamer: control server listening on %s", cfg.ControlAddr)
	}

	if cfg.IcecastBaseURL != "" {
		ic := icecast.NewClient(cfg.IcecastBaseURL)
		mount := icecastMount(cfg.IcecastURL)
		go pollListeners(ctx, ic, mount)
	}

	for {
		if ctx.Err() != nil {
			return
		}

		enc, err := audio.NewEncoder(cfg.IcecastURL)
		if err != nil {
			log.Printf("streamer: start encoder: %v", err)
			contextSleep(ctx, encoderRestartDelay)
			continue
		}

		if !enc.WaitReady(encoderReadyTimeout) {
			log.Printf("streamer: encoder failed to connect to Icecast (URL=%s)", cfg.IcecastURL)
			_ = enc.Close()
			contextSleep(ctx, encoderRestartDelay)
			continue
		}

		log.Printf("streamer: encoder connected to %s", cfg.IcecastURL)
		runInner(ctx, enc, sched, talks, bumpers, ctrl, cfg.NowPlayingPath)
		_ = enc.Close()

		if ctx.Err() != nil {
			return
		}
		log.Printf("streamer: encoder exited unexpectedly, restarting in %v", encoderRestartDelay)
		contextSleep(ctx, encoderRestartDelay)
	}
}

// icecastMount extracts the mountpoint path from an Icecast source URL.
// E.g. "icecast://source:pass@localhost:8000/stream" → "/stream".
func icecastMount(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Path == "" {
		return "/stream"
	}
	return u.Path
}

// pollListeners periodically fetches the listener count from Icecast and logs it.
func pollListeners(ctx context.Context, ic *icecast.Client, mount string) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			n, err := ic.Listeners(mount)
			if err != nil {
				log.Printf("streamer: listeners: %v", err)
			} else {
				log.Printf("streamer: listeners: %d", n)
			}
		case <-ctx.Done():
			return
		}
	}
}

// runInner plays talk segments interleaved with bumpers until ctx is cancelled
// or the encoder process dies. Each talk segment is deleted after playback
// (consume-queue semantics). Bumpers are kept in the pool for reuse.
func runInner(
	ctx context.Context,
	enc *audio.Encoder,
	sched *scheduler.StationSchedule,
	talks *store.TalkStore,
	bumpers *store.BumperStore,
	ctrl *control.Controller,
	nowPlayingPath string,
) {
	var lastBumperPath string

	for {
		if ctx.Err() != nil || !enc.Alive() {
			return
		}

		// Re-resolve active show each iteration so show changes take effect
		// as soon as a new talk segment is started.
		show, err := sched.Resolve(time.Now())
		if err != nil {
			log.Printf("streamer: resolve schedule: %v", err)
			pipeSilence(ctx, enc, emptyQueueDelay)
			continue
		}

		segs, err := talks.List(show.ShowID)
		if err != nil {
			log.Printf("streamer: list talk segments for show %q: %v", show.ShowID, err)
			pipeSilence(ctx, enc, emptyQueueDelay)
			continue
		}
		if len(segs) == 0 {
			log.Printf("streamer: no talk segments for show %q, waiting %v", show.ShowID, emptyQueueDelay)
			pipeSilence(ctx, enc, emptyQueueDelay)
			continue
		}

		// Play only the first segment, then loop back to re-resolve the active
		// show. This ensures a schedule change takes effect after at most one
		// talk segment, rather than after the entire batch is drained.
		seg := segs[0]

		if nowPlayingPath != "" {
			if err := nowplaying.Write(nowPlayingPath, nowplaying.Track{
				ShowID:    show.ShowID,
				ShowName:  show.Name,
				Type:      "talk",
				File:      filepath.Base(seg.Path),
				UpdatedAt: time.Now(),
			}); err != nil {
				log.Printf("streamer: write now-playing: %v", err)
			}
		}

		log.Printf("streamer: playing talk %s", seg.Path)
		skipCh := ctrl.NextSegment()
		if err := pipeDecode(seg.Path, audio.DecodeOptions{IsSpeech: true}, enc, skipCh); err != nil {
			log.Printf("streamer: pipe talk %s: %v", seg.Path, err)
		}
		if err := os.Remove(seg.Path); err != nil && !os.IsNotExist(err) {
			log.Printf("streamer: remove talk segment %s: %v", seg.Path, err)
		}

		if ctx.Err() != nil || !enc.Alive() {
			return
		}

		// Play 3 or 4 bumpers after the talk segment.
		n := rand.Intn(2) + bumpersPerTalk
		for i := 0; i < n; i++ {
			if ctx.Err() != nil || !enc.Alive() {
				return
			}
			bumper, err := bumpers.Pick(show.ShowID, lastBumperPath)
			if err != nil {
				log.Printf("streamer: pick bumper: %v", err)
				break
			}
			if bumper == nil {
				break // pool empty for this show
			}

			if nowPlayingPath != "" {
				if err := nowplaying.Write(nowPlayingPath, nowplaying.Track{
					ShowID:    show.ShowID,
					ShowName:  show.Name,
					Type:      "bumper",
					File:      filepath.Base(bumper.Path),
					UpdatedAt: time.Now(),
				}); err != nil {
					log.Printf("streamer: write now-playing: %v", err)
				}
			}

			log.Printf("streamer: playing bumper %s (%.0fs)", bumper.Path, bumper.Duration)
			dur := bumper.Duration
			opts := audio.DecodeOptions{IsSpeech: false, Duration: &dur}
			if err := pipeDecode(bumper.Path, opts, enc, nil); err != nil {
				log.Printf("streamer: pipe bumper %s: %v", bumper.Path, err)
			}
			lastBumperPath = bumper.Path
		}
	}
}

// pipeDecode starts a decoder for path and pipes it to enc.
// skip may be nil (pass-through from Pipe: nil means never skip).
func pipeDecode(path string, opts audio.DecodeOptions, enc *audio.Encoder, skip <-chan struct{}) error {
	dec, err := audio.NewDecoder(path, opts)
	if err != nil {
		return err
	}
	return audio.Pipe(dec, enc, skip)
}
