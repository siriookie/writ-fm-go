package streamer

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestControlServerRun_ShutsDownOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	srv := &controlServer{
		addr:            "127.0.0.1:0",
		handler:         http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		shutdownTimeout: 200 * time.Millisecond,
	}

	done := make(chan error, 1)
	go func() {
		done <- srv.Run(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

func TestControlServerRun_ReturnsListenError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()

	srv := &controlServer{
		addr:            ln.Addr().String(),
		handler:         http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		shutdownTimeout: 200 * time.Millisecond,
	}

	err = srv.Run(context.Background())
	if err == nil {
		t.Fatal("expected listen error, got nil")
	}
}

type mockListenerCounter struct {
	listeners int
	err       error
	calls     chan struct{}
}

func (m *mockListenerCounter) Listeners(string) (int, error) {
	select {
	case m.calls <- struct{}{}:
	default:
	}
	if m.err != nil {
		return 0, m.err
	}
	return m.listeners, nil
}

func TestListenerPollerRun_ExitsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	poller := &listenerPoller{
		client:   &mockListenerCounter{listeners: 3, calls: make(chan struct{}, 1)},
		mount:    "/stream",
		interval: 10 * time.Millisecond,
	}

	done := make(chan error, 1)
	go func() {
		done <- poller.Run(ctx)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run did not return after cancellation")
	}
}

func TestListenerPollerRun_ContinuesAfterListenerError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	counter := &mockListenerCounter{
		err:   errors.New("boom"),
		calls: make(chan struct{}, 4),
	}
	poller := &listenerPoller{
		client:   counter,
		mount:    "/stream",
		interval: 10 * time.Millisecond,
	}

	done := make(chan error, 1)
	go func() {
		done <- poller.Run(ctx)
	}()

	timeout := time.After(200 * time.Millisecond)
	callCount := 0
	for callCount < 2 {
		select {
		case <-counter.calls:
			callCount++
		case <-timeout:
			t.Fatalf("expected at least 2 poll attempts, got %d", callCount)
		}
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run did not return after cancellation")
	}
}
