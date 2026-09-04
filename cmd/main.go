package main

import (
	"context"
	"net/http"

	"github.com/a-h/templ"
	"github.com/tgustinak/pohkapela.sk/internal/server"
	"github.com/tgustinak/pohkapela.sk/web/templates"
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

		renderTemplate(templates.Home(), w)
	})

	srv.Route("GET /pre-organizatorov", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(templates.Organizers(), w)
	})

	srv.Start()
}

func renderTemplate(t templ.Component, w http.ResponseWriter) {
	t.Render(context.Background(), w)
}
