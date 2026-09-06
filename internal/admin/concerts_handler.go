// Package admin provides HTTP handlers for the admin section.
package admin

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/tgustinak/pohkapela.sk/internal/auth"
	"github.com/tgustinak/pohkapela.sk/internal/concerts"
	"github.com/tgustinak/pohkapela.sk/internal/db"
	"github.com/tgustinak/pohkapela.sk/internal/htmx"
	admintemplates "github.com/tgustinak/pohkapela.sk/web/templates/admin"
)

// ConcertsHandler serves admin concert management (CRUD).
type ConcertsHandler struct {
	queries *db.Queries
}

// NewConcertsHandler creates an admin concerts handler.
func NewConcertsHandler(queries *db.Queries) *ConcertsHandler {
	return &ConcertsHandler{queries: queries}
}

// ListPage renders the concert management page with upcoming
// and past concerts.
func (h *ConcertsHandler) ListPage(w http.ResponseWriter, r *http.Request) {
	csrf, _ := auth.CsrfFromRequest(r)

	upcoming, err := h.queries.ListUpcomingConcerts(r.Context(), concerts.Today())
	if err != nil {
		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	past, err := h.queries.ListPastConcerts(r.Context(), concerts.Today())
	if err != nil {
		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	admintemplates.AdminConcerts(upcoming, past, csrf).Render(r.Context(), w)
}

// Create adds a concert. htmx requests get the refreshed tables plus
// a toast; plain form posts redirect back to the list.
func (h *ConcertsHandler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.createFailed(w, r, "Nepodarilo sa spracovať formulár, skús to znova.")

		return
	}

	name, date, errMsg := parseConcertForm(r)
	if errMsg != "" {
		h.createFailed(w, r, errMsg)

		return
	}

	if _, err := h.queries.CreateConcert(r.Context(), db.CreateConcertParams{
		Name:      name,
		EventDate: date,
	}); err != nil {
		h.createFailed(w, r, "Koncert sa nepodarilo uložiť, skús to znova.")

		return
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/concerts")

		return
	}

	htmx.Trigger(w, map[string]string{
		"showToast":        "Koncert pridaný.",
		"concertFormReset": "true",
	})
	h.renderTables(w, r)
}

// Row renders a single concert table row (also the cancel target
// for inline editing).
func (h *ConcertsHandler) Row(w http.ResponseWriter, r *http.Request) {
	concert, ok := h.findConcert(w, r)
	if !ok {
		return
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/concerts")

		return
	}

	admintemplates.AdminConcertRow(concert).Render(r.Context(), w)
}

// EditForm renders the inline edit row for a concert.
func (h *ConcertsHandler) EditForm(w http.ResponseWriter, r *http.Request) {
	concert, ok := h.findConcert(w, r)
	if !ok {
		return
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/concerts")

		return
	}

	admintemplates.AdminConcertEditRow(concert, "").Render(r.Context(), w)
}

// Update saves an edited concert. On success it returns the updated
// row plus a toast; on validation error the edit form with the message.
func (h *ConcertsHandler) Update(w http.ResponseWriter, r *http.Request) {
	concert, ok := h.findConcert(w, r)
	if !ok {
		return
	}

	if err := r.ParseForm(); err != nil {
		h.updateFailed(w, r, concert, "Nepodarilo sa spracovať formulár, skús to znova.")

		return
	}

	name, date, errMsg := parseConcertForm(r)
	if errMsg != "" {
		h.updateFailed(w, r, concert, errMsg)

		return
	}

	updated, err := h.queries.UpdateConcert(r.Context(), db.UpdateConcertParams{
		Name:      name,
		EventDate: date,
		ID:        concert.ID,
	})
	if err != nil {
		h.updateFailed(w, r, concert, "Koncert sa nepodarilo uložiť, skús to znova.")

		return
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/concerts")

		return
	}

	htmx.Toast(w, "Koncert uložený.")
	admintemplates.AdminConcertRow(updated).Render(r.Context(), w)
}

// Delete removes a concert. The htmx swap deletes the row client-side.
func (h *ConcertsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	concert, ok := h.findConcert(w, r)
	if !ok {
		return
	}

	if err := h.queries.DeleteConcert(r.Context(), concert.ID); err != nil {
		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/concerts")

		return
	}

	htmx.Toast(w, "Koncert zmazaný.")
}

// createFailed answers a failed create: toast for htmx (tables are
// re-rendered unchanged), plain error otherwise.
func (h *ConcertsHandler) createFailed(w http.ResponseWriter, r *http.Request, msg string) {
	if !htmx.IsHtmxRequest(r) {
		http.Error(w, msg, http.StatusBadRequest)

		return
	}

	htmx.Toast(w, msg)
	h.renderTables(w, r)
}

// updateFailed answers a failed update: the edit form with an inline
// error for htmx, plain error otherwise.
func (h *ConcertsHandler) updateFailed(w http.ResponseWriter, r *http.Request, concert db.Concert, msg string) {
	if !htmx.IsHtmxRequest(r) {
		http.Error(w, msg, http.StatusBadRequest)

		return
	}

	admintemplates.AdminConcertEditRow(concert, msg).Render(r.Context(), w)
}

// renderTables renders both concert tables (upcoming + past).
func (h *ConcertsHandler) renderTables(w http.ResponseWriter, r *http.Request) {
	upcoming, err := h.queries.ListUpcomingConcerts(r.Context(), concerts.Today())
	if err != nil {
		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	past, err := h.queries.ListPastConcerts(r.Context(), concerts.Today())
	if err != nil {
		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	admintemplates.AdminConcertTables(upcoming, past).Render(r.Context(), w)
}

// findConcert loads the concert addressed by the {id} path value.
// It writes 404 when the id is invalid or the concert does not exist.
func (h *ConcertsHandler) findConcert(w http.ResponseWriter, r *http.Request) (db.Concert, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "Koncert sa nenašiel.", http.StatusNotFound)

		return db.Concert{}, false
	}

	concert, err := h.queries.GetConcertByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Koncert sa nenašiel.", http.StatusNotFound)

			return db.Concert{}, false
		}

		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return db.Concert{}, false
	}

	return concert, true
}

// parseConcertForm validates the concert form fields.
// It returns the cleaned values or a Slovak error message.
func parseConcertForm(r *http.Request) (string, string, string) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		return "", "", "Zadaj názov koncertu."
	}

	if len([]rune(name)) > 200 {
		return "", "", "Názov je príliš dlhý (max 200 znakov)."
	}

	date := strings.TrimSpace(r.FormValue("event_date"))
	if _, err := concerts.ParseDate(date); err != nil {
		return "", "", "Zadaj platný dátum."
	}

	return name, date, ""
}
