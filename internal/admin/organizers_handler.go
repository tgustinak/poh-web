package admin

import (
	"bytes"
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
	"github.com/tgustinak/pohkapela.sk/internal/organizers"
	admintemplates "github.com/tgustinak/pohkapela.sk/web/templates/admin"
)

// organizerUploadPrefix is the public path prefix for organizer files,
// served by the /uploads/ route with immutable caching (file names carry
// a content hash, so every new file is a fresh URL).
const organizerUploadPrefix = "/uploads/"

// organizerFileName matches server-generated organizer file names
// (organizatori-<kind>-<sha256 prefix>.<ext>). Only matching files are
// ever deleted by the handler; /static/ seed files are never touched.
var organizerFileName = regexp.MustCompile(`^organizatori-(rider|input-list|logo|foto|nahlad)-[0-9a-f]{16}\.(pdf|jpg|jpeg|png|webp)$`)

// OrganizersHandler serves admin management of the "Pre organizátorov"
// page: replacing the 3 fixed documents and CRUD of promo photos.
type OrganizersHandler struct {
	queries *db.Queries
	// filesDir is the directory holding uploaded organizer files
	// (data/uploads by default). Tests override it so they never
	// touch real assets.
	filesDir string
}

// NewOrganizersHandler creates an admin organizers handler.
func NewOrganizersHandler(queries *db.Queries, uploadDir string) *OrganizersHandler {
	return &OrganizersHandler{queries: queries, filesDir: uploadDir}
}

