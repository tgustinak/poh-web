package main

import (
	"context"
	"net/http"

	"github.com/a-h/templ"
	"github.com/tgustinak/pohkapela.sk/internal/server"
	"github.com/tgustinak/pohkapela.sk/web/templates"
	admintemplates "github.com/tgustinak/pohkapela.sk/web/templates/admin"
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

	srv.Route("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminLogin(), w)
	})

	srv.Route("GET /admin/dashboard", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminDashboard(), w)
	})

	srv.Route("GET /admin/hudba", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminMusic(), w)
	})

	srv.Route("GET /admin/koncerty", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminConcerts(), w)
	})

	srv.Route("GET /admin/merch", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminMerch(), w)
	})

	srv.Route("GET /admin/klipy", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminVideos(), w)
	})

	srv.Route("GET /admin/o-nas", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminAbout(), w)
	})

	srv.Route("GET /admin/social", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminSocial(), w)
	})

	srv.Route("GET /admin/kontakt", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminContact(), w)
	})

	srv.Route("GET /admin/organizatori", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminOrganizers(), w)
	})

	srv.Start()
}

func renderTemplate(t templ.Component, w http.ResponseWriter) {
	t.Render(context.Background(), w)
}
