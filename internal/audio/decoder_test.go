package audio

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// ---- helpers ----------------------------------------------------------------

func ffmpegAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// makeSilenceWAV writes a minimal valid WAV file (0.5s mono silence) to a
// temp file and returns its path. No external tools required.
func makeSilenceWAV(t *testing.T) string {
	t.Helper()
	const (
		sampleRate = 44100
		channels   = 1
		bitsPerSam = 16
	)
	numSamples := sampleRate / 2 // 0.5 seconds
	dataSize := numSamples * channels * bitsPerSam / 8

	buf := &bytes.Buffer{}
	buf.WriteString("RIFF")
	binary.Write(buf, binary.LittleEndian, uint32(36+dataSize)) //nolint:errcheck
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	binary.Write(buf, binary.LittleEndian, uint32(16))                           //nolint:errcheck
	binary.Write(buf, binary.LittleEndian, uint16(1))                            // PCM
	binary.Write(buf, binary.LittleEndian, uint16(channels))                     //nolint:errcheck
	binary.Write(buf, binary.LittleEndian, uint32(sampleRate))                   //nolint:errcheck
	binary.Write(buf, binary.LittleEndian, uint32(sampleRate*channels*bitsPerSam/8)) //nolint:errcheck
	binary.Write(buf, binary.LittleEndian, uint16(channels*bitsPerSam/8))        //nolint:errcheck
	binary.Write(buf, binary.LittleEndian, uint16(bitsPerSam))                   //nolint:errcheck
	buf.WriteString("data")
	binary.Write(buf, binary.LittleEndian, uint32(dataSize)) //nolint:errcheck
	buf.Write(make([]byte, dataSize))                         // silence

	f, err := os.CreateTemp(t.TempDir(), "silence-*.wav")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

// filterChain extracts the value of the -af flag from an arg slice.
func filterChain(t *testing.T, args []string) string {
	t.Helper()
	for i, a := range args {
		if a == "-af" && i+1 < len(args) {
			return args[i+1]
		}
	}
	t.Fatal("no -af flag found in args")
	return ""
}

func hasArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// ---- unit tests: buildArgs --------------------------------------------------

func TestBuildArgs_OutputAlwaysStdout(t *testing.T) {
	args := buildArgs("test.wav", DecodeOptions{IsSpeech: true})
	if args[len(args)-1] != "-" {
		t.Errorf("last arg must be - (stdout), got %q", args[len(args)-1])
	}
}

func TestBuildArgs_OutputFormatFlags(t *testing.T) {
	args := buildArgs("test.wav", DecodeOptions{})
	for _, want := range []string{"-vn", "-f", "s16le", "-ar", "44100", "-ac", "2"} {
		if !hasArg(args, want) {
			t.Errorf("missing arg %q in %v", want, args)
		}
	}
}

func TestBuildArgs_AlwaysResamples(t *testing.T) {
	args := buildArgs("test.wav", DecodeOptions{})
	if !strings.Contains(filterChain(t, args), "aresample=44100") {
		t.Errorf("filter chain must contain aresample=44100, got: %q", filterChain(t, args))
	}
}

func TestBuildArgs_InputPath(t *testing.T) {
	args := buildArgs("/some/track.flac", DecodeOptions{})
	for i, a := range args {
		if a == "-i" && i+1 < len(args) {
			if args[i+1] != "/some/track.flac" {
				t.Errorf("wrong input path after -i: %q", args[i+1])
			}
			return
		}
	}
	t.Fatal("missing -i flag")
}

// --- speech ------------------------------------------------------------------

func TestBuildArgs_Speech_UsesHigherLUFS(t *testing.T) {
	chain := filterChain(t, buildArgs("x.wav", DecodeOptions{IsSpeech: true}))
	if !strings.Contains(chain, "loudnorm=I=-14") {
		t.Errorf("speech must use -14 LUFS, got: %q", chain)
	}
}

func TestBuildArgs_Speech_NoFade(t *testing.T) {
	chain := filterChain(t, buildArgs("x.wav", DecodeOptions{IsSpeech: true}))
	if strings.Contains(chain, "afade") {
		t.Errorf("speech must not have afade, got: %q", chain)
	}
}

// --- music bumper ------------------------------------------------------------

func TestBuildArgs_Music_UsesLowerLUFS(t *testing.T) {
	chain := filterChain(t, buildArgs("x.flac", DecodeOptions{IsSpeech: false}))
	if !strings.Contains(chain, "loudnorm=I=-16") {
		t.Errorf("music must use -16 LUFS, got: %q", chain)
	}
}

func TestBuildArgs_Music_AlwaysFadesIn(t *testing.T) {
	chain := filterChain(t, buildArgs("x.flac", DecodeOptions{IsSpeech: false}))
	if !strings.Contains(chain, "afade=t=in:st=0:d=8") {
		t.Errorf("music must have 8s fade-in, got: %q", chain)
	}
}

func TestBuildArgs_Music_FadesOut_WhenLong(t *testing.T) {
	dur := 60.0
	chain := filterChain(t, buildArgs("x.flac", DecodeOptions{Duration: &dur}))
	if !strings.Contains(chain, "afade=t=out") {
		t.Errorf("music >16s must have fade-out, got: %q", chain)
	}
	// fade start = 60 - 8 = 52 → formatted as 52.000
	if !strings.Contains(chain, "st=52") {
		t.Errorf("fade-out should start at 52s (duration-8), got: %q", chain)
	}
}

func TestBuildArgs_Music_NoFadeOut_WhenShort(t *testing.T) {
	dur := 10.0
	chain := filterChain(t, buildArgs("x.flac", DecodeOptions{Duration: &dur}))
	if strings.Contains(chain, "afade=t=out") {
		t.Errorf("music ≤16s must not have fade-out, got: %q", chain)
	}
}

func TestBuildArgs_Music_NoFadeOut_AtExactBoundary(t *testing.T) {
	dur := 16.0 // not strictly greater than 16
	chain := filterChain(t, buildArgs("x.flac", DecodeOptions{Duration: &dur}))
	if strings.Contains(chain, "afade=t=out") {
		t.Errorf("music at exactly 16s must not have fade-out, got: %q", chain)
	}
}

func TestBuildArgs_Music_NoFadeOut_NilDuration(t *testing.T) {
	chain := filterChain(t, buildArgs("x.flac", DecodeOptions{Duration: nil}))
	if strings.Contains(chain, "afade=t=out") {
		t.Errorf("unknown duration must not have fade-out, got: %q", chain)
	}
}

// --- seek / duration ---------------------------------------------------------

func TestBuildArgs_StartTime_AddsSS(t *testing.T) {
	args := buildArgs("x.wav", DecodeOptions{StartTime: 10.5})
	ssIdx := -1
	for i, a := range args {
		if a == "-ss" {
			ssIdx = i
			break
		}
	}
	if ssIdx < 0 {
		t.Fatal("missing -ss flag")
	}
	if args[ssIdx+1] != "10.5" {
		t.Errorf("-ss value: want 10.5, got %q", args[ssIdx+1])
	}
}

func TestBuildArgs_StartTime_BeforeInput(t *testing.T) {
	// -ss must come before -i for fast (container-level) seek.
	args := buildArgs("x.wav", DecodeOptions{StartTime: 5.0})
	ssIdx, iIdx := -1, -1
	for i, a := range args {
		switch a {
		case "-ss":
			ssIdx = i
		case "-i":
			iIdx = i
		}
	}
	if ssIdx < 0 || iIdx < 0 {
		t.Fatal("missing -ss or -i")
	}
	if ssIdx > iIdx {
		t.Errorf("-ss (idx %d) must appear before -i (idx %d)", ssIdx, iIdx)
	}
}

func TestBuildArgs_ZeroStartTime_OmitsSS(t *testing.T) {
	if hasArg(buildArgs("x.wav", DecodeOptions{StartTime: 0}), "-ss") {
		t.Error("zero start time must not produce -ss flag")
	}
}

func TestBuildArgs_Duration_AddsT(t *testing.T) {
	dur := 30.0
	args := buildArgs("x.wav", DecodeOptions{Duration: &dur})
	for i, a := range args {
		if a == "-t" && i+1 < len(args) {
			if args[i+1] != "30" {
				t.Errorf("-t value: want 30, got %q", args[i+1])
			}
			return
		}
	}
	t.Fatal("missing -t flag")
}

func TestBuildArgs_NilDuration_OmitsT(t *testing.T) {
	if hasArg(buildArgs("x.wav", DecodeOptions{Duration: nil}), "-t") {
		t.Error("nil duration must not produce -t flag")
	}
}

// ---- integration tests: NewDecoder / Read / Close ---------------------------

func TestNewDecoder_ReadsPCMBytes(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg not in PATH")
	}
	dec, err := NewDecoder(makeSilenceWAV(t), DecodeOptions{IsSpeech: true})
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}
	defer dec.Close()

	buf := make([]byte, 8192)
	n, err := dec.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read: %v", err)
	}
	if n == 0 {
		t.Error("expected PCM bytes, got 0")
	}
}

