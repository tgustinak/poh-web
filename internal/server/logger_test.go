package server

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWithDefaults(t *testing.T) {
	opts := NewServerOptions{}.withDefaults()

	if opts.ReadTimeout != defaultReadTimeout {
		t.Fatalf("expected default read timeout, got %v", opts.ReadTimeout)
	}

	if opts.WriteTimeout != defaultWriteTimeout {
		t.Fatalf("expected default write timeout, got %v", opts.WriteTimeout)
	}

	if opts.IdleTimeout != defaultIdleTimeout {
		t.Fatalf("expected default idle timeout, got %v", opts.IdleTimeout)
	}

	custom := NewServerOptions{ReadTimeout: time.Second}.withDefaults()
	if custom.ReadTimeout != time.Second {
		t.Fatalf("expected custom read timeout to be kept, got %v", custom.ReadTimeout)
	}

	if custom.WriteTimeout != defaultWriteTimeout {
		t.Fatalf("expected default write timeout, got %v", custom.WriteTimeout)
	}
}

func TestLogger(t *testing.T) {
	var logs bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))

	handler := Logger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/dashboard", nil))

	if rec.Code != http.StatusTeapot {
		t.Fatalf("expected status to pass through, got %d", rec.Code)
	}

	out := logs.String()
	for _, want := range []string{"method=GET", "path=/admin/dashboard", "status=418"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected log to contain %q, got %q", want, out)
		}
	}
}

func TestLoggerSkipsStatic(t *testing.T) {
	var logs bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))

	handler := Logger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/js/htmx.min.js", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status to pass through, got %d", rec.Code)
	}

	if logs.String() != "" {
		t.Fatalf("expected no log for static assets, got %q", logs.String())
	}
}
