package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

// ---- mockSilencePiper -------------------------------------------------------

type mockSilencePiper struct {
	writes   [][]byte
	alive    bool
	writeErr error
}

func (m *mockSilencePiper) Write(p []byte) (int, error) {
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	cp := make([]byte, len(p))
	copy(cp, p)
	m.writes = append(m.writes, cp)
	return len(p), nil
}

func (m *mockSilencePiper) Alive() bool { return m.alive }

// ---- pipeSilence tests ------------------------------------------------------

func TestPipeSilence_AllBytesZero(t *testing.T) {
	m := &mockSilencePiper{alive: true}
	pipeSilence(context.Background(), m, 150*time.Millisecond)
	if len(m.writes) == 0 {
		t.Fatal("expected at least one write")
	}
	for i, chunk := range m.writes {
		for j, b := range chunk {
			if b != 0 {
				t.Fatalf("write[%d][%d] = %d, want 0", i, j, b)
			}
		}
	}
}

func TestPipeSilence_CancelledContextWritesNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m := &mockSilencePiper{alive: true}
	pipeSilence(ctx, m, 10*time.Second)
	if len(m.writes) != 0 {
		t.Fatalf("expected 0 writes for cancelled context, got %d", len(m.writes))
	}
}

func TestPipeSilence_DeadEncoderWritesNothing(t *testing.T) {
	m := &mockSilencePiper{alive: false}
	pipeSilence(context.Background(), m, 10*time.Second)
	if len(m.writes) != 0 {
		t.Fatalf("expected 0 writes for dead encoder, got %d", len(m.writes))
	}
}

func TestPipeSilence_WriteErrorStops(t *testing.T) {
	m := &mockSilencePiper{alive: true, writeErr: errors.New("broken pipe")}
	pipeSilence(context.Background(), m, 10*time.Second)
	if len(m.writes) != 0 {
		t.Fatalf("expected 0 successful writes after error, got %d", len(m.writes))
	}
}

func TestPipeSilence_CancelCutsShort(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(25 * time.Millisecond)
		cancel()
	}()
	m := &mockSilencePiper{alive: true}
	start := time.Now()
	pipeSilence(ctx, m, 10*time.Second)
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("pipeSilence took %v after cancel, want < 500ms", elapsed)
	}
}

// scheduleFixture returns the path to the testdata schedule YAML.
// Go test runs from the package directory, so a relative path is sufficient.
func scheduleFixture() string {
	return "testdata/schedule.yaml"
}

func TestContextSleep_RunsToCompletion(t *testing.T) {
	ctx := context.Background()
	start := time.Now()
	contextSleep(ctx, 50*time.Millisecond)
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Errorf("contextSleep returned too early: %v", elapsed)
	}
}

func TestContextSleep_CancelCutsShort(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	contextSleep(ctx, 10*time.Second)
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Errorf("contextSleep did not return promptly after cancel: %v", elapsed)
	}
}

func TestContextSleep_AlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before calling

	start := time.Now()
	contextSleep(ctx, 10*time.Second)
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("contextSleep should return immediately for cancelled context: %v", elapsed)
	}
}

// TestRun_CancelledContextExitsImmediately verifies that run() returns
// immediately when the context is already cancelled before it is called.
// Uses an invalid Icecast URL so no real connection is attempted.
func TestRun_CancelledContextExitsImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already done

	cfg := Config{
		IcecastURL:      "icecast://source:hackme@localhost:19999/stream",
		SchedulePath:    scheduleFixture(),
		TalkSegmentsDir: t.TempDir(),
		BumperDir:       t.TempDir(),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		run(ctx, cfg)
	}()
	select {
	case <-done:
		// passed
	case <-time.After(2 * time.Second):
		t.Fatal("run() did not exit within 2 s for a pre-cancelled context")
	}
}

// TestRun_ExitsAfterContextTimeout verifies that run() exits cleanly once the
// context deadline passes, even while waiting through an encoder-restart delay.
func TestRun_ExitsAfterContextTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping encoder-fail restart test in -short mode")
	}

	// Cancel after slightly more than one encoderRestartDelay so the loop
	// has time to attempt one restart and then exit on cancellation.
	ctx, cancel := context.WithTimeout(context.Background(), encoderRestartDelay+500*time.Millisecond)
	defer cancel()

	cfg := Config{
		IcecastURL:      "icecast://source:hackme@localhost:19999/stream", // unreachable
		SchedulePath:    scheduleFixture(),
		TalkSegmentsDir: t.TempDir(),
		BumperDir:       t.TempDir(),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		run(ctx, cfg)
	}()
	select {
	case <-done:
		// passed — run exited after context cancellation
	case <-time.After(10 * time.Second):
		t.Fatal("run() did not exit within 10 s")
	}
}
