package audio

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// ---- helpers for Pipe tests -------------------------------------------------------

// closeSpy wraps an io.Reader and tracks whether Close was called.
type closeSpy struct {
	io.Reader
	closed bool
}

func (cs *closeSpy) Close() error {
	cs.closed = true
	return nil
}

// failWriter returns a fixed error on every Write call.
type failWriter struct{ err error }

func (fw failWriter) Write([]byte) (int, error) { return 0, fw.err }

// failReadCloser returns a fixed error on every Read call.
type failReadCloser struct{ err error }

func (fr failReadCloser) Read([]byte) (int, error) { return 0, fr.err }
func (fr failReadCloser) Close() error              { return nil }

// ---- unit tests: Pipe -------------------------------------------------------------

func TestPipe_EOF_ReturnsNil(t *testing.T) {
	src := &closeSpy{Reader: strings.NewReader("PCM data")}
	var dst bytes.Buffer

	if err := Pipe(src, &dst, nil); err != nil {
		t.Fatalf("expected nil on EOF, got %v", err)
	}
	if dst.String() != "PCM data" {
		t.Errorf("data mismatch: got %q", dst.String())
	}
}

func TestPipe_LargeInput_AllChunksForwarded(t *testing.T) {
	// Build input larger than one chunk (> 8192 bytes) to exercise the loop.
	payload := bytes.Repeat([]byte("x"), 30_000)
	src := &closeSpy{Reader: bytes.NewReader(payload)}
	var dst bytes.Buffer

	if err := Pipe(src, &dst, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dst.Len() != len(payload) {
		t.Errorf("forwarded %d bytes, want %d", dst.Len(), len(payload))
	}
}

func TestPipe_AlwaysClosesSource(t *testing.T) {
	src := &closeSpy{Reader: strings.NewReader("data")}
	var dst bytes.Buffer

	_ = Pipe(src, &dst, nil)

	if !src.closed {
		t.Error("src.Close was not called")
	}
}

func TestPipe_AlwaysClosesSource_OnWriteError(t *testing.T) {
	src := &closeSpy{Reader: strings.NewReader("data")}
	dst := failWriter{err: errors.New("boom")}

	_ = Pipe(src, dst, nil)

	if !src.closed {
		t.Error("src.Close was not called even after write error")
	}
}

func TestPipe_WriteError_ReturnsError(t *testing.T) {
	sentinel := errors.New("encoder died")
	src := &closeSpy{Reader: bytes.NewReader(bytes.Repeat([]byte{0}, 8192))}
	dst := failWriter{err: sentinel}

	err := Pipe(src, dst, nil)
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel write error, got %v", err)
	}
}

func TestPipe_SkipSignal_ReturnsErrSkipped(t *testing.T) {
	// Pre-fill the skip channel so the signal is ready on the first iteration.
	skip := make(chan struct{}, 1)
	skip <- struct{}{}

	// Reader has data; Pipe should return ErrSkipped without consuming all of it.
	src := &closeSpy{Reader: bytes.NewReader(bytes.Repeat([]byte{0}, 1_000_000))}
	var dst bytes.Buffer

	err := Pipe(src, &dst, skip)
	if !errors.Is(err, ErrSkipped) {
		t.Fatalf("want ErrSkipped, got %v", err)
	}
}

func TestPipe_SkipSignal_ClosesSource(t *testing.T) {
	skip := make(chan struct{}, 1)
	skip <- struct{}{}

	src := &closeSpy{Reader: bytes.NewReader(bytes.Repeat([]byte{0}, 1_000_000))}
	var dst bytes.Buffer

	_ = Pipe(src, &dst, skip)

	if !src.closed {
		t.Error("src.Close not called after skip")
	}
}

func TestPipe_NilSkipChannel_NeverSkips(t *testing.T) {
	// nil channel is never ready; Pipe should drain to EOF normally.
	src := &closeSpy{Reader: strings.NewReader("audio bytes")}
	var dst bytes.Buffer

	err := Pipe(src, &dst, nil)
	if err != nil {
		t.Fatalf("nil skip channel caused unexpected error: %v", err)
	}
}

func TestPipe_EmptySource_ReturnsNil(t *testing.T) {
	src := &closeSpy{Reader: strings.NewReader("")}
	var dst bytes.Buffer

	if err := Pipe(src, &dst, nil); err != nil {
		t.Fatalf("empty source: want nil, got %v", err)
	}
}

func TestPipe_ReadError_ReturnsError(t *testing.T) {
	sentinel := errors.New("read failed")
	src := failReadCloser{err: sentinel}
	var dst bytes.Buffer

	err := Pipe(src, &dst, nil)
	if !errors.Is(err, sentinel) {
		t.Errorf("want read error %v, got %v", sentinel, err)
	}
}

// ---- integration tests: Pipe with real Decoder -----------------------------------

func TestPipe_Decoder_DrainsSilenceWAV(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg not in PATH")
	}
	dec, err := NewDecoder(makeSilenceWAV(t), DecodeOptions{IsSpeech: true})
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}
	var dst bytes.Buffer
	if err := Pipe(dec, &dst, nil); err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	if dst.Len() == 0 {
		t.Error("expected PCM bytes in dst, got 0")
	}
	t.Logf("piped %d PCM bytes from 0.5s WAV", dst.Len())
}

func TestPipe_Decoder_SkipMidTrack(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg not in PATH")
	}
	dec, err := NewDecoder(makeSilenceWAV(t), DecodeOptions{IsSpeech: true})
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}
	skip := make(chan struct{}, 1)
	skip <- struct{}{}

	err = Pipe(dec, io.Discard, skip)
	if !errors.Is(err, ErrSkipped) {
		t.Fatalf("want ErrSkipped, got %v", err)
	}
}
