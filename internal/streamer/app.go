package streamer

import (
	"context"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/writ-fm/go/internal/api"
	"github.com/writ-fm/go/internal/control"
	"github.com/writ-fm/go/internal/icecast"
	"github.com/writ-fm/go/internal/nowplaying"
	"github.com/writ-fm/go/internal/playout"
	"github.com/writ-fm/go/internal/scheduler"
	"github.com/writ-fm/go/internal/stats"
	"github.com/writ-fm/go/internal/store"
)

const (
	encoderRestartDelay  = 2 * time.Second // kept in streamer for lifecycle tests and shutdown timing
	silenceChunkInterval = 100 * time.Millisecond
	silenceChunkSize     = 44100 * 2 * 2 / 10 // 17640 bytes = 0.1 s of s16le stereo 44100 Hz
)

// Config holds the runtime configuration for the streamer process.
type Config struct {
	IcecastURL      string // e.g. icecast://source:hackme@localhost:8000/stream
	SchedulePath    string // path to schedule YAML, e.g. config/schedule.yaml
	TalkSegmentsDir string // path to output/talk_segments
	BumperDir       string // path to output/music_bumpers
	ControlAddr     string // TCP address for the HTTP control server, e.g. 127.0.0.1:6600; empty = disabled
	NowPlayingPath  string // path to write now-playing.json; empty = disabled
	IcecastBaseURL  string // e.g. http://localhost:8000 for listener polling; empty = disabled
	APIAddr         string // TCP address for the REST API server, e.g. :8001; empty = disabled
	MessagesFile    string // path to listener messages JSON; defaults to ~/.writ/messages.json
}

// silencePiper is the subset of *audio.Encoder used by pipeSilence.
// Extracted as an interface so tests can inject a mock without a real ffmpeg process.
type silencePiper interface {
	io.Writer
	Alive() bool
}

// Run starts the streamer application and blocks until ctx is cancelled.
func Run(ctx context.Context, cfg Config) {
	sched, err := scheduler.LoadSchedule(cfg.SchedulePath)
	if err != nil {
		log.Fatalf("streamer: load schedule: %v", err)
	}

	talks := store.NewTalkStore(cfg.TalkSegmentsDir)
	bumpers := store.NewBumperStore(cfg.BumperDir)
	stopper := NewStopper(ctx)
	defer func() {
		stopper.Stop()
		stopper.Wait()
	}()

	tracker := stats.NewTracker()

	var sinks []nowplaying.Sink
	if cfg.NowPlayingPath != "" {
		sinks = append(sinks, nowplaying.NewJSONSink(cfg.NowPlayingPath))
	}
	sinks = append(sinks, tracker)

	// Shared now-playing state: written by the playout service, read by the API server.
	state := nowplaying.NewStateWithSinks(sinks...)

	ctrl := control.NewController()
	if cfg.ControlAddr != "" {
		srv := newControlServer(cfg.ControlAddr, ctrl)
		if err := stopper.Go("control-server", func(taskCtx context.Context) error {
			err := srv.Run(taskCtx)
			if err != nil {
				stopper.Stop()
			}
			return err
		}); err != nil {
			log.Fatalf("streamer: start control server: %v", err)
		}
		log.Printf("streamer: control server listening on %s", cfg.ControlAddr)
	}

	var ic *icecast.Client
	if cfg.IcecastBaseURL != "" {
		ic = icecast.NewClient(cfg.IcecastBaseURL)
		mount := icecastMount(cfg.IcecastURL)
		poller := newListenerPoller(ic, mount)
		if err := stopper.Go("listener-poller", poller.Run); err != nil {
			log.Fatalf("streamer: start listener poller: %v", err)
		}
	}

	if cfg.APIAddr != "" {
		messagesFile := cfg.MessagesFile
		if messagesFile == "" {
			messagesFile = defaultMessagesFile()
		}
		apiCfg := api.Config{
			Addr:         cfg.APIAddr,
			Mount:        icecastMount(cfg.IcecastURL),
			MessagesFile: messagesFile,
		}
		apiSrv := api.New(apiCfg, state, sched, ic, tracker)
		if err := stopper.Go("api-server", apiSrv.Run); err != nil {
			log.Fatalf("streamer: start api server: %v", err)
		}
		log.Printf("streamer: API server listening on %s", cfg.APIAddr)
	}

	playout.New(cfg.IcecastURL, sched, talks, bumpers, ctrl, state).Run(stopper.Context())
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

// icecastMount extracts the mountpoint path from an Icecast source URL.
// E.g. "icecast://source:pass@localhost:8000/stream" -> "/stream".
func icecastMount(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Path == "" {
		return "/stream"
	}
	return u.Path
}

// defaultMessagesFile returns the default path for listener messages.
func defaultMessagesFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".writ/messages.json"
	}
	return filepath.Join(home, ".writ", "messages.json")
}
