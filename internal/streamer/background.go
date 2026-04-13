package streamer

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/writ-fm/go/internal/icecast"
)

const controlServerShutdownTimeout = 2 * time.Second

// Runner is the minimal lifecycle interface for streamer background services.
type Runner interface {
	Run(ctx context.Context) error
}

type controlServer struct {
	addr            string
	handler         http.Handler
	shutdownTimeout time.Duration
}

func newControlServer(addr string, handler http.Handler) Runner {
	return &controlServer{
		addr:            addr,
		handler:         handler,
		shutdownTimeout: controlServerShutdownTimeout,
	}
}

func (s *controlServer) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:    s.addr,
		Handler: s.handler,
	}

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	err = srv.Serve(ln)
	<-shutdownDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

type listenerPoller struct {
	client   listenerCounter
	mount    string
	interval time.Duration
}

type listenerCounter interface {
	Listeners(mountpoint string) (int, error)
}

func newListenerPoller(client *icecast.Client, mount string) Runner {
	return &listenerPoller{
		client:   client,
		mount:    mount,
		interval: 60 * time.Second,
	}
}

func (p *listenerPoller) Run(ctx context.Context) error {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			n, err := p.client.Listeners(p.mount)
			if err != nil {
				log.Printf("streamer: listeners: %v", err)
				continue
			}
			log.Printf("streamer: listeners: %d", n)
		case <-ctx.Done():
			return nil
		}
	}
}
