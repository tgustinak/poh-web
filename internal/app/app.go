// Package app wires all HTTP routes of the application.
// The returned handler is used by cmd/main.go and by integration tests,
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
	"github.com/tgustinak/pohkapela.sk/internal/concerts"
	"github.com/tgustinak/pohkapela.sk/internal/db"
	"github.com/tgustinak/pohkapela.sk/internal/server"
	"github.com/tgustinak/pohkapela.sk/web/templates"
	admintemplates "github.com/tgustinak/pohkapela.sk/web/templates/admin"
)

// Deps are the dependencies needed to build the application handler.
type Deps struct {
	Auth            *auth.Service
	Queries         *db.Queries
	AuthHandler     *admin.AuthHandler
	LogsHandler     *admin.LogsHandler
	ConcertsHandler *admin.ConcertsHandler
}

// NewMux builds the full application route tree wrapped
// in the request logging middleware.
func NewMux(deps Deps) http.Handler {
	mux := http.NewServeMux()

	mux.Handle(
		"/static/",
		http.StripPrefix(
			"/static/",
			http.FileServer(http.Dir(staticDir())),
		),
	)

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		today := concerts.Today()

		upcoming, err := deps.Queries.ListUpcomingConcerts(r.Context(), today)
		if err != nil {
			http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

			return
		}

		past, err := deps.Queries.ListPastConcerts(r.Context(), today)
		if err != nil {
			http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

			return
		}

		w.WriteHeader(http.StatusOK)

		renderTemplate(templates.Home(
			concerts.FromDBMany(upcoming),
			concerts.GroupPastByYear(past),
			len(past),
		), w)
	})

	mux.HandleFunc("GET /pre-organizatorov", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(templates.Organizers(), w)
	})

	mux.HandleFunc("GET /admin", deps.AuthHandler.LoginPage)
	mux.HandleFunc("POST /admin/login", deps.AuthHandler.Login)
	mux.HandleFunc("POST /admin/logout", protected(deps.Auth, deps.AuthHandler.Logout))

	mux.HandleFunc("GET /admin/dashboard", protected(deps.Auth, func(w http.ResponseWriter, r *http.Request) {
		upcoming, err := deps.Queries.CountUpcomingConcerts(r.Context(), concerts.Today())
		if err != nil {
			http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

			return
		}

		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminDashboard(csrf(r), upcoming), w)
	}))

	mux.HandleFunc("GET /admin/music", protected(deps.Auth, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminMusic(csrf(r)), w)
	}))

	mux.HandleFunc("GET /admin/concerts", protected(deps.Auth, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		deps.ConcertsHandler.ListPage(w, r)
	}))

	mux.HandleFunc("POST /admin/concerts", protected(deps.Auth, deps.ConcertsHandler.Create))
	mux.HandleFunc("GET /admin/concerts/{id}", protected(deps.Auth, deps.ConcertsHandler.Row))
	mux.HandleFunc("GET /admin/concerts/{id}/edit", protected(deps.Auth, deps.ConcertsHandler.EditForm))
	mux.HandleFunc("PUT /admin/concerts/{id}", protected(deps.Auth, deps.ConcertsHandler.Update))
	mux.HandleFunc("DELETE /admin/concerts/{id}", protected(deps.Auth, deps.ConcertsHandler.Delete))

	mux.HandleFunc("GET /admin/merch", protected(deps.Auth, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminMerch(csrf(r)), w)
	}))

	mux.HandleFunc("GET /admin/videos", protected(deps.Auth, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminVideos(csrf(r)), w)
	}))

	mux.HandleFunc("GET /admin/about", protected(deps.Auth, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminAbout(csrf(r)), w)
	}))

	mux.HandleFunc("GET /admin/social", protected(deps.Auth, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminSocial(csrf(r)), w)
	}))

	mux.HandleFunc("GET /admin/contact", protected(deps.Auth, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminContact(csrf(r)), w)
	}))

	mux.HandleFunc("GET /admin/organizers", protected(deps.Auth, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		renderTemplate(admintemplates.AdminOrganizers(csrf(r)), w)
	}))

	mux.HandleFunc("GET /admin/logs", protected(deps.Auth, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		deps.LogsHandler.LogsPage(w, r)
	}))

	mux.HandleFunc("GET /admin/logs/entries", protected(deps.Auth, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		deps.LogsHandler.LogsEntries(w, r)
	}))

	return server.Logger(mux)
}

// protected wraps a handler with authentication and CSRF protection.
func protected(authSvc *auth.Service, handler http.HandlerFunc) http.HandlerFunc {
	return authSvc.RequireAuth(auth.RequireCsrf(handler)).ServeHTTP
}

// csrf returns the session CSRF token for template rendering.
// Behind RequireAuth it is always present.
func csrf(r *http.Request) string {
	token, _ := auth.CsrfFromRequest(r)

	return token
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