// ListPage renders the organizers management page.
func (h *OrganizersHandler) ListPage(w http.ResponseWriter, r *http.Request) {
	csrf, _ := auth.CsrfFromRequest(r)

	docs, err := h.queries.ListOrganizerDocumentsOrdered(r.Context())
	if err != nil {
		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	photos, err := h.queries.ListOrganizerPhotosOrdered(r.Context())
	if err != nil {
		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	admintemplates.AdminOrganizers(docs, photos, csrf).Render(r.Context(), w)
}

// ReplaceDocument swaps the file behind one fixed document slot.
// htmx requests get the re-rendered row plus a toast; plain form posts
// redirect back to the list.
func (h *OrganizersHandler) ReplaceDocument(w http.ResponseWriter, r *http.Request) {
	slot := r.PathValue("slot")
	if !organizers.IsSlot(slot) {
		http.Error(w, "Dokument sa nenašiel.", http.StatusNotFound)

		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, organizers.MaxDocumentBytes+(1<<20))

	if err := r.ParseMultipartForm(organizers.MaxDocumentBytes); err != nil {
		h.replaceFailed(w, r, slot, "Súbor je príliš veľký (max 15 MB).")

		return
	}

	data, err := readUploadFile(r, "file", organizers.MaxDocumentBytes)
	if err != nil {
		h.replaceFailed(w, r, slot, "Vyber PDF súbor a skús to znova.")

		return
	}

	if _, ok := organizers.DetectDocument(data); !ok {
		h.replaceFailed(w, r, slot, "Dokument musí byť PDF.")

		return
	}

	name := "organizatori-" + slot + "-" + contentHash(data) + ".pdf"
	urlPath := organizerUploadPrefix + name

	if err := h.storeFile(name, data); err != nil {
		h.replaceFailed(w, r, slot, "Súbor sa nepodarilo uložiť.")

		return
	}

	old, err := h.queries.GetOrganizerDocumentBySlot(r.Context(), slot)
	if err != nil {
		h.replaceFailed(w, r, slot, "Súbor sa nepodarilo uložiť.")

		return
	}

	updated, err := h.queries.UpdateOrganizerDocumentFile(r.Context(), db.UpdateOrganizerDocumentFileParams{
		FilePath:  urlPath,
		SizeBytes: int64(len(data)),
		Slot:      slot,
	})
	if err != nil {
		h.replaceFailed(w, r, slot, "Súbor sa nepodarilo uložiť.")

		return
	}

	// Best effort: drop the previous upload so files do not accumulate.
	h.removeUploadFile(old.FilePath, urlPath)

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/organizers")

		return
	}

	htmx.Toast(w, "Dokument vymenený.")
	admintemplates.AdminDocumentRow(updated, "").Render(r.Context(), w)
}

// replaceFailed answers a failed document replacement: the row with an
// inline error for htmx, plain error otherwise.
func (h *OrganizersHandler) replaceFailed(w http.ResponseWriter, r *http.Request, slot, msg string) {
	if !htmx.IsHtmxRequest(r) {
		http.Error(w, msg, http.StatusBadRequest)

		return
	}

	doc, err := h.queries.GetOrganizerDocumentBySlot(r.Context(), slot)
	if err != nil {
		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	admintemplates.AdminDocumentRow(doc, msg).Render(r.Context(), w)
}

// CreatePhoto adds a promo photo (full file + optional lightweight
// preview + alt text). New photos go last in display order.
// htmx requests get the refreshed grid plus a toast; plain form posts
// redirect back to the list.
func (h *OrganizersHandler) CreatePhoto(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2*organizers.MaxPhotoBytes+(1<<20))

	if err := r.ParseMultipartForm(2 * organizers.MaxPhotoBytes); err != nil {
		h.createPhotoFailed(w, r, "Fotka je príliš veľká (max 10 MB).")

		return
	}

	alt, ok := organizers.ValidateAlt(r.FormValue("alt"))
	if !ok {
		h.createPhotoFailed(w, r, "Popis je príliš dlhý (max 200 znakov).")

		return
	}

	data, err := readUploadFile(r, "photo", organizers.MaxPhotoBytes)
	if err != nil {
		h.createPhotoFailed(w, r, "Vyber fotku (JPG, PNG alebo WebP) a skús to znova.")

		return
	}

	ext, ok := organizers.DetectPhoto(data)
	if !ok {
		h.createPhotoFailed(w, r, "Podporované sú len JPG, PNG a WebP (max 10 MB).")

		return
	}

	previewData, previewExt := data, ext
	separatePreview := false
	if raw, err := readUploadFile(r, "preview", organizers.MaxPhotoBytes); err == nil {
		pext, ok := organizers.DetectPhoto(raw)
		if !ok {
			h.createPhotoFailed(w, r, "Náhľad musí byť JPG, PNG alebo WebP (max 10 MB).")

			return
		}

		previewData, previewExt = raw, pext
		separatePreview = true
	}

	fileName := "organizatori-foto-" + contentHash(data) + ext
	previewName := fileName
	if separatePreview && !bytes.Equal(previewData, data) {
		previewName = "organizatori-nahlad-" + contentHash(previewData) + previewExt
	}

	if err := h.storeFile(fileName, data); err != nil {
		h.createPhotoFailed(w, r, "Fotku sa nepodarilo uložiť.")

		return
	}

	if previewName != fileName {
		if err := h.storeFile(previewName, previewData); err != nil {
			h.createPhotoFailed(w, r, "Fotku sa nepodarilo uložiť.")

			return
		}
	} else {
		previewName = fileName
	}

	lastPos, err := h.queries.LastOrganizerPhotoPosition(r.Context())
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		h.createPhotoFailed(w, r, "Fotku sa nepodarilo uložiť.")

		return
	}

	created, err := h.queries.CreateOrganizerPhoto(r.Context(), db.CreateOrganizerPhotoParams{
		FilePath:    organizerUploadPrefix + fileName,
		PreviewPath: organizerUploadPrefix + previewName,
		Alt:         alt,
		MetaLabel:   organizers.MetaLabel(ext, int64(len(data))),
		Position:    lastPos + 1,
	})
	if err != nil {
		h.createPhotoFailed(w, r, "Fotku sa nepodarilo uložiť.")

		return
	}
	_ = created

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/organizers")

		return
	}

	htmx.Trigger(w, htmx.Events{
		ShowToast: "Fotka pridaná.",
		FormReset: "organizer-photo-create-form",
	})
	h.renderPhotoGrid(w, r)
}

// createPhotoFailed answers a failed photo create: toast for htmx
// (grid is re-rendered unchanged), plain error otherwise.
func (h *OrganizersHandler) createPhotoFailed(w http.ResponseWriter, r *http.Request, msg string) {
	if !htmx.IsHtmxRequest(r) {
		http.Error(w, msg, http.StatusBadRequest)

		return
	}

	htmx.Toast(w, msg)
	h.renderPhotoGrid(w, r)
}

// PhotoRow renders a single photo card (also the cancel target
// for inline alt editing).
func (h *OrganizersHandler) PhotoRow(w http.ResponseWriter, r *http.Request) {
	photo, ok := h.findPhoto(w, r)
	if !ok {
		return
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/organizers")

		return
	}

	admintemplates.AdminPhotoCard(photo, "").Render(r.Context(), w)
}

// PhotoEditForm renders the inline alt edit card for a photo.
func (h *OrganizersHandler) PhotoEditForm(w http.ResponseWriter, r *http.Request) {
	photo, ok := h.findPhoto(w, r)
	if !ok {
		return
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/organizers")

		return
	}

	admintemplates.AdminPhotoEditCard(photo, "").Render(r.Context(), w)
}

