// Package app wires all HTTP routes of the application.
// The returned handler is used by cmd/main.go and by integration tests,
// so both exercise the exact same routing, middleware and handlers.
package app

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/a-h/templ"
	"github.com/tgustinak/pohkapela.sk/internal/admin"
	"github.com/tgustinak/pohkapela.sk/internal/auth"
	"github.com/tgustinak/pohkapela.sk/internal/concerts"
	"github.com/tgustinak/pohkapela.sk/internal/db"
	"github.com/tgustinak/pohkapela.sk/internal/organizers"
	"github.com/tgustinak/pohkapela.sk/internal/scripts"
	"github.com/tgustinak/pohkapela.sk/internal/server"
	"github.com/tgustinak/pohkapela.sk/web/templates"
	admintemplates "github.com/tgustinak/pohkapela.sk/web/templates/admin"
)

// Deps are the dependencies needed to build the application handler.
type Deps struct {
	Auth              *auth.Service
	Queries           *db.Queries
	UploadDir         string
	AuthHandler       *admin.AuthHandler
	LogsHandler       *admin.LogsHandler
	ConcertsHandler   *admin.ConcertsHandler
	VideosHandler     *admin.VideosHandler
	MusicHandler      *admin.MusicHandler
	AboutHandler      *admin.AboutHandler
	SocialHandler     *admin.SocialHandler
	OrganizersHandler *admin.OrganizersHandler
	ContactHandler    *admin.ContactHandler
	VPSHandler        *admin.VPSHandler
}

