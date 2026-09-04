package server

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"
)

const (
	defaultReadTimeout  = 10 * time.Second
	defaultWriteTimeout = 30 * time.Second
	defaultIdleTimeout  = 60 * time.Second
)

type NewServerOptions struct {
	Port                    string
	GracefulShutdownTimeout time.Duration
	ReadTimeout             time.Duration
	WriteTimeout            time.Duration
	IdleTimeout             time.Duration
}

// withDefaults fills zero timeouts with safe defaults.
func (o NewServerOptions) withDefaults() NewServerOptions {
	if o.ReadTimeout <= 0 {
		o.ReadTimeout = defaultReadTimeout
	}

	if o.WriteTimeout <= 0 {
		o.WriteTimeout = defaultWriteTimeout
	}

	if o.IdleTimeout <= 0 {
		o.IdleTimeout = defaultIdleTimeout
	}

	return o
}

type Server struct {
	mux     *http.ServeMux
	options NewServerOptions
}

// Mount registers handler h for pattern,
// e.g. the full application mux at "/".
func (s *Server) Mount(pattern string, handler http.Handler) {
	s.mux.Handle(pattern, handler)
}

func (s *Server) Start() {
	quitCh := make(chan os.Signal, 1)
	signal.Notify(quitCh, os.Interrupt)

	srv := &http.Server{
		Addr:         s.options.Port,
		Handler:      s.mux,
		ReadTimeout:  s.options.ReadTimeout,
		WriteTimeout: s.options.WriteTimeout,
		IdleTimeout:  s.options.IdleTimeout,
	}

	slog.Info("server starting", "addr", "http://localhost"+s.options.Port)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-quitCh

	slog.Info("server shutting down", "drain_timeout_s", s.options.GracefulShutdownTimeout)

	ctx, cancel := context.WithTimeout(context.Background(), s.options.GracefulShutdownTimeout*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server shutdown failed", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}

func NewServer(opts NewServerOptions) *Server {
	return &Server{
		mux:     http.NewServeMux(),
		options: opts.withDefaults(),
	}
}
