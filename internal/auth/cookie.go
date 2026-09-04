package auth

import (
	"net/http"
	"time"
)

// CookieName is the name of the admin session cookie.
const CookieName = "poh_session"

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