// NewMux builds the full application route tree wrapped
// in the request logging middleware.
func NewMux(deps Deps) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := healthCheck(r, deps); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "unhealthy"})

			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write([]byte("User-agent: *\nAllow: /\nSitemap: https://pohkapela.sk/sitemap.xml\n"))
	})

	mux.HandleFunc("GET /sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>` +
			`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` +
			`<url><loc>https://pohkapela.sk/</loc><changefreq>weekly</changefreq><priority>1.0</priority></url>` +
			`<url><loc>https://pohkapela.sk/pre-organizatorov</loc><changefreq>monthly</changefreq><priority>0.8</priority></url>` +
			`</urlset>`))
	})

	mux.Handle(
		"/static/",
		http.StripPrefix(
			"/static/",
			cacheControlFileServer(staticDir(), "public, max-age=31536000, immutable"),
		),
	)

	// Uploaded files (contact photo, ...) carry a content hash in their
	// file names, so every new upload is a fresh immutable URL.
	mux.Handle(
		"/uploads/",
		http.StripPrefix("/uploads/", immutableFileServer(deps.UploadDir)),
	)

	// Browsers request /favicon.ico by default even when the page
	// declares <link rel="icon">, so serve it explicitly instead
	// of returning 404 (visible as a console error).
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeFile(w, r, filepath.Join(staticDir(), "favicon.ico"))
	})

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

		clipRows, err := deps.Queries.ListVideosOrdered(r.Context())
		if err != nil {
			http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

			return
		}

		albumRows, err := deps.Queries.ListAlbumsOrdered(r.Context())
		if err != nil {
			http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

			return
		}

		aboutRow, err := deps.Queries.GetAbout(r.Context())
		if err != nil {
			http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

			return
		}

		socialRows, err := deps.Queries.ListSocialLinksOrdered(r.Context())
		if err != nil {
			http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

			return
		}

		contactData, err := deps.ContactHandler.GetContact(r.Context())
		if err != nil {
			http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

			return
		}

		w.WriteHeader(http.StatusOK)

		renderTemplate(templates.Home(
			templates.NewHomeData(templates.HomeParams{
				Concerts: templates.NewConcertsData(templates.ConcertsParams{
					Upcoming: upcoming,
					Past:     past,
				}),
				Videos:  clipRows,
				About:   aboutRow,
				Social:  socialRows,
				Albums:  albumRows,
				Contact: contactData,
			}),
		), w, r)
	})

	mux.HandleFunc("GET /pre-organizatorov", func(w http.ResponseWriter, r *http.Request) {
		contactData, err := deps.ContactHandler.GetContact(r.Context())
		if err != nil {
			http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

			return
		}

		docs, err := deps.Queries.ListOrganizerDocumentsOrdered(r.Context())
		if err != nil {
			http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

			return
		}

		photoRows, err := deps.Queries.ListOrganizerPhotosOrdered(r.Context())
		if err != nil {
			http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

			return
		}

		w.WriteHeader(http.StatusOK)

		renderTemplate(templates.Organizers(
			contactData,
			organizers.DocumentsFromDBMany(docs),
			organizers.PhotosFromDBMany(photoRows),
		), w, r)
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

		renderTemplate(admintemplates.AdminDashboard(csrf(r), upcoming), w, r)
	}))

	mux.HandleFunc("GET /admin/music", protected(deps.Auth, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		deps.MusicHandler.ListPage(w, r)
	}))

	mux.HandleFunc("POST /admin/music", protected(deps.Auth, deps.MusicHandler.Create))
	mux.HandleFunc("POST /admin/music/reorder", protected(deps.Auth, deps.MusicHandler.Reorder))
	mux.HandleFunc("GET /admin/music/{id}", protected(deps.Auth, deps.MusicHandler.Row))
	mux.HandleFunc("GET /admin/music/{id}/edit", protected(deps.Auth, deps.MusicHandler.EditForm))
	mux.HandleFunc("PUT /admin/music/{id}", protected(deps.Auth, deps.MusicHandler.Update))
	mux.HandleFunc("DELETE /admin/music/{id}", protected(deps.Auth, deps.MusicHandler.Delete))

	mux.HandleFunc("GET /admin/concerts", protected(deps.Auth, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		deps.ConcertsHandler.ListPage(w, r)
	}))

	mux.HandleFunc("POST /admin/concerts", protected(deps.Auth, deps.ConcertsHandler.Create))
	mux.HandleFunc("GET /admin/concerts/{id}", protected(deps.Auth, deps.ConcertsHandler.Row))
	mux.HandleFunc("GET /admin/concerts/{id}/edit", protected(deps.Auth, deps.ConcertsHandler.EditForm))
	mux.HandleFunc("PUT /admin/concerts/{id}", protected(deps.Auth, deps.ConcertsHandler.Update))
	mux.HandleFunc("DELETE /admin/concerts/{id}", protected(deps.Auth, deps.ConcertsHandler.Delete))

	mux.HandleFunc("GET /admin/videos", protected(deps.Auth, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		deps.VideosHandler.ListPage(w, r)
	}))

	mux.HandleFunc("POST /admin/videos", protected(deps.Auth, deps.VideosHandler.Create))
	mux.HandleFunc("POST /admin/videos/reorder", protected(deps.Auth, deps.VideosHandler.Reorder))
	mux.HandleFunc("GET /admin/videos/{id}", protected(deps.Auth, deps.VideosHandler.Row))
	mux.HandleFunc("GET /admin/videos/{id}/edit", protected(deps.Auth, deps.VideosHandler.EditForm))
	mux.HandleFunc("PUT /admin/videos/{id}", protected(deps.Auth, deps.VideosHandler.Update))
	mux.HandleFunc("DELETE /admin/videos/{id}", protected(deps.Auth, deps.VideosHandler.Delete))

	mux.HandleFunc("GET /admin/about", protected(deps.Auth, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		deps.AboutHandler.EditPage(w, r)
	}))

	mux.HandleFunc("PUT /admin/about", protected(deps.Auth, deps.AboutHandler.Update))

	mux.HandleFunc("GET /admin/social", protected(deps.Auth, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		deps.SocialHandler.ListPage(w, r)
	}))

	mux.HandleFunc("GET /admin/social/{platform}", protected(deps.Auth, deps.SocialHandler.Row))
	mux.HandleFunc("GET /admin/social/{platform}/edit", protected(deps.Auth, deps.SocialHandler.EditForm))
	mux.HandleFunc("PUT /admin/social/{platform}", protected(deps.Auth, deps.SocialHandler.Update))

	mux.HandleFunc("GET /admin/contact", protected(deps.Auth, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		deps.ContactHandler.EditPage(w, r)
	}))

	mux.HandleFunc("PUT /admin/contact", protected(deps.Auth, deps.ContactHandler.Update))
	mux.HandleFunc("PUT /admin/contact/photo", protected(deps.Auth, deps.ContactHandler.UploadPhoto))
	mux.HandleFunc("DELETE /admin/contact/photo", protected(deps.Auth, deps.ContactHandler.DeletePhoto))

	mux.HandleFunc("GET /admin/organizers", protected(deps.Auth, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		deps.OrganizersHandler.ListPage(w, r)
	}))

	mux.HandleFunc("PUT /admin/organizers/documents/{slot}", protected(deps.Auth, deps.OrganizersHandler.ReplaceDocument))
	mux.HandleFunc("POST /admin/organizers/photos", protected(deps.Auth, deps.OrganizersHandler.CreatePhoto))
	mux.HandleFunc("GET /admin/organizers/photos/{id}", protected(deps.Auth, deps.OrganizersHandler.PhotoRow))
	mux.HandleFunc("GET /admin/organizers/photos/{id}/edit", protected(deps.Auth, deps.OrganizersHandler.PhotoEditForm))
	mux.HandleFunc("PUT /admin/organizers/photos/{id}", protected(deps.Auth, deps.OrganizersHandler.UpdatePhotoAlt))
	mux.HandleFunc("DELETE /admin/organizers/photos/{id}", protected(deps.Auth, deps.OrganizersHandler.DeletePhoto))

	mux.HandleFunc("GET /admin/logs", protected(deps.Auth, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		deps.LogsHandler.LogsPage(w, r)
	}))

	mux.HandleFunc("GET /admin/logs/entries", protected(deps.Auth, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		deps.LogsHandler.LogsEntries(w, r)
	}))

	mux.HandleFunc("GET /admin/vps", protected(deps.Auth, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		deps.VPSHandler.VPSPage(w, r)
	}))

	return server.SecurityHeaders(server.Logger(withScriptCollector(mux)))
}

