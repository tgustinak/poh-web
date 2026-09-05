package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tgustinak/pohkapela.sk/internal/logbuffer"
)

func TestLogsPageRendersEntries(t *testing.T) {
	_, logsHandler, _ := newTestHandlers(t)

	logsHandler.logs.Append(logbuffer.Entry{Level: "WARN", TimeLabel: "12:00:00", Message: "disk almost full", Attrs: "free=5%"})

	req := httptest.NewRequest(http.MethodGet, "/admin/logy", nil)
	rec := httptest.NewRecorder()

	logsHandler.LogsPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	for _, want := range []string{"Logy", "disk almost full", "WARN"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected logs page to contain %q", want)
		}
	}
}

func TestLogsEntriesPaginates(t *testing.T) {
	_, logsHandler, _ := newTestHandlers(t)

	for i := 0; i < 5; i++ {
		logsHandler.logs.Append(logbuffer.Entry{Level: "ERROR", TimeLabel: "12:00:00", Message: "boom"})
	}

	getEntries := func(query string) string {
		t.Helper()

		req := httptest.NewRequest(http.MethodGet, "/admin/logy/entries"+query, nil)
		rec := httptest.NewRecorder()

		logsHandler.LogsEntries(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected entries 200, got %d", rec.Code)
		}

		return rec.Body.String()
	}

	page1 := getEntries("?limit=2")
	if strings.Count(page1, "boom") != 2 {
		t.Fatalf("expected 2 rows on first page, got %q", page1)
	}

	if !strings.Contains(page1, "log-more") {
		t.Fatalf("expected infinite-scroll sentinel on first page, got %q", page1)
	}

	before := logsCursor(t, page1)

	page2 := getEntries("?limit=2&before=" + before)
	if strings.Count(page2, "boom") != 2 {
		t.Fatalf("expected 2 rows on second page, got %q", page2)
	}

	before2 := logsCursor(t, page2)

	page3 := getEntries("?limit=2&before=" + before2)
	if strings.Count(page3, "boom") != 1 {
		t.Fatalf("expected 1 row on last page, got %q", page3)
	}

	if strings.Contains(page3, "log-more") {
		t.Fatalf("expected no sentinel on last page, got %q", page3)
	}

	if !strings.Contains(page3, "Koniec záznamov.") {
		t.Fatalf("expected end marker on last page, got %q", page3)
	}
}

func logsCursor(t *testing.T, body string) string {
	t.Helper()

	const prefix = "before="
	idx := strings.Index(body, prefix)
	if idx < 0 {
		t.Fatalf("expected pagination cursor, got %q", body)
	}

	rest := body[idx+len(prefix):]
	end := strings.IndexFunc(rest, func(r rune) bool {
		return r < '0' || r > '9'
	})
	if end < 0 {
		end = len(rest)
	}

	return rest[:end]
}
