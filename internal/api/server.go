// Package api implements the WRIT-FM REST API server.
//
// The server is designed to run as a goroutine alongside the streamer.
// All dependencies are injected via New(); the package defines its own
// consumer-side interfaces so callers are not forced to import concrete types.
package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/writ-fm/go/internal/domain"
	"github.com/writ-fm/go/internal/nowplaying"
)

const (
	apiShutdownTimeout  = 5 * time.Second
	listenerCacheTTL    = 15 * time.Second
)

// ---------------------------------------------------------------------------
// Consumer-side interfaces
// ---------------------------------------------------------------------------

// TrackState is the read-only view of the current on-air track.
// Implemented by *nowplaying.State.
type TrackState interface {
	Get() nowplaying.Track
}

// ScheduleResolver resolves the active show for a given point in time.
// Implemented by *scheduler.StationSchedule.
type ScheduleResolver interface {
	Resolve(t time.Time) (*domain.ResolvedShow, error)
}

// ListenerCounter fetches the live listener count from Icecast.
// Implemented by *icecast.Client.
type ListenerCounter interface {
	Listeners(mountpoint string) (int, error)
}

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

// Config holds the runtime configuration for the API server.
type Config struct {
	Addr         string // TCP address, e.g. ":8001"
	Mount        string // Icecast mountpoint, e.g. "/stream"
	MessagesFile string // Path to messages JSON, e.g. ~/.writ/messages.json
}

// ---------------------------------------------------------------------------
// Server
// ---------------------------------------------------------------------------

// Server is the WRIT-FM REST API server.
type Server struct {
	cfg      Config
	state    TrackState
	sched    ScheduleResolver
	lc       *cachedListenerCounter
	messages *messageStore
	router   http.Handler

	startedAt time.Time

	mu             sync.Mutex
	tracksPlayed   int
	lastTrackName  string
	totalListeners int64
}

// New creates a Server with all dependencies injected.
// listeners may be nil; listener count will always be 0 in that case.
func New(cfg Config, state TrackState, sched ScheduleResolver, listeners ListenerCounter) *Server {
	s := &Server{
		cfg:      cfg,
		state:    state,
		sched:    sched,
		lc:       newCachedListenerCounter(listeners, cfg.Mount, listenerCacheTTL),
		messages: newMessageStore(cfg.MessagesFile),
		startedAt: time.Now(),
	}
	s.router = s.buildRouter()
	return s
}

func (s *Server) buildRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(corsMiddleware)

	r.Get("/", s.handleNowPlaying)
	r.Get("/now-playing", s.handleNowPlaying)
	r.Get("/health", s.handleHealth)
	r.Get("/schedule", s.handleSchedule)
	r.Get("/stats", s.handleStats)
	r.Get("/messages", s.handleGetMessages)
	r.Post("/message", s.handlePostMessage)
	r.Options("/*", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	return r
}

// Run starts the HTTP server and blocks until ctx is cancelled.
// Returns nil on clean shutdown; non-nil on bind or serve errors.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:    s.cfg.Addr,
		Handler: s.router,
	}

	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return err
	}

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), apiShutdownTimeout)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	err = srv.Serve(ln)
	<-shutdownDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// ---------------------------------------------------------------------------
// CORS middleware
// ---------------------------------------------------------------------------

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		next.ServeHTTP(w, r)
	})
}

// ---------------------------------------------------------------------------
// Cached listener counter
// ---------------------------------------------------------------------------

// cachedListenerCounter wraps a ListenerCounter with a short TTL cache so that
// frequent API requests don't hammer the Icecast status endpoint.
// On error it returns the last known value (fail-open).
type cachedListenerCounter struct {
	counter   ListenerCounter
	mount     string
	ttl       time.Duration
	mu        sync.Mutex
	value     int
	fetchedAt time.Time
}

func newCachedListenerCounter(c ListenerCounter, mount string, ttl time.Duration) *cachedListenerCounter {
	return &cachedListenerCounter{counter: c, mount: mount, ttl: ttl}
}

func (c *cachedListenerCounter) get() int {
	if c.counter == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.fetchedAt) < c.ttl {
		return c.value
	}
	if n, err := c.counter.Listeners(c.mount); err == nil {
		c.value = n
	}
	c.fetchedAt = time.Now()
	return c.value
}
