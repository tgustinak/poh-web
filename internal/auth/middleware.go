package auth

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"

	"github.com/tgustinak/pohkapela.sk/internal/db"
)

// CsrfHeaderName carries the CSRF token on htmx requests
// (set automatically from the meta tag, see AdminScripts).
const CsrfHeaderName = "X-CSRF-Token"

// CsrfFieldName carries the CSRF token in plain HTML form posts
// (hidden input, no-JS fallback).
const CsrfFieldName = "csrf_token"

// userContextKey carries the authenticated user in the request context.
type userContextKey struct{}

// csrfContextKey carries the session CSRF token in the request context.
type csrfContextKey struct{}

// RequireAuth redirects unauthenticated requests to the admin login page.
// Authenticated requests continue with the user and the session CSRF
// token stored in the context.
func (s *Service) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, user, err := s.Authenticate(r.Context(), SessionToken(r))
		if err != nil {
			ClearSessionCookie(w)
			ClearCsrfCookie(w)

			if IsHtmxRequest(r) {
				w.Header().Set("HX-Redirect", "/admin")
				w.WriteHeader(http.StatusOK)

				return
			}

			http.Redirect(w, r, "/admin", http.StatusSeeOther)

			return
		}

		ctx := context.WithValue(r.Context(), userContextKey{}, user)
		ctx = context.WithValue(ctx, csrfContextKey{}, session.CsrfToken)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// CsrfFailureMessage is shown when a request carries a missing
// or invalid CSRF token.
const CsrfFailureMessage = "Neplatný bezpečnostný token. Obnov stránku a skús znova."

// RequireCsrf rejects unsafe methods (POST, PUT, PATCH, DELETE) without
// a valid CSRF token. The expected token comes from the session
// (stored in the context by RequireAuth, which must run first).
// Safe methods pass through untouched.
func RequireCsrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isUnsafeMethod(r.Method) {
			next.ServeHTTP(w, r)

			return
		}

		expected, _ := CsrfFromRequest(r)
		if !ValidCsrf(r, expected) {
			slog.Warn("csrf check failed", "method", r.Method, "path", r.URL.Path)

			if IsHtmxRequest(r) {
				Toast(w, CsrfFailureMessage)

				return
			}

			http.Error(w, CsrfFailureMessage, http.StatusForbidden)

			return
		}

		next.ServeHTTP(w, r)
	})
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// ValidCsrf reports whether r carries the expected CSRF token, read from
// the X-CSRF-Token header (htmx) or the csrf_token form field
// (plain HTML forms). Comparison is constant-time and empty tokens
// never validate.
func ValidCsrf(r *http.Request, expected string) bool {
	if expected == "" {
		return false
	}

	got := r.Header.Get(CsrfHeaderName)
	if got == "" {
		got = r.FormValue(CsrfFieldName)
	}

	if got == "" {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

// UserFromRequest returns the authenticated user stored by RequireAuth.
func UserFromRequest(r *http.Request) (*db.User, bool) {
	user, ok := r.Context().Value(userContextKey{}).(*db.User)

	return user, ok
}

// CsrfFromRequest returns the session CSRF token stored by RequireAuth.
func CsrfFromRequest(r *http.Request) (string, bool) {
	token, ok := r.Context().Value(csrfContextKey{}).(string)

	return token, ok && token != ""
}
