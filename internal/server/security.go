package server

import (
	"net/http"
)

// SecurityHeaders sets baseline hardening headers on every response.
// CSP allows inline scripts/styles because the site uses templ inline
// scripts; tightening to nonces is a future step. PDFs/uploads get
// extra protection at the uploads handler level.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=()")
		// Only sent over HTTPS by browsers; harmless on plain HTTP.
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; "+
			"script-src 'self' 'unsafe-inline'; "+
			"style-src 'self' 'unsafe-inline'; "+
			"img-src 'self' data: https:; "+
			"media-src 'self' https:; "+
			"font-src 'self' data:; "+
			"connect-src 'self'; "+
			"frame-src 'self' https://www.youtube-nocookie.com https://www.youtube.com https://open.spotify.com; "+
			"object-src 'none'; base-uri 'self'; form-action 'self'")

		next.ServeHTTP(w, r)
	})
}
