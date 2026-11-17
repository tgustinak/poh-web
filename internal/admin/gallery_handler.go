package admin

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/tgustinak/pohkapela.sk/internal/auth"
	"github.com/tgustinak/pohkapela.sk/internal/db"
	"github.com/tgustinak/pohkapela.sk/internal/gallery"
	"github.com/tgustinak/pohkapela.sk/internal/htmx"
	admintemplates "github.com/tgustinak/pohkapela.sk/web/templates/admin"
)

// galleryUploadPrefix is the public path prefix for gallery photos,
// served by the /uploads/ route with immutable caching (file names carry
// a content hash, so every new file is a fresh URL).
const galleryUploadPrefix = "/uploads/"

// galleryFileName matches server-generated gallery upload names
// (galeria-foto-<sha256 prefix>.<ext> / galeria-nahlad-<sha256 prefix>.<ext>).
// Only matching files are ever deleted by the handler; /static/ seed files
// are never touched.
var galleryFileName = regexp.MustCompile(`^galeria-(foto|nahlad)-[0-9a-f]{16}\.(jpg|jpeg|png|webp)$`)

// GalleryHandler serves admin management of the public gallery:
// one ordered list mixing uploaded photos and YouTube videos.
type GalleryHandler struct {
	queries *db.Queries
	// filesDir is the directory holding uploaded gallery photos
	// (data/uploads by default). Tests override it so they never
	// touch real assets.
	filesDir string
}

// NewGalleryHandler creates an admin gallery handler.
func NewGalleryHandler(queries *db.Queries, uploadDir string) *GalleryHandler {
	return &GalleryHandler{queries: queries, filesDir: uploadDir}
}

// ListPage renders the gallery management page in shared display order.
func (h *GalleryHandler) ListPage(w http.ResponseWriter, r *http.Request) {
	csrf, _ := auth.CsrfFromRequest(r)

	items, err := h.queries.ListGalleryItemsOrdered(r.Context())
	if err != nil {
		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	admintemplates.AdminGallery(items, csrf).Render(r.Context(), w)
}

// CreatePhoto adds an uploaded photo to the gallery. The browser sends the
// 4:3 crop chosen in the admin cropper; the server then normalizes every
// upload (center-crop, max 1600 px, JPEG re-encode) and derives a 640 px
// preview, so camera originals never reach visitors. New items go first in
// the shared order. htmx requests get the refreshed grid plus a toast;
// plain form posts redirect back to the list.
func (h *GalleryHandler) CreatePhoto(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2*gallery.MaxPhotoBytes+(1<<20))

	if err := r.ParseMultipartForm(2 * gallery.MaxPhotoBytes); err != nil {
		h.createPhotoFailed(w, r, "Fotka je príliš veľká (max 10 MB).")

		return
	}

	alt, ok := gallery.ValidateAlt(r.FormValue("alt"))
	if !ok {
		h.createPhotoFailed(w, r, "Popis je príliš dlhý (max 200 znakov).")

		return
	}

	data, err := readUploadFile(r, "photo", gallery.MaxPhotoBytes)
	if err != nil {
		h.createPhotoFailed(w, r, "Vyber fotku (JPG, PNG alebo WebP) a skús to znova.")

		return
	}

	mainData, previewData, ok := gallery.ProcessPhoto(data)
	if !ok {
		h.createPhotoFailed(w, r, "Fotku sa nepodarilo spracovať. Podporované sú JPG, PNG a WebP (max 10 MB).")

		return
	}

	fileName := "galeria-foto-" + contentHash(mainData) + ".jpg"
	previewName := "galeria-nahlad-" + contentHash(previewData) + ".jpg"
	if bytes.Equal(previewData, mainData) {
		previewName = fileName
	}

	if err := h.storeFile(fileName, mainData); err != nil {
		h.createPhotoFailed(w, r, "Fotku sa nepodarilo uložiť.")

		return
	}

	if previewName != fileName {
		if err := h.storeFile(previewName, previewData); err != nil {
			_ = os.Remove(filepath.Join(h.filesDir, fileName))
			h.createPhotoFailed(w, r, "Fotku sa nepodarilo uložiť.")

			return
		}
	}

	if err := h.queries.ShiftGalleryItemPositions(r.Context()); err != nil {
		h.createPhotoFailed(w, r, "Fotku sa nepodarilo uložiť.")

		return
	}

	if _, err := h.queries.CreateGalleryItem(r.Context(), db.CreateGalleryItemParams{
		Kind:        gallery.KindPhoto,
		FilePath:    galleryUploadPrefix + fileName,
		PreviewPath: galleryUploadPrefix + previewName,
		Alt:         alt,
		Position:    1,
	}); err != nil {
		h.createPhotoFailed(w, r, "Fotku sa nepodarilo uložiť.")

		return
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/gallery")

		return
	}

	htmx.Trigger(w, htmx.Events{
		ShowToast: "Fotka pridaná.",
		FormReset: "gallery-photo-form",
	})
	h.renderList(w, r)
}

