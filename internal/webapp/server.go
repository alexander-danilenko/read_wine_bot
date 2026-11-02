package webapp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Timeouts for the HTTP server. The API answers small JSON bodies, so these are generous.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 2 * time.Minute
	shutdownTimeout   = 10 * time.Second
)

// Server is the HTTP server of the Mini App.
type Server struct {
	srv *http.Server
	ln  net.Listener
}

// Listen opens addr, so a busy port stops the bot at startup, not on the first request.
func Listen(ctx context.Context, addr string, h http.Handler) (*Server, error) {
	var lc net.ListenConfig

	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}

	return &Server{
		srv: &http.Server{
			Handler:           h,
			ReadHeaderTimeout: readHeaderTimeout,
			ReadTimeout:       readTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
		},
		ln: ln,
	}, nil
}

// Addr is the address the server listens on.
func (s *Server) Addr() net.Addr {
	return s.ln.Addr()
}

// Run serves until ctx is done, then lets running requests finish within shutdownTimeout.
func (s *Server) Run(ctx context.Context) error {
	done := make(chan error, 1)

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()

		done <- s.srv.Shutdown(shutdownCtx)
	}()

	if err := s.srv.Serve(s.ln); !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve http: %w", err)
	}

	if err := <-done; err != nil {
		return fmt.Errorf("shut down http: %w", err)
	}

	return nil
}
