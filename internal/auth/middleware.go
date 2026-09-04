package auth

import (
	"context"
	"net/http"

	"github.com/tgustinak/pohkapela.sk/internal/db"
)

// userContextKey carries the authenticated user in the request context.
type userContextKey struct{}

// RequireAuth redirects unauthenticated requests to the admin login page.
// Authenticated requests continue with the user stored in the context.
func (s *Service) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := s.Authenticate(r.Context(), SessionToken(r))
		if err != nil {
			ClearSessionCookie(w)
			http.Redirect(w, r, "/admin", http.StatusSeeOther)

			return
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, user)))
	})
}

// UserFromRequest returns the authenticated user stored by RequireAuth.
func UserFromRequest(r *http.Request) (*db.User, bool) {
	user, ok := r.Context().Value(userContextKey{}).(*db.User)

	return user, ok
}
