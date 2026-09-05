// Package admin provides HTTP handlers for the admin section.
package admin

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/tgustinak/pohkapela.sk/internal/auth"
	admintemplates "github.com/tgustinak/pohkapela.sk/web/templates/admin"
)

// AuthHandler serves admin login, logout and session-aware redirects.
type AuthHandler struct {
	auth *auth.Service
}

// NewAuthHandler creates an admin auth handler.
func NewAuthHandler(authSvc *auth.Service) *AuthHandler {
	return &AuthHandler{auth: authSvc}
}

// LoginPage renders the login form, redirecting already logged-in admins
// to the dashboard.
func (h *AuthHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
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
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
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
			auth.Toast(w, auth.CsrfFailureMessage)

			return
		}

		http.Error(w, auth.CsrfFailureMessage, http.StatusForbidden)

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
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	_ = h.auth.Logout(r.Context(), auth.SessionToken(r))

	auth.ClearSessionCookie(w)
	auth.ClearCsrfCookie(w)
	auth.Redirect(w, r, "/admin")
}