// createPhotoFailed answers a failed photo create: toast for htmx
// (grid is re-rendered unchanged), plain error otherwise.
func (h *GalleryHandler) createPhotoFailed(w http.ResponseWriter, r *http.Request, msg string) {
	if !htmx.IsHtmxRequest(r) {
		http.Error(w, msg, http.StatusBadRequest)

		return
	}

	htmx.Toast(w, msg)
	h.renderList(w, r)
}

// CreateVideo adds a YouTube video to the gallery. New items go first
// in the shared order. htmx requests get the refreshed grid plus a toast;
// plain form posts redirect back to the list.
func (h *GalleryHandler) CreateVideo(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.createVideoFailed(w, r, "Nepodarilo sa spracovať formulár, skús to znova.")

		return
	}

	youtubeID, ok := gallery.YouTubeID(r.FormValue("youtube_url"))
	if !ok {
		h.createVideoFailed(w, r, "Zadaj platnú YouTube adresu alebo ID videa.")

		return
	}

	title, ok := gallery.ValidateTitle(r.FormValue("title"))
	if !ok {
		if strings.TrimSpace(r.FormValue("title")) == "" {
			h.createVideoFailed(w, r, "Zadaj názov videa.")

			return
		}

		h.createVideoFailed(w, r, "Názov je príliš dlhý (max 200 znakov).")

		return
	}

	if err := h.queries.ShiftGalleryItemPositions(r.Context()); err != nil {
		h.createVideoFailed(w, r, "Video sa nepodarilo uložiť, skús to znova.")

		return
	}

	if _, err := h.queries.CreateGalleryItem(r.Context(), db.CreateGalleryItemParams{
		Kind:      gallery.KindVideo,
		YoutubeID: youtubeID,
		Title:     title,
		Position:  1,
	}); err != nil {
		h.createVideoFailed(w, r, "Video sa nepodarilo uložiť, skús to znova.")

		return
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/gallery")

		return
	}

	htmx.Trigger(w, htmx.Events{
		ShowToast: "Video pridané.",
		FormReset: "gallery-video-form",
	})
	h.renderList(w, r)
}

// createVideoFailed answers a failed video create: toast for htmx
// (grid is re-rendered unchanged), plain error otherwise.
func (h *GalleryHandler) createVideoFailed(w http.ResponseWriter, r *http.Request, msg string) {
	if !htmx.IsHtmxRequest(r) {
		http.Error(w, msg, http.StatusBadRequest)

		return
	}

	htmx.Toast(w, msg)
	h.renderList(w, r)
}

// Row renders a single gallery card (also the cancel target for
// inline editing).
func (h *GalleryHandler) Row(w http.ResponseWriter, r *http.Request) {
	item, ok := h.findItem(w, r)
	if !ok {
		return
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/gallery")

		return
	}

	admintemplates.AdminGalleryCard(item, "").Render(r.Context(), w)
}

// EditForm renders the inline edit card for an item.
func (h *GalleryHandler) EditForm(w http.ResponseWriter, r *http.Request) {
	item, ok := h.findItem(w, r)
	if !ok {
		return
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/gallery")

		return
	}

	admintemplates.AdminGalleryEditCard(item, "").Render(r.Context(), w)
}

