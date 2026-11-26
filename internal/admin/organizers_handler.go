package admin

import (
	"bytes"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/tgustinak/poh-web/internal/auth"
	"github.com/tgustinak/poh-web/internal/db"
	"github.com/tgustinak/poh-web/internal/htmx"
	"github.com/tgustinak/poh-web/internal/organizers"
	"github.com/tgustinak/poh-web/internal/uploads"
	admintemplates "github.com/tgustinak/poh-web/web/templates/admin"
)

// OrganizersHandler serves admin management of the "Pre organizátorov"
// page: replacing the 3 fixed documents and CRUD of promo photos.
type OrganizersHandler struct {
	queries *db.Queries
	// uploads stores uploaded organizer files (data/uploads by default).
	// Tests point it at a temp dir so they never touch real assets.
	uploads *uploads.Store
}

// NewOrganizersHandler creates an admin organizers handler.
func NewOrganizersHandler(queries *db.Queries, uploadDir string) *OrganizersHandler {
	return &OrganizersHandler{queries: queries, uploads: uploads.NewStore(uploadDir)}
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

	data, err := uploads.ReadFormFile(r, "file", organizers.MaxDocumentBytes)
	if err != nil {
		h.replaceFailed(w, r, slot, "Vyber PDF súbor a skús to znova.")

		return
	}

	if _, ok := organizers.DetectDocument(data); !ok {
		h.replaceFailed(w, r, slot, "Dokument musí byť PDF.")

		return
	}

	urlPath, err := h.uploads.Save("organizatori-"+slot, data, ".pdf")
	if err != nil {
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
	h.uploads.Remove(old.FilePath, urlPath)

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

	data, err := uploads.ReadFormFile(r, "photo", organizers.MaxPhotoBytes)
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
	if raw, err := uploads.ReadFormFile(r, "preview", organizers.MaxPhotoBytes); err == nil {
		pext, ok := organizers.DetectPhoto(raw)
		if !ok {
			h.createPhotoFailed(w, r, "Náhľad musí byť JPG, PNG alebo WebP (max 10 MB).")

			return
		}

		previewData, previewExt = raw, pext
		separatePreview = true
	}

	filePath, err := h.uploads.Save("organizatori-foto", data, ext)
	if err != nil {
		h.createPhotoFailed(w, r, "Fotku sa nepodarilo uložiť.")

		return
	}

	previewPath := filePath
	if separatePreview && !bytes.Equal(previewData, data) {
		previewPath, err = h.uploads.Save("organizatori-nahlad", previewData, previewExt)
		if err != nil {
			h.createPhotoFailed(w, r, "Fotku sa nepodarilo uložiť.")

			return
		}
	}

	lastPos, err := h.queries.LastOrganizerPhotoPosition(r.Context())
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		h.createPhotoFailed(w, r, "Fotku sa nepodarilo uložiť.")

		return
	}

	created, err := h.queries.CreateOrganizerPhoto(r.Context(), db.CreateOrganizerPhotoParams{
		FilePath:    filePath,
		PreviewPath: previewPath,
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
	h.uploads.Remove(photo.FilePath, "")
	h.uploads.Remove(photo.PreviewPath, photo.FilePath)

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
