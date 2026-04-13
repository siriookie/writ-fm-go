package streamer

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
)

// Stopper manages application-level background tasks for the streamer.
//
// It owns a derived context that is cancelled during shutdown, rejects new task
// registration once stopping begins, and waits for all registered tasks to exit.
type Stopper struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	stopping bool
	wg       sync.WaitGroup
}

// NewStopper creates a Stopper derived from parent.
func NewStopper(parent context.Context) *Stopper {
	ctx, cancel := context.WithCancel(parent)
	return &Stopper{
		ctx:    ctx,
		cancel: cancel,
	}
}

// Context returns the Stopper-managed context shared by background tasks.
func (s *Stopper) Context() context.Context {
	return s.ctx
}

// Go registers and starts an application-level background task.
//
// Once Stop has been called, Go rejects new tasks. Task exit is logged with the
// task name to make lifecycle behavior observable during shutdown and failures.
func (s *Stopper) Go(name string, fn func(context.Context) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopping {
		return fmt.Errorf("stopper is stopping; cannot start task %q", name)
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		err := fn(s.ctx)
		switch {
		case err == nil:
			log.Printf("streamer: task %q exited", name)
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			log.Printf("streamer: task %q stopped: %v", name, err)
		default:
			log.Printf("streamer: task %q failed: %v", name, err)
		}
	}()
	return nil
}

// Stop begins graceful shutdown. It is safe to call more than once.
func (s *Stopper) Stop() {
	s.mu.Lock()
	alreadyStopping := s.stopping
	s.stopping = true
	s.mu.Unlock()

	if alreadyStopping {
		return
	}
	s.cancel()
}

// Wait blocks until all registered tasks have exited.
func (s *Stopper) Wait() {
	s.wg.Wait()
}