// UpdatePhotoAlt saves the edited photo description. On success it
// returns the updated card plus a toast; on validation error the edit
// card with the message.
func (h *OrganizersHandler) UpdatePhotoAlt(w http.ResponseWriter, r *http.Request) {
	photo, ok := h.findPhoto(w, r)
	if !ok {
		return
	}

	if err := r.ParseForm(); err != nil {
		h.updateAltFailed(w, r, photo, "Nepodarilo sa spracovať formulár, skús to znova.")

		return
	}

	alt, ok := organizers.ValidateAlt(r.FormValue("alt"))
	if !ok {
		h.updateAltFailed(w, r, photo, "Popis je príliš dlhý (max 200 znakov).")

		return
	}

	updated, err := h.queries.UpdateOrganizerPhotoAlt(r.Context(), db.UpdateOrganizerPhotoAltParams{
		Alt: alt,
		ID:  photo.ID,
	})
	if err != nil {
		h.updateAltFailed(w, r, photo, "Popis sa nepodarilo uložiť.")

		return
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/organizers")

		return
	}

	htmx.Toast(w, "Popis uložený.")
	admintemplates.AdminPhotoCard(updated, "").Render(r.Context(), w)
}

// updateAltFailed answers a failed alt update: the edit card with an
// inline error for htmx, plain error otherwise.
func (h *OrganizersHandler) updateAltFailed(w http.ResponseWriter, r *http.Request, photo db.OrganizerPhoto, msg string) {
	if !htmx.IsHtmxRequest(r) {
		http.Error(w, msg, http.StatusBadRequest)

		return
	}

	photo.Alt = strings.TrimSpace(r.FormValue("alt"))

	admintemplates.AdminPhotoEditCard(photo, msg).Render(r.Context(), w)
}

// DeletePhoto removes a promo photo: the row plus its uploaded files.
// htmx swaps the card out client-side; plain posts redirect to the list.
func (h *OrganizersHandler) DeletePhoto(w http.ResponseWriter, r *http.Request) {
	photo, ok := h.findPhoto(w, r)
	if !ok {
		return
	}

	if err := h.queries.DeleteOrganizerPhoto(r.Context(), photo.ID); err != nil {
		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	// Best effort: uploaded files are just a cache of the DB state.
	h.removeUploadFile(photo.FilePath, "")
	h.removeUploadFile(photo.PreviewPath, photo.FilePath)

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/organizers")

		return
	}

	htmx.Toast(w, "Fotka zmazaná.")
}

// renderPhotoGrid renders the photo grid in display order.
func (h *OrganizersHandler) renderPhotoGrid(w http.ResponseWriter, r *http.Request) {
	photos, err := h.queries.ListOrganizerPhotosOrdered(r.Context())
	if err != nil {
		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	admintemplates.AdminPhotoGrid(photos).Render(r.Context(), w)
}

// findPhoto loads the photo addressed by the {id} path value.
// It writes 404 when the id is invalid or the photo does not exist.
func (h *OrganizersHandler) findPhoto(w http.ResponseWriter, r *http.Request) (db.OrganizerPhoto, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "Fotka sa nenašla.", http.StatusNotFound)

		return db.OrganizerPhoto{}, false
	}

	photo, err := h.queries.GetOrganizerPhotoByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Fotka sa nenašla.", http.StatusNotFound)

			return db.OrganizerPhoto{}, false
		}

		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return db.OrganizerPhoto{}, false
	}

	return photo, true
}

// storeFile writes uploaded bytes to the uploads directory.
func (h *OrganizersHandler) storeFile(name string, data []byte) error {
	if err := os.MkdirAll(h.filesDir, 0o755); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(h.filesDir, name), data, 0o644)
}

// removeUploadFile deletes a previous upload file, keeping the one at
// keepPath. Only server-generated names inside the uploads directory
// qualify; /static/ seed paths and unknown names are never touched.
func (h *OrganizersHandler) removeUploadFile(urlPath, keepPath string) {
	if urlPath == "" || urlPath == keepPath {
		return
	}

	name := strings.TrimPrefix(urlPath, organizerUploadPrefix)
	if name == urlPath || strings.Contains(name, "/") || !organizerFileName.MatchString(name) {
		return
	}

	_ = os.Remove(filepath.Join(h.filesDir, name))
}

// contentHash returns the first 16 hex chars of the data sha256,
// used to build content-addressed upload file names.
func contentHash(data []byte) string {
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])[:16]
}

// readUploadFile reads a single multipart file field, capped at limit+1
// bytes so oversize uploads are detected. It returns an error when the
// field is missing or empty.
func readUploadFile(r *http.Request, field string, limit int64) ([]byte, error) {
	file, _, err := r.FormFile(field)
	if err != nil {
		return nil, err
	}

	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || len(data) == 0 {
		return nil, errors.New("empty upload")
	}

	return data, nil
}