// Update saves an edited gallery item. Photos update alt text; videos
// update the YouTube reference and title. On success it returns the
// updated card plus a toast; on validation error the edit card.
func (h *GalleryHandler) Update(w http.ResponseWriter, r *http.Request) {
	item, ok := h.findItem(w, r)
	if !ok {
		return
	}

	if err := r.ParseForm(); err != nil {
		h.updateFailed(w, r, item, "Nepodarilo sa spracovať formulár, skús to znova.")

		return
	}

	if item.Kind == gallery.KindVideo {
		youtubeID, valid := gallery.YouTubeID(r.FormValue("youtube_url"))
		if !valid {
			h.updateFailed(w, r, item, "Zadaj platnú YouTube adresu alebo ID videa.")

			return
		}

		title, valid := gallery.ValidateTitle(r.FormValue("title"))
		if !valid {
			if strings.TrimSpace(r.FormValue("title")) == "" {
				h.updateFailed(w, r, item, "Zadaj názov videa.")

				return
			}

			h.updateFailed(w, r, item, "Názov je príliš dlhý (max 200 znakov).")

			return
		}

		updated, err := h.queries.UpdateGalleryVideo(r.Context(), db.UpdateGalleryVideoParams{
			YoutubeID: youtubeID,
			Title:     title,
			ID:        item.ID,
		})
		if err != nil {
			h.updateFailed(w, r, item, "Video sa nepodarilo uložiť, skús to znova.")

			return
		}

		h.respondUpdated(w, r, updated, "Video uložené.")

		return
	}

	alt, valid := gallery.ValidateAlt(r.FormValue("alt"))
	if !valid {
		h.updateFailed(w, r, item, "Popis je príliš dlhý (max 200 znakov).")

		return
	}

	updated, err := h.queries.UpdateGalleryPhotoAlt(r.Context(), db.UpdateGalleryPhotoAltParams{
		Alt: alt,
		ID:  item.ID,
	})
	if err != nil {
		h.updateFailed(w, r, item, "Popis sa nepodarilo uložiť.")

		return
	}

	h.respondUpdated(w, r, updated, "Popis uložený.")
}

// updateFailed answers a failed update: the edit card with an inline
// error for htmx, plain error otherwise.
func (h *GalleryHandler) updateFailed(w http.ResponseWriter, r *http.Request, item db.GalleryItem, msg string) {
	if !htmx.IsHtmxRequest(r) {
		http.Error(w, msg, http.StatusBadRequest)

		return
	}

	if item.Kind == gallery.KindVideo {
		item.YoutubeID = strings.TrimSpace(r.FormValue("youtube_url"))
		item.Title = strings.TrimSpace(r.FormValue("title"))
	} else {
		item.Alt = strings.TrimSpace(r.FormValue("alt"))
	}

	admintemplates.AdminGalleryEditCard(item, msg).Render(r.Context(), w)
}

// respondUpdated renders the saved card for htmx or redirects for a
// plain form post.
func (h *GalleryHandler) respondUpdated(w http.ResponseWriter, r *http.Request, item db.GalleryItem, toast string) {
	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/gallery")

		return
	}

	htmx.Toast(w, toast)
	admintemplates.AdminGalleryCard(item, "").Render(r.Context(), w)
}

