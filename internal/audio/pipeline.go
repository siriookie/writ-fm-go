// Package audio manages ffmpeg subprocesses for decoding and encoding audio.
package audio

import (
	"errors"
	"io"
)

// ErrSkipped is returned by Pipe when a skip signal is received before the
// source reaches EOF.
var ErrSkipped = errors.New("audio: track skipped")

// Pipe reads raw PCM from src in 8192-byte chunks and writes each chunk to dst.
// It returns when one of three conditions is met:
//
//   - src reaches io.EOF → returns nil (track finished naturally)
//   - skip receives a value → returns ErrSkipped (caller should advance to next track)
//   - dst.Write returns an error → returns that error (encoder died)
//
// src.Close is always called before Pipe returns, regardless of exit reason.
// This kills the underlying ffmpeg decoder subprocess.
//
// A nil skip channel is valid and means "never skip": the nil case in the
// select is never ready, so Pipe drains src to EOF normally.
//
// Chunk size (8192 bytes) is chosen to balance throughput and skip-signal
// latency. At 44100 Hz stereo s16le, each chunk represents ~46 ms of audio —
// fine-grained enough that a skip command takes effect within one chunk.
func Pipe(src io.ReadCloser, dst io.Writer, skip <-chan struct{}) error {
	defer src.Close()

	buf := make([]byte, 8192)
	for {
		// Non-blocking check for skip signal before each read.
		// If skip is nil, this select always takes the default branch.
		select {
		case <-skip:
			return ErrSkipped
		default:
		}

		n, readErr := src.Read(buf)
		if n > 0 {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}
