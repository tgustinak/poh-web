// Package admin provides HTTP handlers for the admin section.
package admin

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/tgustinak/pohkapela.sk/internal/auth"
	"github.com/tgustinak/pohkapela.sk/internal/logbuffer"
	admintemplates "github.com/tgustinak/pohkapela.sk/web/templates/admin"
)

// csrfFailure is shown when a request carries a missing or invalid CSRF token.
const csrfFailure = "Neplatný bezpečnostný token. Obnov stránku a skús znova."

// Handler serves admin login, logout and session-aware redirects.
type Handler struct {
	auth *auth.Service
	logs *logbuffer.Buffer
}

// NewHandler creates an admin handler.
func NewHandler(authSvc *auth.Service, logs *logbuffer.Buffer) *Handler {
	return &Handler{auth: authSvc, logs: logs}
}

// LoginPage renders the login form, redirecting already logged-in admins
// to the dashboard.
func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	if _, _, err := h.auth.Authenticate(r.Context(), auth.SessionToken(r)); err == nil {
		http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)

		return
	}

	auth.ClearSessionCookie(w)

	csrf, err := auth.EnsureCsrfCookie(w, r)
	if err != nil {
		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	admintemplates.AdminLogin("", csrf).Render(r.Context(), w)
}

// Login verifies the CSRF token, then credentials, sets the session
// cookie and redirects to the dashboard. htmx requests get HX-Redirect
// on success and a showToast trigger on failure; plain form posts get
// full-page responses.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		if auth.IsHtmxRequest(r) {
			auth.Toast(w, "Nepodarilo sa spracovať formulár, skús to znova.")

			return
		}

		w.WriteHeader(http.StatusBadRequest)
		admintemplates.AdminLogin("Nepodarilo sa spracovať formulár, skús to znova.", auth.CsrfCookieValue(r)).Render(r.Context(), w)

		return
	}

	if !auth.ValidCsrf(r, auth.CsrfCookieValue(r)) {
		if auth.IsHtmxRequest(r) {
			auth.Toast(w, csrfFailure)

			return
		}

		http.Error(w, csrfFailure, http.StatusForbidden)

		return
	}

	sessionToken, csrfToken, err := h.auth.Login(r.Context(), r.FormValue("email"), r.FormValue("password"))
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			slog.Warn("admin login failed", "email", r.FormValue("email"))

			if auth.IsHtmxRequest(r) {
				auth.Toast(w, "Nesprávny e-mail alebo heslo.")

				return
			}

			w.WriteHeader(http.StatusUnauthorized)
			admintemplates.AdminLogin("Nesprávny e-mail alebo heslo.", auth.CsrfCookieValue(r)).Render(r.Context(), w)

			return
		}

		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	auth.SetSessionCookie(w, r, sessionToken)
	auth.SetCsrfCookie(w, r, csrfToken)
	auth.Redirect(w, r, "/admin/dashboard")
}

// Logout deletes the current session and redirects to the login page.
// The route is wrapped in RequireAuth + RequireCsrf (see app mux).
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	_ = h.auth.Logout(r.Context(), auth.SessionToken(r))

	auth.ClearSessionCookie(w)
	auth.ClearCsrfCookie(w)
	auth.Redirect(w, r, "/admin")
}

// LogsPage renders the log viewer with the newest entries.
func (h *Handler) LogsPage(w http.ResponseWriter, r *http.Request) {
	csrf, _ := auth.CsrfFromRequest(r)

	entries, next := h.logs.Recent(logbuffer.DefaultPageSize)

	admintemplates.AdminLogs(entries, next, csrf).Render(r.Context(), w)
}

// LogsEntries renders log rows for htmx infinite scroll (older pages)
// and for the refresh button (newest page). Query params:
// before=<seq> loads entries older than seq, limit caps page size.
func (h *Handler) LogsEntries(w http.ResponseWriter, r *http.Request) {
	before, limit := logPageParams(r)

	var entries []logbuffer.Entry
	var next uint64
	if before == 0 {
		entries, next = h.logs.Recent(limit)
	} else {
		entries, next = h.logs.Before(before, limit)
	}

	admintemplates.LogRows(entries, next, before != 0).Render(r.Context(), w)
}

func logPageParams(r *http.Request) (uint64, int) {
	limit := logbuffer.DefaultPageSize
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}

	var before uint64
	if v := r.URL.Query().Get("before"); v != "" {
		before, _ = strconv.ParseUint(v, 10, 64)
	}

	return before, limit
}
