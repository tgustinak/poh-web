package admin

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/tgustinak/pohkapela.sk/internal/auth"
	"github.com/tgustinak/pohkapela.sk/internal/db"
	"github.com/tgustinak/pohkapela.sk/internal/htmx"
	"github.com/tgustinak/pohkapela.sk/internal/music"
	admintemplates "github.com/tgustinak/pohkapela.sk/web/templates/admin"
)

// musicUploadPrefix is the public path prefix for album covers,
// served by the /uploads/ route with immutable caching (file names carry
// a content hash, so every new cover is a fresh URL).
const musicUploadPrefix = "/uploads/"

// albumCoverName matches server-generated album cover file names
// (hudba-<sha256 prefix>.<ext>). Only matching files are ever deleted
// by the handler; /static/ seed files are never touched.
var albumCoverName = regexp.MustCompile(`^hudba-[0-9a-f]{16}\.(jpg|jpeg|png|webp)$`)

// MusicHandler serves admin album management (CRUD + reorder) and the
// Spotify URL singleton.
type MusicHandler struct {
	queries *db.Queries
	// coversDir is the directory holding uploaded album covers
	// (data/uploads by default). Tests override it so they never
	// touch real assets.
	coversDir string
}

// NewMusicHandler creates an admin music handler.
func NewMusicHandler(queries *db.Queries, uploadDir string) *MusicHandler {
	return &MusicHandler{queries: queries, coversDir: uploadDir}
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

	cover, err := readAlbumCover(r)
	if err != nil {
		h.createFailed(w, r, "Vyber obal (JPG, PNG alebo WebP) a skús to znova.")

		return
	}

	ext, ok := music.DetectCover(cover)
	if !ok {
		h.createFailed(w, r, "Obal musí byť JPG, PNG alebo WebP (max 10 MB).")

		return
	}

	name := "hudba-" + albumContentHash(cover) + ext

	if err := h.storeCover(name, cover); err != nil {
		h.createFailed(w, r, "Album sa nepodarilo uložiť, skús to znova.")

		return
	}

	if err := h.queries.ShiftAlbumPositions(r.Context()); err != nil {
		h.createFailed(w, r, "Album sa nepodarilo uložiť, skús to znova.")

		return
	}

	created, err := h.queries.CreateAlbum(r.Context(), db.CreateAlbumParams{
		Title:     params.Title,
		Year:      params.Year,
		Badge:     params.Badge,
		CoverPath: musicUploadPrefix + name,
		IsNew:     boolToInt(params.IsNew),
		Position:  1,
	})
	if err != nil {
		h.createFailed(w, r, "Album sa nepodarilo uložiť, skús to znova.")

		return
	}
	_ = created

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

	if cover, err := readAlbumCover(r); err == nil {
		ext, ok := music.DetectCover(cover)
		if !ok {
			h.updateFailed(w, r, album, "Obal musí byť JPG, PNG alebo WebP (max 10 MB).")

			return
		}

		name := "hudba-" + albumContentHash(cover) + ext

		if err := h.storeCover(name, cover); err != nil {
			h.updateFailed(w, r, album, "Obal sa nepodarilo uložiť.")

			return
		}

		withCover, err := h.queries.UpdateAlbumCover(r.Context(), db.UpdateAlbumCoverParams{
			CoverPath: musicUploadPrefix + name,
			ID:        album.ID,
		})
		if err != nil {
			h.updateFailed(w, r, album, "Obal sa nepodarilo uložiť.")

			return
		}

		// Best effort: drop the previous upload.
		h.removeCoverFile(album.CoverPath, musicUploadPrefix+name)
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
	h.removeCoverFile(album.CoverPath, "")

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

	rows, err := h.queries.ListAlbumsOrdered(r.Context())
	if err != nil {
		h.reorderFailed(w, r, "Poradie sa nepodarilo uložiť, skús to znova.")

		return
	}

	if len(rows) != len(ids) {
		h.reorderFailed(w, r, "Zoznam sa medzitým zmenil, obnov stránku a skús znova.")

		return
	}

	known := make(map[int64]struct{}, len(rows))
	for _, a := range rows {
		known[a.ID] = struct{}{}
	}

	for _, id := range ids {
		if _, ok := known[id]; !ok {
			h.reorderFailed(w, r, "Zoznam sa medzitým zmenil, obnov stránku a skús znova.")

			return
		}
	}

	for i, id := range ids {
		if err := h.queries.UpdateAlbumPosition(r.Context(), db.UpdateAlbumPositionParams{
			Position: int64(i + 1),
			ID:       id,
		}); err != nil {
			h.reorderFailed(w, r, "Poradie sa nepodarilo uložiť, skús to znova.")

			return
		}
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

// storeCover writes an uploaded cover to the uploads directory.
func (h *MusicHandler) storeCover(name string, data []byte) error {
	if err := os.MkdirAll(h.coversDir, 0o755); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(h.coversDir, name), data, 0o644)
}

// removeCoverFile deletes a previous cover file, keeping the one at
// keepPath. Only server-generated names inside the uploads directory
// qualify; /static/ seed paths are never touched.
func (h *MusicHandler) removeCoverFile(urlPath, keepPath string) {
	if urlPath == "" || urlPath == keepPath {
		return
	}

	name := strings.TrimPrefix(urlPath, musicUploadPrefix)
	if name == urlPath || strings.Contains(name, "/") || !albumCoverName.MatchString(name) {
		return
	}

	_ = os.Remove(filepath.Join(h.coversDir, name))
}

// albumContentHash returns the first 16 hex chars of the data sha256,
// used to build content-addressed cover file names.
func albumContentHash(data []byte) string {
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])[:16]
}

// boolToInt maps a checkbox to the 0/1 storage form.
func boolToInt(b bool) int64 {
	if b {
		return 1
	}

	return 0
}

// readAlbumCover reads the optional "cover" multipart file field.
// It returns an error when the field is missing or empty, so callers
// can tell "no cover uploaded" (create: error, update: keep existing).
func readAlbumCover(r *http.Request) ([]byte, error) {
	file, _, err := r.FormFile("cover")
	if err != nil {
		return nil, err
	}

	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, music.MaxCoverBytes+1))
	if err != nil || len(data) == 0 {
		return nil, errors.New("empty cover")
	}

	return data, nil
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
