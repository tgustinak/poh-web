package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	SecurityHeaders(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	for _, h := range []string{
		"X-Content-Type-Options",
		"X-Frame-Options",
		"Referrer-Policy",
		"Strict-Transport-Security",
		"Content-Security-Policy",
	} {
		if rec.Header().Get(h) == "" {
			t.Fatalf("expected header %s to be set", h)
		}
	}
}
