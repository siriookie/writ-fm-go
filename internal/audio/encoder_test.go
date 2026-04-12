package audio

import (
	"testing"
	"time"
)

// ---- unit tests: buildEncoderArgs ------------------------------------------------

func TestBuildEncoderArgs_Encoder_RealtimeFlag(t *testing.T) {
	if !hasArg(buildEncoderArgs("icecast://url"), "-re") {
		t.Error("missing -re flag (required to prevent Icecast buffer flood)")
	}
}

func TestBuildEncoderArgs_Encoder_InputIsStdin(t *testing.T) {
	args := buildEncoderArgs("icecast://url")
	for i, a := range args {
		if a == "-i" && i+1 < len(args) {
			if args[i+1] != "-" {
				t.Errorf("-i value: want - (stdin), got %q", args[i+1])
			}
			return
		}
	}
	t.Fatal("missing -i flag")
}

func TestBuildEncoderArgs_Encoder_InputFormatFlags(t *testing.T) {
	args := buildEncoderArgs("icecast://url")
	for _, want := range []string{"-f", "s16le", "-ar", "44100", "-ac", "2"} {
		if !hasArg(args, want) {
			t.Errorf("missing input format arg %q in %v", want, args)
		}
	}
}

func TestBuildEncoderArgs_Encoder_InputFlagsBeforeInput(t *testing.T) {
	// Input format (-f s16le) must come before -i so ffmpeg knows how to
	// interpret the stdin bytes; after -i it would be an output flag.
	args := buildEncoderArgs("icecast://url")
	iIdx := -1
	for i, a := range args {
		if a == "-i" {
			iIdx = i
			break
		}
	}
	if iIdx < 0 {
		t.Fatal("missing -i flag")
	}
	fIdx := -1
	for i, a := range args {
		if a == "-f" && i+1 < len(args) && args[i+1] == "s16le" {
			fIdx = i
			break
		}
	}
	if fIdx < 0 {
		t.Fatal("missing -f s16le input flag")
	}
	if fIdx > iIdx {
		t.Errorf("-f s16le (idx %d) must appear before -i (idx %d)", fIdx, iIdx)
	}
}

func TestBuildEncoderArgs_Encoder_OutputCodecLame(t *testing.T) {
	if !hasArg(buildEncoderArgs("icecast://url"), "libmp3lame") {
		t.Error("missing libmp3lame codec")
	}
}

func TestBuildEncoderArgs_Encoder_OutputBitrate(t *testing.T) {
	if !hasArg(buildEncoderArgs("icecast://url"), "96k") {
		t.Error("missing 96k bitrate")
	}
}

func TestBuildEncoderArgs_Encoder_OutputFormatMP3(t *testing.T) {
	args := buildEncoderArgs("icecast://url")
	// There are two -f flags: -f s16le (input) and -f mp3 (output).
	// Check that at least one -f mp3 pair exists anywhere in the args.
	for i, a := range args {
		if a == "-f" && i+1 < len(args) && args[i+1] == "mp3" {
			return
		}
	}
	t.Errorf("missing -f mp3 output format flag in %v", args)
}

func TestBuildEncoderArgs_Encoder_ContentType(t *testing.T) {
	if !hasArg(buildEncoderArgs("icecast://url"), "audio/mpeg") {
		t.Error("missing audio/mpeg content_type")
	}
}

func TestBuildEncoderArgs_Encoder_URLIsLastArg(t *testing.T) {
	url := "icecast://source:pass@localhost:8000/stream"
	args := buildEncoderArgs(url)
	if args[len(args)-1] != url {
		t.Errorf("last arg must be the Icecast URL, got %q", args[len(args)-1])
	}
}

// ---- integration tests: NewEncoder / Alive / WaitReady / Close ------------------

func TestNewEncoder_StartsProcess(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg not in PATH")
	}
	// Port 19999 is almost certainly not running Icecast; ffmpeg should
	// start successfully but exit quickly when it can't connect.
	enc, err := NewEncoder("icecast://source:pass@127.0.0.1:19999/test")
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	defer enc.Close()
}

func TestNewEncoder_AliveImmediately(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg not in PATH")
	}
	enc, err := NewEncoder("icecast://source:pass@127.0.0.1:19999/test")
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	defer enc.Close()

	// Right after start the process must be alive (hasn't had time to fail yet).
	if !enc.Alive() {
		t.Error("expected encoder to be alive immediately after start")
	}
}

func TestNewEncoder_WaitReady_DiesOnBadURL(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg not in PATH")
	}
	enc, err := NewEncoder("icecast://source:pass@127.0.0.1:19999/test")
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	defer enc.Close()

	// ffmpeg fails fast on a refused connection (typically < 100 ms).
	// Waiting 500 ms should be more than enough for it to have exited.
	if enc.WaitReady(500 * time.Millisecond) {
		t.Error("WaitReady should return false for an unreachable Icecast URL")
	}
}

func TestNewEncoder_CloseDoesNotHang(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg not in PATH")
	}
	enc, err := NewEncoder("icecast://source:pass@127.0.0.1:19999/test")
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}

	done := make(chan struct{})
	go func() {
		enc.Close()
		close(done)
	}()

	select {
	case <-done:
		// good
	case <-time.After(3 * time.Second):
		t.Error("Close() hung for more than 3 seconds")
	}
}

func TestNewEncoder_CloseAfterProcessDied(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg not in PATH")
	}
	enc, err := NewEncoder("icecast://source:pass@127.0.0.1:19999/test")
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}

	// Wait for ffmpeg to exit on its own (bad URL → connection refused).
	enc.WaitReady(500 * time.Millisecond)

	// Close on an already-dead process must not panic or hang.
	enc.Close()
}
