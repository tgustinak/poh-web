// Package admin provides HTTP handlers for the admin section.
package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf16"

	"github.com/tgustinak/pohkapela.sk/internal/auth"
	admintemplates "github.com/tgustinak/pohkapela.sk/web/templates/admin"
)

// Handler serves admin login, logout and session-aware redirects.
type Handler struct {
	auth *auth.Service
}

// NewHandler creates an admin handler.
func NewHandler(authSvc *auth.Service) *Handler {
	return &Handler{auth: authSvc}
}

// isHtmx reports whether the request was made by htmx.
// htmx requests carry an HX-Request header and expect HX-Redirect /
// HX-Trigger driven responses instead of full-page renders.
func isHtmx(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// redirect navigates to target: via HX-Redirect for htmx requests,
// via a 303 redirect otherwise (progressive enhancement fallback).
func redirect(w http.ResponseWriter, r *http.Request, target string) {
	if isHtmx(r) {
		w.Header().Set("HX-Redirect", target)
		w.WriteHeader(http.StatusOK)

		return
	}

	http.Redirect(w, r, target, http.StatusSeeOther)
}

// toast notifies htmx clients with a toast message via HX-Trigger.
// The admin shell listens for the showToast event (see AdminScripts).
// A 200 status is used deliberately: htmx skips trigger processing
// for error responses.
func toast(w http.ResponseWriter, msg string) {
	payload, err := headerJSON(map[string]string{"showToast": msg})
	if err != nil {
		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	w.Header().Set("HX-Trigger", payload)
	w.WriteHeader(http.StatusOK)
}

// headerJSON marshals v to JSON with all non-ASCII characters escaped
// as \u sequences, so the result is pure ASCII and safe to send in an
// HTTP header. Browsers decode response headers as Latin-1, so raw
// UTF-8 bytes would arrive in JavaScript as mojibake
// (e.g. "á" becoming "Ã¡").
func headerJSON(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}

	var out strings.Builder

	for _, r := range string(raw) {
		if r <= unicode.MaxASCII {
			out.WriteRune(r)

			continue
		}

		if r > 0xFFFF {
			hi, lo := utf16.EncodeRune(r)
			fmt.Fprintf(&out, "\\u%04x\\u%04x", hi, lo)

			continue
		}

		fmt.Fprintf(&out, "\\u%04x", r)
	}

	return out.String(), nil
}

// LoginPage renders the login form, redirecting already logged-in admins
// to the dashboard.
func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	if _, err := h.auth.Authenticate(r.Context(), auth.SessionToken(r)); err == nil {
		http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)

		return
	}

	auth.ClearSessionCookie(w)
	admintemplates.AdminLogin("").Render(r.Context(), w)
}

// Login verifies credentials, sets the session cookie and redirects
// to the dashboard. htmx requests get HX-Redirect on success and a
// showToast trigger on failure; plain form posts get full-page responses.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		if isHtmx(r) {
			toast(w, "Nepodarilo sa spracovať formulár, skús to znova.")

			return
		}

		w.WriteHeader(http.StatusBadRequest)
		admintemplates.AdminLogin("Nepodarilo sa spracovať formulár, skús to znova.").Render(r.Context(), w)

		return
	}

	token, err := h.auth.Login(r.Context(), r.FormValue("email"), r.FormValue("password"))
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			if isHtmx(r) {
				toast(w, "Nesprávny e-mail alebo heslo.")

				return
			}

			w.WriteHeader(http.StatusUnauthorized)
			admintemplates.AdminLogin("Nesprávny e-mail alebo heslo.").Render(r.Context(), w)

			return
		}

		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	auth.SetSessionCookie(w, r, token)
	redirect(w, r, "/admin/dashboard")
}

// Logout deletes the current session and redirects to the login page.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	_ = h.auth.Logout(r.Context(), auth.SessionToken(r))

	auth.ClearSessionCookie(w)
	redirect(w, r, "/admin")
}