// Delete removes a gallery item. Photo uploads are cleaned up best effort;
// the htmx swap deletes the card client-side.
func (h *GalleryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	item, ok := h.findItem(w, r)
	if !ok {
		return
	}

	if err := h.queries.DeleteGalleryItem(r.Context(), item.ID); err != nil {
		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	if item.Kind == gallery.KindPhoto {
		h.removeUnreferencedUpload(r.Context(), item.FilePath, item.PreviewPath)
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/gallery")

		return
	}

	toast := "Položka zmazaná."
	if item.Kind == gallery.KindVideo {
		toast = "Video zmazané."
	} else {
		toast = "Fotka zmazaná."
	}

	htmx.Toast(w, toast)
}

// Reorder saves the shared order sent by drag & drop. It expects repeated
// "id" fields in the new order (position 1 = first). Unknown or duplicate
// ids are rejected; missing ids fail the request atomically.
func (h *GalleryHandler) Reorder(w http.ResponseWriter, r *http.Request) {
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

	rows, err := h.queries.ListGalleryItemsOrdered(r.Context())
	if err != nil {
		h.reorderFailed(w, r, "Poradie sa nepodarilo uložiť, skús to znova.")

		return
	}

	if len(rows) != len(ids) {
		h.reorderFailed(w, r, "Zoznam sa medzitým zmenil, obnov stránku a skús znova.")

		return
	}

	known := make(map[int64]struct{}, len(rows))
	for _, item := range rows {
		known[item.ID] = struct{}{}
	}

	for _, id := range ids {
		if _, ok := known[id]; !ok {
			h.reorderFailed(w, r, "Zoznam sa medzitým zmenil, obnov stránku a skús znova.")

			return
		}
	}

	for i, id := range ids {
		if err := h.queries.UpdateGalleryItemPosition(r.Context(), db.UpdateGalleryItemPositionParams{
			Position: int64(i + 1),
			ID:       id,
		}); err != nil {
			h.reorderFailed(w, r, "Poradie sa nepodarilo uložiť, skús to znova.")

			return
		}
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/gallery")

		return
	}

	htmx.Toast(w, "Poradie uložené.")
	h.renderList(w, r)
}

// reorderFailed answers a failed reorder: toast plus re-rendered grid
// for htmx, plain error otherwise.
func (h *GalleryHandler) reorderFailed(w http.ResponseWriter, r *http.Request, msg string) {
	if !htmx.IsHtmxRequest(r) {
		http.Error(w, msg, http.StatusBadRequest)

		return
	}

	htmx.Toast(w, msg)
	h.renderList(w, r)
}

// renderList renders the mixed gallery grid in shared display order.
func (h *GalleryHandler) renderList(w http.ResponseWriter, r *http.Request) {
	items, err := h.queries.ListGalleryItemsOrdered(r.Context())
	if err != nil {
		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	admintemplates.AdminGalleryList(items).Render(r.Context(), w)
}

// findItem loads the gallery item addressed by the {id} path value.
// It writes 404 when the id is invalid or the item does not exist.
func (h *GalleryHandler) findItem(w http.ResponseWriter, r *http.Request) (db.GalleryItem, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "Položka sa nenašla.", http.StatusNotFound)

		return db.GalleryItem{}, false
	}

	item, err := h.queries.GetGalleryItemByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Položka sa nenašla.", http.StatusNotFound)

			return db.GalleryItem{}, false
		}

		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return db.GalleryItem{}, false
	}

	return item, true
}

// storeFile writes uploaded bytes to the gallery uploads directory.
func (h *GalleryHandler) storeFile(name string, data []byte) error {
	if err := os.MkdirAll(h.filesDir, 0o755); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(h.filesDir, name), data, 0o644)
}

// removeUnreferencedUpload deletes photo files that no remaining gallery
// item references. Upload names are content-addressed, so two identical
// photos share one file; deleting only one of them must keep the file.
// If the list cannot be loaded, files are left in place (harmless orphans)
// rather than breaking a surviving item.
func (h *GalleryHandler) removeUnreferencedUpload(ctx context.Context, paths ...string) {
	rows, err := h.queries.ListGalleryItemsOrdered(ctx)
	if err != nil {
		return
	}

	inUse := make(map[string]struct{}, len(rows)*2)
	for _, row := range rows {
		if row.FilePath != "" {
			inUse[row.FilePath] = struct{}{}
		}

		if row.PreviewPath != "" {
			inUse[row.PreviewPath] = struct{}{}
		}
	}

	for _, path := range paths {
		if _, ok := inUse[path]; ok {
			continue
		}

		h.removeUploadFile(path, "")
	}
}

// removeUploadFile deletes a previous gallery upload, keeping the file at
// keepPath. Only server-generated names inside the uploads directory
// qualify; /static/ seed paths and unknown names are never touched.
func (h *GalleryHandler) removeUploadFile(urlPath, keepPath string) {
	if urlPath == "" || urlPath == keepPath {
		return
	}

	name := strings.TrimPrefix(urlPath, galleryUploadPrefix)
	if name == urlPath || strings.Contains(name, "/") || !galleryFileName.MatchString(name) {
		return
	}

	_ = os.Remove(filepath.Join(h.filesDir, name))
}
