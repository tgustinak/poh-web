package auth

import (
	"net/http"
	"time"
)

// CookieName is the name of the admin session cookie.
const CookieName = "poh_session"

// CsrfCookieName carries the CSRF token: an anonymous one before login
// (double-submit with the login form) and the session-bound one after.
const CsrfCookieName = "poh_csrf"

// SetSessionCookie stores the raw session token in an HttpOnly cookie.
// Secure is enabled automatically for HTTPS requests.
func SetSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().UTC().Add(SessionTTL),
	})
}

// ClearSessionCookie removes the session cookie from the client.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

// SessionToken extracts the raw session token from the request cookie.
func SessionToken(r *http.Request) string {
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		return ""
	}

	return cookie.Value
}

// EnsureCsrfCookie returns the CSRF cookie value, setting a fresh random
// one when missing. Used for the anonymous login form.
func EnsureCsrfCookie(w http.ResponseWriter, r *http.Request) (string, error) {
	if cookie, err := r.Cookie(CsrfCookieName); err == nil && cookie.Value != "" {
		return cookie.Value, nil
	}

	token, err := newToken()
	if err != nil {
		return "", err
	}

	SetCsrfCookie(w, r, token)

	return token, nil
}

// SetCsrfCookie stores the CSRF token in an HttpOnly cookie.
// The token is also rendered into pages (meta tag, hidden inputs),
// so JavaScript never needs to read the cookie itself.
func SetCsrfCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CsrfCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearCsrfCookie removes the CSRF cookie from the client.
func ClearCsrfCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CsrfCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

// CsrfCookieValue extracts the CSRF token from the request cookie.
func CsrfCookieValue(r *http.Request) string {
	cookie, err := r.Cookie(CsrfCookieName)
	if err != nil {
		return ""
	}

	return cookie.Value
}
