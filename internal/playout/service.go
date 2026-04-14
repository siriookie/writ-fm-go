package playout

import (
	"context"
	"io"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/writ-fm/go/internal/audio"
	"github.com/writ-fm/go/internal/control"
	"github.com/writ-fm/go/internal/nowplaying"
	"github.com/writ-fm/go/internal/scheduler"
	"github.com/writ-fm/go/internal/store"
)

const (
	encoderReadyTimeout  = 300 * time.Millisecond
	encoderRestartDelay  = 2 * time.Second
	emptyQueueDelay      = 30 * time.Second
	bumpersPerTalk       = 3
	silenceChunkInterval = 100 * time.Millisecond
	silenceChunkSize     = 44100 * 2 * 2 / 10
)

// TrackPublisher is the playout-side consumer contract for now-playing events.
type TrackPublisher interface {
	Publish(nowplaying.Track) error
}

// Service orchestrates the station playout loop. It owns the business workflow
// that resolves shows, consumes queued talk segments, inserts bumpers, and
// publishes listener-facing now-playing state.
type Service struct {
	icecastURL string
	sched      *scheduler.StationSchedule
	talks      *store.TalkStore
	bumpers    *store.BumperStore
	ctrl       *control.Controller
	publisher  TrackPublisher
	timeNow    func() time.Time
}

// New returns a playout service wired with the required collaborators.
func New(
	icecastURL string,
	sched *scheduler.StationSchedule,
	talks *store.TalkStore,
	bumpers *store.BumperStore,
	ctrl *control.Controller,
	publisher TrackPublisher,
) *Service {
	return &Service{
		icecastURL: icecastURL,
		sched:      sched,
		talks:      talks,
		bumpers:    bumpers,
		ctrl:       ctrl,
		publisher:  publisher,
		timeNow:    time.Now,
	}
}

// Run starts the persistent encoder lifecycle and blocks until ctx is cancelled.
func (s *Service) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		enc, err := audio.NewEncoder(s.icecastURL)
		if err != nil {
			log.Printf("streamer: start encoder: %v", err)
			contextSleep(ctx, encoderRestartDelay)
			continue
		}

		if !enc.WaitReady(encoderReadyTimeout) {
			log.Printf("streamer: encoder failed to connect to Icecast (URL=%s)", s.icecastURL)
			_ = enc.Close()
			contextSleep(ctx, encoderRestartDelay)
			continue
		}

		log.Printf("streamer: encoder connected to %s", s.icecastURL)
		s.runInner(ctx, enc)
		_ = enc.Close()

		if ctx.Err() != nil {
			return
		}
		log.Printf("streamer: encoder exited unexpectedly, restarting in %v", encoderRestartDelay)
		contextSleep(ctx, encoderRestartDelay)
	}
}

func (s *Service) runInner(ctx context.Context, enc *audio.Encoder) {
	var lastBumperPath string

	for {
		if ctx.Err() != nil || !enc.Alive() {
			return
		}

		show, err := s.sched.Resolve(s.timeNow())
		if err != nil {
			log.Printf("streamer: resolve schedule: %v", err)
			pipeSilence(ctx, enc, emptyQueueDelay)
			continue
		}

		segs, err := s.talks.List(show.ShowID)
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

		seg := segs[0]
		segBase := filepath.Base(seg.Path)
		if err := s.publisher.Publish(nowplaying.Track{
			ShowID:      show.ShowID,
			ShowName:    show.Name,
			Type:        "talk",
			Name:        nowplaying.CleanName(segBase, true),
			Host:        show.Host,
			SegmentType: nowplaying.ExtractSegmentType(segBase),
			UpdatedAt:   s.timeNow(),
		}); err != nil {
			log.Printf("streamer: write now-playing: %v", err)
		}

		log.Printf("streamer: playing talk %s", seg.Path)
		skipCh := s.ctrl.NextSegment()
		if err := pipeDecode(seg.Path, audio.DecodeOptions{IsSpeech: true}, enc, skipCh); err != nil {
			log.Printf("streamer: pipe talk %s: %v", seg.Path, err)
		}
		if err := os.Remove(seg.Path); err != nil && !os.IsNotExist(err) {
			log.Printf("streamer: remove talk segment %s: %v", seg.Path, err)
		}

		if ctx.Err() != nil || !enc.Alive() {
			return
		}

		n := rand.Intn(2) + bumpersPerTalk
		for i := 0; i < n; i++ {
			if ctx.Err() != nil || !enc.Alive() {
				return
			}
			bumper, err := s.bumpers.Pick(show.ShowID, lastBumperPath)
			if err != nil {
				log.Printf("streamer: pick bumper: %v", err)
				break
			}
			if bumper == nil {
				break
			}

			if err := s.publisher.Publish(nowplaying.Track{
				ShowID:      show.ShowID,
				ShowName:    show.Name,
				Type:        "bumper",
				Name:        bumperDisplayName(bumper),
				AIGenerated: bumper.Caption != "" || bumper.DisplayName != "",
				Caption:     bumper.Caption,
				UpdatedAt:   s.timeNow(),
			}); err != nil {
				log.Printf("streamer: write now-playing: %v", err)
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

func pipeDecode(path string, opts audio.DecodeOptions, enc *audio.Encoder, skip <-chan struct{}) error {
	dec, err := audio.NewDecoder(path, opts)
	if err != nil {
		return err
	}
	return audio.Pipe(dec, enc, skip)
}

func bumperDisplayName(b *store.BumperTrack) string {
	if b.DisplayName != "" {
		return b.DisplayName
	}
	return "AI Music"
}

type silencePiper interface {
	io.Writer
	Alive() bool
}

func pipeSilence(ctx context.Context, w silencePiper, d time.Duration) {
	chunk := make([]byte, silenceChunkSize)
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

func contextSleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}
