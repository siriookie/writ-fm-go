package audio

import (
	"os"
	"testing"
	"time"
)

// TestE2E_DecoderPipeEncoder runs a full audio pipeline:
//
//	WAV file → ffmpeg decoder → Pipe → ffmpeg encoder → Icecast
//
// Skipped unless both ffmpeg is in PATH and ICECAST_URL is set.
//
// Example:
//
//	ICECAST_URL="icecast://source:hackme@localhost:8000/stream" \
//	  go test ./internal/audio/ -v -run TestE2E -timeout 30s
func TestE2E_DecoderPipeEncoder(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg not in PATH")
	}
	url := os.Getenv("ICECAST_URL")
	if url == "" {
		t.Skip("ICECAST_URL not set")
	}

	enc, err := NewEncoder(url)
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	defer enc.Close()

	if !enc.WaitReady(2 * time.Second) {
		t.Fatal("encoder did not connect to Icecast within 2s — is Icecast running?")
	}
	t.Log("encoder connected to Icecast")

	dec, err := NewDecoder(makeSilenceWAV(t), DecodeOptions{IsSpeech: true})
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}

	if err := Pipe(dec, enc, nil); err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	t.Log("pipe finished: 0.5s silence streamed to Icecast successfully")
}

// TestE2E_SkipMidStream verifies that a skip signal interrupts the pipeline
// cleanly while the encoder stays alive.
func TestE2E_SkipMidStream(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg not in PATH")
	}
	url := os.Getenv("ICECAST_URL")
	if url == "" {
		t.Skip("ICECAST_URL not set")
	}

	enc, err := NewEncoder(url)
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	defer enc.Close()

	if !enc.WaitReady(2 * time.Second) {
		t.Fatal("encoder did not connect to Icecast within 2s")
	}

	dec, err := NewDecoder(makeSilenceWAV(t), DecodeOptions{IsSpeech: true})
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}

	skip := make(chan struct{}, 1)
	skip <- struct{}{}

	if err := Pipe(dec, enc, skip); err != ErrSkipped {
		t.Fatalf("want ErrSkipped, got %v", err)
	}

	// Encoder must still be alive after a skip — no reconnect needed.
	if !enc.Alive() {
		t.Error("encoder died after skip; it should stay connected to Icecast")
	}
	t.Log("encoder survived skip, still connected to Icecast")
}
