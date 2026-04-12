// Package audio manages ffmpeg subprocesses for decoding and encoding audio.
package audio

import (
	"fmt"
	"io"
	"os/exec"
	"time"
)

const (
	icecastIceName        = "WRIT-FM"
	icecastIceDescription = "The frequency between frequencies"
	icecastIceGenre       = "Talk Radio"
)

// Encoder wraps a persistent ffmpeg subprocess that reads raw PCM from its
// stdin, encodes it as MP3, and streams to an Icecast server.
//
// The encoder process must never be restarted mid-stream — Icecast drops all
// listener connections the moment the source disconnects. The outer loop in
// the streamer is responsible for restarting and reconnecting when the encoder
// dies.
//
// Input format expected on stdin: signed 16-bit little-endian, 44100 Hz, stereo.
// Output format streamed to Icecast: MP3, 96 kbps.
type Encoder struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	done  chan struct{} // closed when cmd.Wait() returns
}

// Write sends raw PCM bytes to the encoder stdin. Satisfies io.Writer.
func (e *Encoder) Write(p []byte) (int, error) {
	return e.stdin.Write(p)
}

// Alive reports whether the encoder process is still running.
// Safe to call concurrently.
func (e *Encoder) Alive() bool {
	select {
	case <-e.done:
		return false
	default:
		return true
	}
}

// WaitReady waits up to d for the encoder to establish its Icecast connection,
// then returns true if the process is still alive.
//
// ffmpeg exits within ~100 ms if it cannot reach the Icecast server, so
// waiting 300 ms is enough to distinguish "connected" from "failed to connect".
func (e *Encoder) WaitReady(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-e.done:
		return false // died before d elapsed
	case <-t.C:
		return e.Alive()
	}
}

// Close flushes the encoder stdin and waits for the process to exit.
// Safe to call after the process has already exited.
func (e *Encoder) Close() error {
	_ = e.stdin.Close() // signal EOF to ffmpeg; ignore broken-pipe on dead process
	<-e.done            // wait for cmd.Wait() goroutine to finish
	return nil
}

// NewEncoder starts a persistent ffmpeg encoder process that reads PCM from
// its stdin and streams MP3 to the given Icecast URL.
// The caller must call Close() when done.
func NewEncoder(url string) (*Encoder, error) {
	args := buildEncoderArgs(url)
	cmd := exec.Command(args[0], args[1:]...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("audio: encoder stdin pipe: %w", err)
	}
	cmd.Stderr = io.Discard // suppress ffmpeg progress/warning output

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("audio: start encoder: %w", err)
	}

	enc := &Encoder{cmd: cmd, stdin: stdin, done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(enc.done)
	}()

	return enc, nil
}

// buildEncoderArgs constructs the ffmpeg argument slice for streaming PCM to
// an Icecast server.
//
// Input (from stdin):   s16le PCM, 44100 Hz, stereo
// Output (to Icecast):  MP3, 96 kbps
//
// -re (realtime): consume stdin at 1× playback speed. Without it, ffmpeg drains
// stdin as fast as possible, flooding the Icecast buffer and causing listeners
// to receive audio in a burst rather than at the correct rate.
func buildEncoderArgs(url string) []string {
	return []string{
		"ffmpeg", "-v", "warning",
		"-re",                 // consume stdin at real-time rate
		"-f", "s16le",         // input format: signed 16-bit little-endian PCM
		"-ar", "44100",        // input sample rate: 44100 Hz
		"-ac", "2",            // input channels: stereo
		"-i", "-",             // read from stdin
		"-acodec", "libmp3lame",         // encode output to MP3 using LAME
		"-b:a", "96k",                   // output bitrate
		"-content_type", "audio/mpeg",   // Icecast stream MIME type
		"-ice_name", icecastIceName,
		"-ice_description", icecastIceDescription,
		"-ice_genre", icecastIceGenre,
		"-f", "mp3", // output container format
		url,         // Icecast destination URL
	}
}
