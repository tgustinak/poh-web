package admin

import (
	"net/http"
	"strconv"

	"github.com/tgustinak/pohkapela.sk/internal/auth"
	"github.com/tgustinak/pohkapela.sk/internal/logbuffer"
	admintemplates "github.com/tgustinak/pohkapela.sk/web/templates/admin"
)

// LogsHandler serves the admin log viewer.
type LogsHandler struct {
	logs *logbuffer.Buffer
}

// NewLogsHandler creates an admin logs handler.
func NewLogsHandler(logs *logbuffer.Buffer) *LogsHandler {
	return &LogsHandler{logs: logs}
}

// LogsPage renders the log viewer with the newest entries.
func (h *LogsHandler) LogsPage(w http.ResponseWriter, r *http.Request) {
	csrf, _ := auth.CsrfFromRequest(r)

	entries, next := h.logs.Recent(logbuffer.DefaultPageSize)

	admintemplates.AdminLogs(entries, next, csrf).Render(r.Context(), w)
}

// LogsEntries renders log rows for htmx infinite scroll (older pages)
// and for the refresh button (newest page). Query params:
// before=<seq> loads entries older than seq, limit caps page size.
func (h *LogsHandler) LogsEntries(w http.ResponseWriter, r *http.Request) {
	before, limit := logPageParams(r)

	var entries []logbuffer.Entry
	var next uint64
	if before == 0 {
		entries, next = h.logs.Recent(limit)
	} else {
		entries, next = h.logs.Before(before, limit)
	}

	admintemplates.LogRows(entries, next, before != 0).Render(r.Context(), w)
}

func logPageParams(r *http.Request) (uint64, int) {
	limit := logbuffer.DefaultPageSize
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}

	var before uint64
	if v := r.URL.Query().Get("before"); v != "" {
		before, _ = strconv.ParseUint(v, 10, 64)
	}

	return before, limit
}
