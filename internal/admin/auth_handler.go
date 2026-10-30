// Package admin provides HTTP handlers for the admin section.
package admin

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/tgustinak/pohkapela.sk/internal/auth"
	"github.com/tgustinak/pohkapela.sk/internal/htmx"
	admintemplates "github.com/tgustinak/pohkapela.sk/web/templates/admin"
)

// AuthHandler serves admin login, logout and session-aware redirects.
type AuthHandler struct {
	auth        *auth.Service
	limiter     *auth.LoginRateLimiter
	behindProxy bool
}

// NewAuthHandler creates an admin auth handler.
func NewAuthHandler(authSvc *auth.Service) *AuthHandler {
	return NewAuthHandlerWithLimiter(authSvc, auth.DefaultLoginRateLimiter(), false)
}

// NewAuthHandlerWithLimiter creates a handler with explicit limiter/proxy
// config (used by production wiring and tests).
func NewAuthHandlerWithLimiter(authSvc *auth.Service, limiter *auth.LoginRateLimiter, behindProxy bool) *AuthHandler {
	if limiter == nil {
		limiter = auth.DefaultLoginRateLimiter()
	}

	return &AuthHandler{auth: authSvc, limiter: limiter, behindProxy: behindProxy}
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
	// Cap login body to 1MB before parsing (credential fields are tiny).
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	ip := auth.ClientIP(r, h.behindProxy)
	if h.limiter.Blocked(ip) {
		slog.Warn("admin login rate-limited", "ip", ip)

		if htmx.IsHtmxRequest(r) {
			htmx.Toast(w, "Príliš veľa pokusov. Skús to znova o pár minút.")

			return
		}

		http.Error(w, "Príliš veľa pokusov. Skús to znova o pár minút.", http.StatusTooManyRequests)

		return
	}

	if err := r.ParseForm(); err != nil {
		if htmx.IsHtmxRequest(r) {
			htmx.Toast(w, "Nepodarilo sa spracovať formulár, skús to znova.")

			return
		}

		w.WriteHeader(http.StatusBadRequest)
		admintemplates.AdminLogin("Nepodarilo sa spracovať formulár, skús to znova.", auth.CsrfCookieValue(r)).Render(r.Context(), w)

		return
	}

	if !auth.ValidCsrf(r, auth.CsrfCookieValue(r)) {
		if htmx.IsHtmxRequest(r) {
			htmx.Toast(w, auth.CsrfFailureMessage)

			return
		}

		http.Error(w, auth.CsrfFailureMessage, http.StatusForbidden)

		return
	}

	sessionToken, csrfToken, err := h.auth.Login(r.Context(), r.FormValue("email"), r.FormValue("password"))
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			slog.Warn("admin login failed", "ip", ip)

			if locked := h.limiter.RecordFailure(ip); locked {
				slog.Warn("admin login locked out", "ip", ip)
			}

			if htmx.IsHtmxRequest(r) {
				htmx.Toast(w, "Nesprávny e-mail alebo heslo.")

				return
			}

			w.WriteHeader(http.StatusUnauthorized)
			admintemplates.AdminLogin("Nesprávny e-mail alebo heslo.", auth.CsrfCookieValue(r)).Render(r.Context(), w)

			return
		}

		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	h.limiter.Reset(ip)
	auth.SetSessionCookie(w, r, sessionToken)
	auth.SetCsrfCookie(w, r, csrfToken)
	htmx.Redirect(w, r, "/admin/dashboard")
}

// Logout deletes the current session and redirects to the login page.
// The route is wrapped in RequireAuth + RequireCsrf (see app mux).
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	_ = h.auth.Logout(r.Context(), auth.SessionToken(r))

	auth.ClearSessionCookie(w)
	auth.ClearCsrfCookie(w)
	htmx.Redirect(w, r, "/admin")
}
