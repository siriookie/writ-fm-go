package streamer

import (
	"context"
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

func runPlaybackLoop(
	ctx context.Context,
	icecastURL string,
	sched *scheduler.StationSchedule,
	talks *store.TalkStore,
	bumpers *store.BumperStore,
	ctrl *control.Controller,
	state *nowplaying.State,
) {
	for {
		if ctx.Err() != nil {
			return
		}

		enc, err := audio.NewEncoder(icecastURL)
		if err != nil {
			log.Printf("streamer: start encoder: %v", err)
			contextSleep(ctx, encoderRestartDelay)
			continue
		}

		if !enc.WaitReady(encoderReadyTimeout) {
			log.Printf("streamer: encoder failed to connect to Icecast (URL=%s)", icecastURL)
			_ = enc.Close()
			contextSleep(ctx, encoderRestartDelay)
			continue
		}

		log.Printf("streamer: encoder connected to %s", icecastURL)
		runInner(ctx, enc, sched, talks, bumpers, ctrl, state)
		_ = enc.Close()

		if ctx.Err() != nil {
			return
		}
		log.Printf("streamer: encoder exited unexpectedly, restarting in %v", encoderRestartDelay)
		contextSleep(ctx, encoderRestartDelay)
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
	state *nowplaying.State,
) {
	var lastBumperPath string

	for {
		if ctx.Err() != nil || !enc.Alive() {
			return
		}

		show, err := sched.Resolve(timeNow())
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

		seg := segs[0]
		segBase := filepath.Base(seg.Path)

		if err := state.Update(nowplaying.Track{
				ShowID:      show.ShowID,
				ShowName:    show.Name,
				Type:        "talk",
				Name:        nowplaying.CleanName(segBase, true),
				Host:        show.Host,
				SegmentType: nowplaying.ExtractSegmentType(segBase),
				UpdatedAt:   timeNow(),
			}); err != nil {
				log.Printf("streamer: write now-playing: %v", err)
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
				break
			}

			if err := state.Update(nowplaying.Track{
				ShowID:      show.ShowID,
				ShowName:    show.Name,
				Type:        "bumper",
				Name:        bumperDisplayName(bumper),
				AIGenerated: bumper.Caption != "" || bumper.DisplayName != "",
				Caption:     bumper.Caption,
				UpdatedAt:   timeNow(),
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

var timeNow = func() time.Time {
	return time.Now()
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

// bumperDisplayName returns a listener-facing display name for a bumper.
// Prefers the metadata DisplayName; falls back to "AI Music".
func bumperDisplayName(b *store.BumperTrack) string {
	if b.DisplayName != "" {
		return b.DisplayName
	}
	return "AI Music"
}
