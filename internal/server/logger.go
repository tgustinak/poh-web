package server

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Logger logs one structured line per request (method, path, status,
// duration). Static asset requests are skipped to keep logs readable.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") || strings.HasPrefix(r.URL.Path, "/uploads/") {
			next.ServeHTTP(w, r)

			return
		}

		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(recorder, r)

		slog.Info(
			"request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

// statusRecorder captures the status code written by the handler.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.status = code
	rec.ResponseWriter.WriteHeader(code)
}