func TestNewDecoder_DrainToEOF(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg not in PATH")
	}
	dec, err := NewDecoder(makeSilenceWAV(t), DecodeOptions{IsSpeech: true})
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}
	defer dec.Close()

	total, err := io.Copy(io.Discard, dec)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if total == 0 {
		t.Error("expected non-zero PCM output for 0.5s silence")
	}
	t.Logf("drained %d PCM bytes from 0.5s WAV", total)
}

func TestNewDecoder_MusicFadeApplied(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg not in PATH")
	}
	dur := 20.0
	dec, err := NewDecoder(makeSilenceWAV(t), DecodeOptions{IsSpeech: false, Duration: &dur})
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}
	defer dec.Close()

	n, _ := io.Copy(io.Discard, dec)
	if n == 0 {
		t.Error("expected PCM output with music+fade options")
	}
}

func TestNewDecoder_CloseAfterDrain(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg not in PATH")
	}
	dec, err := NewDecoder(makeSilenceWAV(t), DecodeOptions{IsSpeech: true})
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}
	io.Copy(io.Discard, dec) // fully drain
	// Close on an already-finished process must not panic.
	dec.Close()
}

func TestNewDecoder_EarlyClose(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg not in PATH")
	}
	dec, err := NewDecoder(makeSilenceWAV(t), DecodeOptions{IsSpeech: true})
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}
	// Close without reading — must not hang or panic.
	dec.Close()
}
