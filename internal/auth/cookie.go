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

// cookieConfig is set once at boot from env (see ConfigureCookies).
// BehindProxy enables X-Forwarded-Proto handling for deployments
// where TLS is terminated by a reverse proxy in front of the Go app.
var cookieBehindProxy bool

var cookieSecureMode = "auto"

// ConfigureCookies sets proxy/secure behavior for session cookies.
// Call once from main after config.Load. Mode is auto|always|never.
func ConfigureCookies(behindProxy bool, secureMode string) {
	cookieBehindProxy = behindProxy

	if secureMode != "" {
		cookieSecureMode = secureMode
	}
}

// isSecureRequest reports whether the original client request used HTTPS,
// including via X-Forwarded-Proto when behind a trusted proxy.
func isSecureRequest(r *http.Request) bool {
	if cookieSecureMode == "always" {
		return true
	}

	if cookieSecureMode == "never" {
		return false
	}

	if r.TLS != nil {
		return true
	}

	if cookieBehindProxy {
		proto := r.Header.Get("X-Forwarded-Proto")
		if proto == "https" {
			return true
		}
	}

	return false
}

// SetSessionCookie stores the raw session token in an HttpOnly cookie.
// Secure follows the request (or X-Forwarded-Proto behind a proxy,
// or COOKIE_SECURE=always in production).
func SetSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecureRequest(r),
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
		Secure:   isSecureRequest(r),
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
