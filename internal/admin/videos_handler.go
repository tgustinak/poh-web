package admin

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/tgustinak/pohkapela.sk/internal/auth"
	"github.com/tgustinak/pohkapela.sk/internal/db"
	"github.com/tgustinak/pohkapela.sk/internal/htmx"
	"github.com/tgustinak/pohkapela.sk/internal/videos"
	admintemplates "github.com/tgustinak/pohkapela.sk/web/templates/admin"
)

// VideosHandler serves admin videoclip management (CRUD + reorder).
type VideosHandler struct {
	queries *db.Queries
}

// NewVideosHandler creates an admin videos handler.
func NewVideosHandler(queries *db.Queries) *VideosHandler {
	return &VideosHandler{queries: queries}
}

// ListPage renders the videoclip management page in playlist order.
func (h *VideosHandler) ListPage(w http.ResponseWriter, r *http.Request) {
	csrf, _ := auth.CsrfFromRequest(r)

	rows, err := h.queries.ListVideosOrdered(r.Context())
	if err != nil {
		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	admintemplates.AdminVideos(rows, csrf).Render(r.Context(), w)
}

// Create adds a videoclip. htmx requests get the refreshed list plus
// a toast; plain form posts redirect back to the list.
func (h *VideosHandler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.createFailed(w, r, "Nepodarilo sa spracovať formulár, skús to znova.")

		return
	}

	params, errMsg := parseVideoForm(r, 0)
	if errMsg != "" {
		h.createFailed(w, r, errMsg)

		return
	}

	// New clips go first in the playlist: shift existing clips down
	// and insert the new one at position 1.
	if err := h.queries.ShiftVideoPositions(r.Context()); err != nil {
		h.createFailed(w, r, "Klip sa nepodarilo uložiť, skús to znova.")

		return
	}

	created, err := h.queries.CreateVideo(r.Context(), db.CreateVideoParams{
		YoutubeID:     params.YoutubeID,
		Title:         params.Title,
		PublishedDate: params.PublishedDate,
		Duration:      params.Duration,
		Position:      1,
	})
	if err != nil {
		if isDuplicateVideo(err) {
			h.createFailed(w, r, "Tento YouTube klip už v zozname existuje.")

			return
		}

		h.createFailed(w, r, "Klip sa nepodarilo uložiť, skús to znova.")

		return
	}
	_ = created

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/videos")

		return
	}

	htmx.Trigger(w, htmx.Events{
		ShowToast: "Videoklip pridaný.",
		FormReset: "video-create-form",
	})
	h.renderList(w, r)
}

// Row renders a single video table row (also the cancel target
// for inline editing).
func (h *VideosHandler) Row(w http.ResponseWriter, r *http.Request) {
	video, ok := h.findVideo(w, r)
	if !ok {
		return
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/videos")

		return
	}

	admintemplates.AdminVideoRow(video, int(video.Position)).Render(r.Context(), w)
}

// EditForm renders the inline edit row for a video.
func (h *VideosHandler) EditForm(w http.ResponseWriter, r *http.Request) {
	video, ok := h.findVideo(w, r)
	if !ok {
		return
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/videos")

		return
	}

	admintemplates.AdminVideoEditRow(video, "").Render(r.Context(), w)
}

// Update saves an edited videoclip. On success it returns the updated
// row plus a toast; on validation error the edit form with the message.
func (h *VideosHandler) Update(w http.ResponseWriter, r *http.Request) {
	video, ok := h.findVideo(w, r)
	if !ok {
		return
	}

	if err := r.ParseForm(); err != nil {
		h.updateFailed(w, r, video, "Nepodarilo sa spracovať formulár, skús to znova.")

		return
	}

	params, errMsg := parseVideoForm(r, video.Position)
	if errMsg != "" {
		h.updateFailed(w, r, video, errMsg)

		return
	}

	updated, err := h.queries.UpdateVideo(r.Context(), db.UpdateVideoParams{
		YoutubeID:     params.YoutubeID,
		Title:         params.Title,
		PublishedDate: params.PublishedDate,
		Duration:      params.Duration,
		ID:            video.ID,
	})
	if err != nil {
		if isDuplicateVideo(err) {
			h.updateFailed(w, r, video, "Tento YouTube klip už v zozname existuje.")

			return
		}

		h.updateFailed(w, r, video, "Klip sa nepodarilo uložiť, skús to znova.")

		return
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/videos")

		return
	}

	htmx.Toast(w, "Klip uložený.")
	admintemplates.AdminVideoRow(updated, int(updated.Position)).Render(r.Context(), w)
}

// Delete removes a videoclip. The htmx swap deletes the row client-side.
func (h *VideosHandler) Delete(w http.ResponseWriter, r *http.Request) {
	video, ok := h.findVideo(w, r)
	if !ok {
		return
	}

	if err := h.queries.DeleteVideo(r.Context(), video.ID); err != nil {
		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/videos")

		return
	}

	htmx.Toast(w, "Klip zmazaný.")
}

