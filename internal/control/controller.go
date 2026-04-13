// Package control provides the skip-command controller for the streamer.
package control

import (
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
)

// Controller manages a per-segment skip signal and acts as an HTTP handler.
//
// Typical usage:
//
//	ctrl := control.NewController()
//	go http.ListenAndServe(addr, ctrl)
//
//	// before each audio segment:
//	skipCh := ctrl.NextSegment()
//	audio.Pipe(decoder, encoder, skipCh)
type Controller struct {
	mu      sync.Mutex
	current chan struct{} // closed when Skip is called; nil between segments
	router  http.Handler
}

// NewController returns an idle Controller with no active segment.
func NewController() *Controller {
	c := &Controller{}
	c.router = c.newRouter()
	return c
}

// NextSegment returns a fresh skip channel for the upcoming audio segment.
// Any pending skip signal on the previous channel is discarded — the new
// channel is always open when returned.
func (c *Controller) NextSegment() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.current = make(chan struct{})
	return c.current
}

// Skip signals the currently-playing segment to stop early.
// Safe to call when no segment is active (no-op in that case).
// Safe to call more than once for the same segment.
func (c *Controller) Skip() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current != nil {
		close(c.current)
		c.current = nil
	}
}

// ServeHTTP implements http.Handler.
//
//	POST /skip  — signal the current segment to skip; responds 204 No Content.
//
// All other paths respond 404; non-POST methods on /skip respond 405.
func (c *Controller) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.router.ServeHTTP(w, r)
}

func (c *Controller) newRouter() http.Handler {
	r := chi.NewRouter()
	r.Post(
		"/skip", func(w http.ResponseWriter, r *http.Request) {
			c.Skip()
			w.WriteHeader(http.StatusNoContent)
		})
	return r
}