// withScriptCollector scopes a scripts stack to the request, so page
// partials can Defer inline scripts for Base to Flush before </body>.
func withScriptCollector(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, _ := scripts.WithCollector(r.Context())

		next.ServeHTTP(w, r.WithContext(ctx))
	})
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
	// Allow override via environment variable for production deployments
	if dir := os.Getenv("STATIC_DIR"); dir != "" {
		return dir
	}

	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "web", "static")
}

// immutableFileServer serves uploaded files with a long immutable cache
// lifetime. Upload file names embed a content hash, so a changed file is
// always a new URL and browsers never need to revalidate.
func immutableFileServer(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// PDFs are served as downloads to avoid inline polyglot XSS.
		if strings.HasSuffix(strings.ToLower(r.URL.Path), ".pdf") {
			w.Header().Set("Content-Disposition", "attachment")
			w.Header().Set("X-Download-Options", "noopen")
		}
		fs.ServeHTTP(w, r)
	})
}

// cacheControlFileServer serves static assets with explicit caching.
func cacheControlFileServer(dir, cache string) http.Handler {
	fs := http.FileServer(http.Dir(dir))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", cache)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		fs.ServeHTTP(w, r)
	})
}

func healthCheck(r *http.Request, deps Deps) error {
	ctx := r.Context()

	if _, err := deps.Queries.CountUsers(ctx); err != nil {
		return err
	}

	return nil
}

func renderTemplate(t templ.Component, w http.ResponseWriter, r *http.Request) {
	t.Render(r.Context(), w)
}
