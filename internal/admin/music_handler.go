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
	"github.com/tgustinak/pohkapela.sk/internal/music"
	"github.com/tgustinak/pohkapela.sk/internal/ordering"
	"github.com/tgustinak/pohkapela.sk/internal/uploads"
	admintemplates "github.com/tgustinak/pohkapela.sk/web/templates/admin"
)

// MusicHandler serves admin album management (CRUD + reorder) and the
// Spotify URL singleton.
type MusicHandler struct {
	queries *db.Queries
	// uploads stores uploaded album covers (data/uploads by default).
	// Tests point it at a temp dir so they never touch real assets.
	uploads *uploads.Store
}

// NewMusicHandler creates an admin music handler.
func NewMusicHandler(queries *db.Queries, uploadDir string) *MusicHandler {
	return &MusicHandler{queries: queries, uploads: uploads.NewStore(uploadDir)}
}

// ListPage renders the album management page in display order.
func (h *MusicHandler) ListPage(w http.ResponseWriter, r *http.Request) {
	csrf, _ := auth.CsrfFromRequest(r)

	rows, err := h.queries.ListAlbumsOrdered(r.Context())
	if err != nil {
		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	admintemplates.AdminMusic(rows, csrf).Render(r.Context(), w)
}

// Create adds an album with an uploaded cover. New albums go first in
// display order. htmx requests get the refreshed list plus a toast;
// plain form posts redirect back to the list.
func (h *MusicHandler) Create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, music.MaxCoverBytes+(1<<20))

	if err := r.ParseMultipartForm(music.MaxCoverBytes); err != nil {
		h.createFailed(w, r, "Obal je príliš veľký (max 10 MB).")

		return
	}

	params, errMsg := parseAlbumForm(r)
	if errMsg != "" {
		h.createFailed(w, r, errMsg)

		return
	}

	cover, err := uploads.ReadFormFile(r, "cover", music.MaxCoverBytes)
	if err != nil {
		h.createFailed(w, r, "Vyber obal (JPG, PNG alebo WebP) a skús to znova.")

		return
	}

	ext, ok := music.DetectCover(cover)
	if !ok {
		h.createFailed(w, r, "Obal musí byť JPG, PNG alebo WebP (max 10 MB).")

		return
	}

	coverPath, err := h.uploads.Save("hudba", cover, ext)
	if err != nil {
		h.createFailed(w, r, "Album sa nepodarilo uložiť, skús to znova.")

		return
	}

	err = ordering.InsertFirst(r.Context(), h.queries, ordering.Albums, func(q *db.Queries) error {
		_, err := q.CreateAlbum(r.Context(), db.CreateAlbumParams{
			Title:     params.Title,
			Year:      params.Year,
			Badge:     params.Badge,
			CoverPath: coverPath,
			IsNew:     boolToInt(params.IsNew),
			Position:  1,
		})

		return err
	})
	if err != nil {
		h.createFailed(w, r, "Album sa nepodarilo uložiť, skús to znova.")

		return
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/music")

		return
	}

	htmx.Trigger(w, htmx.Events{
		ShowToast: "Album pridaný.",
		FormReset: "album-create-form",
	})
	h.renderList(w, r)
}

// Row renders a single album table row (also the cancel target
// for inline editing).
func (h *MusicHandler) Row(w http.ResponseWriter, r *http.Request) {
	album, ok := h.findAlbum(w, r)
	if !ok {
		return
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/music")

		return
	}

	admintemplates.AdminAlbumRow(album, int(album.Position)).Render(r.Context(), w)
}

// EditForm renders the inline edit row for an album.
func (h *MusicHandler) EditForm(w http.ResponseWriter, r *http.Request) {
	album, ok := h.findAlbum(w, r)
	if !ok {
		return
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/music")

		return
	}

	admintemplates.AdminAlbumEditRow(album, "").Render(r.Context(), w)
}

// Update saves an edited album. An uploaded cover replaces the previous
// one; without a cover only text fields change. On success it returns
// the updated row plus a toast; on validation error the edit form.
func (h *MusicHandler) Update(w http.ResponseWriter, r *http.Request) {
	album, ok := h.findAlbum(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, music.MaxCoverBytes+(1<<20))

	if err := r.ParseMultipartForm(music.MaxCoverBytes); err != nil {
		h.updateFailed(w, r, album, "Obal je príliš veľký (max 10 MB).")

		return
	}

	params, errMsg := parseAlbumForm(r)
	if errMsg != "" {
		h.updateFailed(w, r, album, errMsg)

		return
	}

	updated, err := h.queries.UpdateAlbum(r.Context(), db.UpdateAlbumParams{
		Title: params.Title,
		Year:  params.Year,
		Badge: params.Badge,
		IsNew: boolToInt(params.IsNew),
		ID:    album.ID,
	})
	if err != nil {
		h.updateFailed(w, r, album, "Album sa nepodarilo uložiť, skús to znova.")

		return
	}

	if cover, err := uploads.ReadFormFile(r, "cover", music.MaxCoverBytes); err == nil {
		ext, ok := music.DetectCover(cover)
		if !ok {
			h.updateFailed(w, r, album, "Obal musí byť JPG, PNG alebo WebP (max 10 MB).")

			return
		}

		coverPath, err := h.uploads.Save("hudba", cover, ext)
		if err != nil {
			h.updateFailed(w, r, album, "Obal sa nepodarilo uložiť.")

			return
		}

		withCover, err := h.queries.UpdateAlbumCover(r.Context(), db.UpdateAlbumCoverParams{
			CoverPath: coverPath,
			ID:        album.ID,
		})
		if err != nil {
			h.updateFailed(w, r, album, "Obal sa nepodarilo uložiť.")

			return
		}

		// Best effort: drop the previous upload.
		h.uploads.Remove(album.CoverPath, coverPath)
		updated = withCover
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/music")

		return
	}

	htmx.Toast(w, "Album uložený.")
	admintemplates.AdminAlbumRow(updated, int(updated.Position)).Render(r.Context(), w)
}