// Reorder saves the playlist order sent by drag & drop.
// It expects repeated "id" fields in the new order (position 1 = first).
// Unknown or duplicate ids are rejected; missing ids keep the request
// atomic by failing instead of partially reordering.
func (h *VideosHandler) Reorder(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.reorderFailed(w, r, "Poradie sa nepodarilo uložiť, skús to znova.")

		return
	}

	rawIDs := r.Form["id"]
	if len(rawIDs) == 0 {
		h.reorderFailed(w, r, "Poradie sa nepodarilo uložiť, skús to znova.")

		return
	}

	ids := make([]int64, 0, len(rawIDs))
	seen := make(map[int64]struct{}, len(rawIDs))

	for _, raw := range rawIDs {
		id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil || id <= 0 {
			h.reorderFailed(w, r, "Poradie sa nepodarilo uložiť, skús to znova.")

			return
		}

		if _, dup := seen[id]; dup {
			h.reorderFailed(w, r, "Poradie sa nepodarilo uložiť, skús to znova.")

			return
		}

		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	rows, err := h.queries.ListVideosOrdered(r.Context())
	if err != nil {
		h.reorderFailed(w, r, "Poradie sa nepodarilo uložiť, skús to znova.")

		return
	}

	if len(rows) != len(ids) {
		h.reorderFailed(w, r, "Zoznam sa medzitým zmenil, obnov stránku a skús znova.")

		return
	}

	known := make(map[int64]struct{}, len(rows))
	for _, v := range rows {
		known[v.ID] = struct{}{}
	}

	for _, id := range ids {
		if _, ok := known[id]; !ok {
			h.reorderFailed(w, r, "Zoznam sa medzitým zmenil, obnov stránku a skús znova.")

			return
		}
	}

	for i, id := range ids {
		if err := h.queries.UpdateVideoPosition(r.Context(), db.UpdateVideoPositionParams{
			Position: int64(i + 1),
			ID:       id,
		}); err != nil {
			h.reorderFailed(w, r, "Poradie sa nepodarilo uložiť, skús to znova.")

			return
		}
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/videos")

		return
	}

	htmx.Toast(w, "Poradie uložené.")
	h.renderList(w, r)
}

// createFailed answers a failed create: toast for htmx (list is
// re-rendered unchanged), plain error otherwise.
func (h *VideosHandler) createFailed(w http.ResponseWriter, r *http.Request, msg string) {
	if !htmx.IsHtmxRequest(r) {
		http.Error(w, msg, http.StatusBadRequest)

		return
	}

	htmx.Toast(w, msg)
	h.renderList(w, r)
}

// updateFailed answers a failed update: the edit form with an inline
// error for htmx, plain error otherwise.
func (h *VideosHandler) updateFailed(w http.ResponseWriter, r *http.Request, video db.Video, msg string) {
	if !htmx.IsHtmxRequest(r) {
		http.Error(w, msg, http.StatusBadRequest)

		return
	}

	admintemplates.AdminVideoEditRow(video, msg).Render(r.Context(), w)
}

// reorderFailed answers a failed reorder: toast plus re-rendered list
// for htmx, plain error otherwise.
func (h *VideosHandler) reorderFailed(w http.ResponseWriter, r *http.Request, msg string) {
	if !htmx.IsHtmxRequest(r) {
		http.Error(w, msg, http.StatusBadRequest)

		return
	}

	htmx.Toast(w, msg)
	h.renderList(w, r)
}

// renderList renders the video table in playlist order.
func (h *VideosHandler) renderList(w http.ResponseWriter, r *http.Request) {
	rows, err := h.queries.ListVideosOrdered(r.Context())
	if err != nil {
		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	admintemplates.AdminVideoList(rows).Render(r.Context(), w)
}

// findVideo loads the video addressed by the {id} path value.
// It writes 404 when the id is invalid or the video does not exist.
func (h *VideosHandler) findVideo(w http.ResponseWriter, r *http.Request) (db.Video, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "Klip sa nenašiel.", http.StatusNotFound)

		return db.Video{}, false
	}

	video, err := h.queries.GetVideoByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Klip sa nenašiel.", http.StatusNotFound)

			return db.Video{}, false
		}

		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return db.Video{}, false
	}

	return video, true
}

// videoFormParams holds cleaned video form values.
type videoFormParams struct {
	YoutubeID     string
	Title         string
	PublishedDate string
	Duration      string
}

// parseVideoForm validates the video form fields.
// It returns the cleaned values or a Slovak error message.
func parseVideoForm(r *http.Request, _ int64) (videoFormParams, string) {
	youtubeID, ok := videos.ParseYouTubeID(r.FormValue("youtube_url"))
	if !ok {
		// Also accept a bare ID pasted into the URL field (handled above),
		// anything else is an invalid YouTube reference.
		return videoFormParams{}, "Zadaj platnú YouTube adresu alebo ID klipu."
	}

	title, valid := videos.ValidateTitle(r.FormValue("title"))
	if !valid {
		if strings.TrimSpace(r.FormValue("title")) == "" {
			return videoFormParams{}, "Zadaj názov klipu."
		}

		return videoFormParams{}, "Názov je príliš dlhý (max 200 znakov)."
	}

	date := strings.TrimSpace(r.FormValue("published_date"))
	if _, err := videos.ParseDate(date); err != nil {
		return videoFormParams{}, "Zadaj platný dátum publikovania."
	}

	duration, valid := videos.ValidateDuration(r.FormValue("duration"))
	if !valid {
		return videoFormParams{}, "Zadaj dĺžku v tvare M:SS (napr. 4:24)."
	}

	return videoFormParams{
		YoutubeID:     youtubeID,
		Title:         title,
		PublishedDate: date,
		Duration:      duration,
	}, ""
}

// isDuplicateVideo reports UNIQUE constraint violations on youtube_id.
func isDuplicateVideo(err error) bool {
	msg := strings.ToLower(err.Error())

	return strings.Contains(msg, "unique") && strings.Contains(msg, "videos")
}
