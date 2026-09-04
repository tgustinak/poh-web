// Package app wires all HTTP routes of the application.
// The returned mux is used by cmd/main.go and by integration tests,
// so both exercise the exact same routing, middleware and handlers.
package app

import (
	"context"
	"net/http"
	"path/filepath"
	"runtime"

	"github.com/a-h/templ"
	"github.com/tgustinak/pohkapela.sk/internal/admin"
	"github.com/tgustinak/pohkapela.sk/internal/auth"
	"github.com/tgustinak/pohkapela.sk/web/templates"
	admintemplates "github.com/tgustinak/pohkapela.sk/web/templates/admin"
)

// Deps are the dependencies needed to build the application mux.
type Deps struct {
	Auth         *auth.Service
	AdminHandler *admin.Handler
}

// NewMux builds the full application route tree.
func NewMux(deps Deps) *http.ServeMux {
	mux := http.NewServeMux()

	mux.Handle(
		"/static/",
		http.StripPrefix(
			"/static/",
			http.FileServer(http.Dir(staticDir())),
		),
	)

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(templates.Home(), w)
	})

	mux.HandleFunc("GET /pre-organizatorov", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(templates.Organizers(), w)
	})

	mux.HandleFunc("GET /admin", deps.AdminHandler.LoginPage)
	mux.HandleFunc("POST /admin/login", deps.AdminHandler.Login)
	mux.HandleFunc("POST /admin/logout", deps.AdminHandler.Logout)

	protected := func(handler http.HandlerFunc) http.HandlerFunc {
		return deps.Auth.RequireAuth(handler).ServeHTTP
	}

	mux.HandleFunc("GET /admin/dashboard", protected(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminDashboard(), w)
	}))

	mux.HandleFunc("GET /admin/hudba", protected(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminMusic(), w)
	}))

	mux.HandleFunc("GET /admin/koncerty", protected(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminConcerts(), w)
	}))

	mux.HandleFunc("GET /admin/merch", protected(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminMerch(), w)
	}))

	mux.HandleFunc("GET /admin/klipy", protected(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminVideos(), w)
	}))

	mux.HandleFunc("GET /admin/o-nas", protected(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminAbout(), w)
	}))

	mux.HandleFunc("GET /admin/social", protected(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminSocial(), w)
	}))

	mux.HandleFunc("GET /admin/kontakt", protected(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminContact(), w)
	}))

	mux.HandleFunc("GET /admin/organizatori", protected(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminOrganizers(), w)
	}))

	return mux
}

// staticDir resolves web/static relative to this file,
// so the mux works regardless of the process working directory
// (e.g. when tests run from the package directory).
func staticDir() string {
	_, file, _, _ := runtime.Caller(0)

	return filepath.Join(filepath.Dir(file), "..", "..", "web", "static")
}

func renderTemplate(t templ.Component, w http.ResponseWriter) {
	t.Render(context.Background(), w)
}
