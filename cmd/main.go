package main

import (
	"net/http"

	"github.com/tgustinak/pohkapela.sk/internal/server"
)

const (
	APP_PORT                        = ":8080"
	FORCE_SHUTDOWN_AFTER_IN_SECONDS = 10
)

func main() {
	srv := server.NewServer(server.NewServerOptions{
		Port:                    APP_PORT,
		GracefulShutdownTimeout: FORCE_SHUTDOWN_AFTER_IN_SECONDS,
	})

	srv.Route("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello, world!"))
	})

	srv.Start()
}