// Delete removes an album and its uploaded cover. The htmx swap deletes
// the row client-side.
func (h *MusicHandler) Delete(w http.ResponseWriter, r *http.Request) {
	album, ok := h.findAlbum(w, r)
	if !ok {
		return
	}

	if err := h.queries.DeleteAlbum(r.Context(), album.ID); err != nil {
		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	// Best effort: the cover file is just a cache of the DB state.
	h.uploads.Remove(album.CoverPath, "")

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/music")

		return
	}

	htmx.Toast(w, "Album zmazaný.")
}

// Reorder saves the album order sent by drag & drop.
// It expects repeated "id" fields in the new order (position 1 = first).
func (h *MusicHandler) Reorder(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.reorderFailed(w, r, "Poradie sa nepodarilo uložiť, skús to znova.")

		return
	}

	err := ordering.Reorder(r.Context(), h.queries, ordering.Albums, r.Form["id"])
	if errors.Is(err, ordering.ErrStale) {
		h.reorderFailed(w, r, "Zoznam sa medzitým zmenil, obnov stránku a skús znova.")

		return
	}

	if err != nil {
		h.reorderFailed(w, r, "Poradie sa nepodarilo uložiť, skús to znova.")

		return
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/music")

		return
	}

	htmx.Toast(w, "Poradie uložené.")
	h.renderList(w, r)
}

// UpdateSettings saves the Spotify artist URL. htmx requests get the
// re-rendered form plus a toast; plain form posts redirect back.
// createFailed answers a failed create: toast for htmx (list is
// re-rendered unchanged), plain error otherwise.
func (h *MusicHandler) createFailed(w http.ResponseWriter, r *http.Request, msg string) {
	if !htmx.IsHtmxRequest(r) {
		http.Error(w, msg, http.StatusBadRequest)

		return
	}

	htmx.Toast(w, msg)
	h.renderList(w, r)
}

// updateFailed answers a failed update: the edit form with an inline
// error for htmx, plain error otherwise.
func (h *MusicHandler) updateFailed(w http.ResponseWriter, r *http.Request, album db.Album, msg string) {
	if !htmx.IsHtmxRequest(r) {
		http.Error(w, msg, http.StatusBadRequest)

		return
	}

	admintemplates.AdminAlbumEditRow(album, msg).Render(r.Context(), w)
}

// reorderFailed answers a failed reorder: toast plus re-rendered list
// for htmx, plain error otherwise.
func (h *MusicHandler) reorderFailed(w http.ResponseWriter, r *http.Request, msg string) {
	if !htmx.IsHtmxRequest(r) {
		http.Error(w, msg, http.StatusBadRequest)

		return
	}

	htmx.Toast(w, msg)
	h.renderList(w, r)
}

// renderList renders the album table in display order.
func (h *MusicHandler) renderList(w http.ResponseWriter, r *http.Request) {
	rows, err := h.queries.ListAlbumsOrdered(r.Context())
	if err != nil {
		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	admintemplates.AdminAlbumList(rows).Render(r.Context(), w)
}

// findAlbum loads the album addressed by the {id} path value.
// It writes 404 when the id is invalid or the album does not exist.
func (h *MusicHandler) findAlbum(w http.ResponseWriter, r *http.Request) (db.Album, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "Album sa nenašiel.", http.StatusNotFound)

		return db.Album{}, false
	}

	album, err := h.queries.GetAlbumByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Album sa nenašiel.", http.StatusNotFound)

			return db.Album{}, false
		}

		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return db.Album{}, false
	}

	return album, true
}

// boolToInt maps a checkbox to the 0/1 storage form.
func boolToInt(b bool) int64 {
	if b {
		return 1
	}

	return 0
}

// albumFormParams holds cleaned album form values.
type albumFormParams struct {
	Title string
	Year  int64
	Badge string
	IsNew bool
}

// parseAlbumForm validates the album text fields (title, year, badge,
// is_new checkbox). Cover validation lives with the caller because
// create requires a cover while update treats it as optional.
func parseAlbumForm(r *http.Request) (albumFormParams, string) {
	title, ok := music.ValidateTitle(r.FormValue("title"))
	if !ok {
		if strings.TrimSpace(r.FormValue("title")) == "" {
			return albumFormParams{}, "Zadaj názov albumu."
		}

		return albumFormParams{}, "Názov je príliš dlhý (max 100 znakov)."
	}

	year, ok := music.ValidateYear(r.FormValue("year"))
	if !ok {
		if strings.TrimSpace(r.FormValue("year")) == "" {
			return albumFormParams{}, "Zadaj rok vydania."
		}

		return albumFormParams{}, "Rok musí byť 4-ciferné číslo (1900–2100)."
	}

	badge, ok := music.ValidateBadge(r.FormValue("badge"))
	if !ok {
		return albumFormParams{}, "Štítok je príliš dlhý (max 50 znakov)."
	}

	return albumFormParams{
		Title: title,
		Year:  year,
		Badge: badge,
		IsNew: r.FormValue("is_new") == "1",
	}, ""
}
