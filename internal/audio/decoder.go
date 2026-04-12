// Package audio manages ffmpeg subprocesses for decoding and encoding audio.
package audio

import (
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

// DecodeOptions configures how an audio file is decoded to raw PCM.
type DecodeOptions struct {
	// IsSpeech selects -14 LUFS loudnorm (speech clarity) and disables
	// fade effects. False (music/bumper) uses -16 LUFS with 8s fade in/out.
	IsSpeech bool

	// StartTime offsets playback into the file, in seconds.
	// Zero means start from the beginning (no -ss flag emitted).
	StartTime float64

	// Duration limits how much of the file is decoded, in seconds.
	// Nil means decode to end of file (no -t flag emitted).
	Duration *float64
}

// Decoder wraps an ffmpeg subprocess that emits raw PCM on its stdout pipe.
//
// Output format is always: signed 16-bit little-endian (s16le), 44100 Hz,
// stereo (2 channels). This fixed format lets multiple sources be appended
// to the same encoder stdin without gaps or audio glitches.
//
// Decoder implements io.ReadCloser:
//   - Read returns PCM bytes; io.EOF signals the audio is finished.
//   - Close kills the ffmpeg process and releases all resources.
type Decoder struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser
}

// Read returns raw PCM bytes from the decoder. Satisfies io.Reader.
func (d *Decoder) Read(p []byte) (int, error) {
	return d.stdout.Read(p)
}

// Close stops the ffmpeg process and releases the stdout pipe.
// Safe to call after Read returns io.EOF, and safe to call without reading.
func (d *Decoder) Close() error {
	// Kill first — if the process already exited, Kill is a no-op on the OS
	// level (error is harmless). We must call Wait to reap the zombie.
	_ = d.cmd.Process.Kill()
	// Close the pipe so any blocked Read in another goroutine unblocks.
	_ = d.stdout.Close()
	// Reap the process; ignore the error (always non-nil after Kill).
	_ = d.cmd.Wait()
	return nil
}

// NewDecoder starts an ffmpeg decoder for the given audio file path.
// The caller must call Close() when done, regardless of whether Read
// reaches io.EOF.
func NewDecoder(path string, opts DecodeOptions) (*Decoder, error) {
	args := buildArgs(path, opts)
	cmd := exec.Command(args[0], args[1:]...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("audio: stdout pipe: %w", err)
	}
	cmd.Stderr = io.Discard // suppress ffmpeg progress/warning output

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("audio: start decoder: %w", err)
	}

	return &Decoder{cmd: cmd, stdout: stdout}, nil
}

// buildArgs constructs the ffmpeg argument slice for decoding path to PCM.
//
// Filter chain (in order):
//  1. loudnorm  — normalise loudness (-14 LUFS speech / -16 LUFS music)
//  2. afade in  — 8-second fade-in for music bumpers only
//  3. afade out — 8-second fade-out for music bumpers longer than 16 seconds
//  4. aresample — force output to 44100 Hz
func buildArgs(path string, opts DecodeOptions) []string {
	args := []string{"ffmpeg", "-v", "warning"}

	// -ss before -i = container-level (fast) seek; after -i = slow decode seek.
	if opts.StartTime > 0 {
		args = append(args, "-ss", formatSecs(opts.StartTime))
	}

	args = append(args, "-i", path)

	if opts.Duration != nil {
		args = append(args, "-t", formatSecs(*opts.Duration))
	}

	filters := make([]string, 0, 4)

	if opts.IsSpeech {
		// Speech: louder target (-14 LUFS), tight range (LRA=7) for clarity.
		filters = append(filters, "loudnorm=I=-14:TP=-1.5:LRA=7")
	} else {
		// Music: slightly quieter (-16 LUFS), wider range (LRA=11) for dynamics.
		filters = append(filters, "loudnorm=I=-16:TP=-1.5:LRA=11")
		// 8-second fade-in prevents a jarring cut-in at full volume.
		filters = append(filters, "afade=t=in:st=0:d=8")
		// Fade-out only when we know the duration and the track is long enough
		// that the fade doesn't eat into the audible content.
		if opts.Duration != nil && *opts.Duration > 16 {
			fadeStart := *opts.Duration - 8
			filters = append(filters, fmt.Sprintf("afade=t=out:st=%.3f:d=8", fadeStart))
		}
	}

	// aresample last: ensures every source lands at 44100 Hz regardless of
	// its original sample rate, so PCM chunks from different files are byte-
	// compatible when written to the same encoder stdin.
	filters = append(filters, "aresample=44100")

	args = append(args,
		"-vn",                            // discard video (some WAVs embed album art)
		"-af", strings.Join(filters, ","),
		"-f", "s16le",                    // raw PCM, signed 16-bit little-endian
		"-acodec", "pcm_s16le",
		"-ar", "44100",
		"-ac", "2",                       // stereo
		"-",                              // write to stdout
	)

	return args
}

// formatSecs formats a float64 number of seconds into the shortest decimal
// string accepted by ffmpeg (e.g. 10.5 → "10.5", 30.0 → "30").
func formatSecs(s float64) string {
	return strconv.FormatFloat(s, 'f', -1, 64)
}
