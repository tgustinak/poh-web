package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

type NewServerOptions struct {
	Port                    string
	GracefulShutdownTimeout time.Duration
}

type Server struct {
	mux     *http.ServeMux
	options NewServerOptions
}

func (s *Server) Route(path string, handler http.HandlerFunc) {
	s.mux.HandleFunc(path, handler)
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
		Addr:    s.options.Port,
		Handler: s.mux,
	}

	fmt.Printf("\nStarting on http://localhost%s\n\n", s.options.Port)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	<-quitCh

	fmt.Println()
	fmt.Println("== SERVER ABOUT TO SHUT DOWN ==")
	fmt.Println("Request to shutdown registered.")
	fmt.Println("Waiting for all requests to finish before shutting down.")
	fmt.Println("No more requests being accepted.")
	fmt.Printf("Force shutdown will happen in %d second(s)\n", s.options.GracefulShutdownTimeout)

	ctx, cancel := context.WithTimeout(context.Background(), s.options.GracefulShutdownTimeout*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal(err)
	}

	fmt.Println("== SERVER STOPPED ==")
}

func NewServer(opts NewServerOptions) *Server {
	return &Server{
		mux:     http.NewServeMux(),
		options: opts,
	}
}
