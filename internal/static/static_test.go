package static

import (
	"strings"
	"testing"
)

func TestURLKnownFile(t *testing.T) {
	got := URL("js/htmx.min.js")
	if !strings.HasPrefix(got, "/static/js/htmx.min.js?v=") {
		t.Fatalf("expected versioned URL, got %q", got)
	}
}

func TestURLUnknownFile(t *testing.T) {
	if got := URL("does/not-exist.css"); got != "/static/does/not-exist.css" {
		t.Fatalf("expected plain fallback, got %q", got)
	}
}
