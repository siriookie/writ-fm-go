package streamer

import (
	"context"
	"testing"
	"time"
)

func TestStopperStopCancelsTasks(t *testing.T) {
	stopper := NewStopper(context.Background())

	stopped := make(chan struct{})
	if err := stopper.Go("waiter", func(ctx context.Context) error {
		<-ctx.Done()
		close(stopped)
		return nil
	}); err != nil {
		t.Fatalf("Go: %v", err)
	}

	stopper.Stop()
	select {
	case <-stopped:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("task did not observe cancellation")
	}
	stopper.Wait()
}

func TestStopperWaitWaitsForTaskExit(t *testing.T) {
	stopper := NewStopper(context.Background())

	release := make(chan struct{})
	if err := stopper.Go("blocker", func(ctx context.Context) error {
		<-ctx.Done()
		<-release
		return nil
	}); err != nil {
		t.Fatalf("Go: %v", err)
	}

	stopper.Stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		stopper.Wait()
	}()

	select {
	case <-done:
		t.Fatal("Wait returned before task exit")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Wait did not return after task exit")
	}
}

func TestStopperStopIsIdempotent(t *testing.T) {
	stopper := NewStopper(context.Background())
	stopper.Stop()
	stopper.Stop()
	stopper.Wait()
}

func TestStopperRejectsNewTasksAfterStop(t *testing.T) {
	stopper := NewStopper(context.Background())
	stopper.Stop()

	if err := stopper.Go("late", func(ctx context.Context) error { return nil }); err == nil {
		t.Fatal("expected Go to reject new task after Stop")
	}
}
