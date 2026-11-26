package admin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/tgustinak/poh-web/internal/auth"
	"github.com/tgustinak/poh-web/internal/contact"
	"github.com/tgustinak/poh-web/internal/db"
	"github.com/tgustinak/poh-web/internal/htmx"
	"github.com/tgustinak/poh-web/internal/uploads"
	admintemplates "github.com/tgustinak/poh-web/web/templates/admin"
)

// maxContactPhotoBytes caps the uploaded (already cropped) photo.
// The browser canvas exports ~800x800 webp, typically well below 1 MB.
const maxContactPhotoBytes = 8 << 20

// ContactHandler serves admin editing of the "Kontakt" section
// (singleton row plus manager photo).
type ContactHandler struct {
	queries *db.Queries
	// uploads stores the uploaded manager photo (data/uploads by default).
	// Tests point it at a temp dir so they never touch real assets.
	uploads *uploads.Store
}

// NewContactHandler creates an admin contact handler.
func NewContactHandler(queries *db.Queries, uploadDir string) *ContactHandler {
	return &ContactHandler{queries: queries, uploads: uploads.NewStore(uploadDir)}
}

// GetContact loads the contact row for display. A photo path whose file
// is missing on disk (fresh clone, wiped data/) is reported as empty,
// so templates render the silhouette placeholder instead of a broken image.
func (h *ContactHandler) GetContact(ctx context.Context) (contact.Contact, error) {
	row, err := h.queries.GetContact(ctx)
	if err != nil {
		return contact.Contact{}, err
	}

	c := contact.FromDB(row)
	if c.PhotoPath != "" && !h.uploads.Exists(c.PhotoPath) {
		c.PhotoPath = ""
	}

	return c, nil
}

// EditPage renders the contact edit page with the current values.
func (h *ContactHandler) EditPage(w http.ResponseWriter, r *http.Request) {
	csrf, _ := auth.CsrfFromRequest(r)

	data, err := h.GetContact(r.Context())
	if err != nil {
		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	admintemplates.AdminContact(data, csrf, "").Render(r.Context(), w)
}

// Update saves the contact text fields. htmx requests get the re-rendered
// form plus a toast; plain form posts redirect back to the edit page.
func (h *ContactHandler) Update(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.updateFailed(w, r, contactFromRequest(r), "Nepodarilo sa spracovať formulár, skús to znova.")

		return
	}

	params, errMsg := parseContactForm(r)
	if errMsg != "" {
		h.updateFailed(w, r, contactFromRequest(r), errMsg)

		return
	}

	updated, err := h.queries.UpdateContact(r.Context(), db.UpdateContactParams{
		Name:  params.Name,
		Role:  params.Role,
		Phone: params.Phone,
		Email: params.Email,
	})
	if err != nil {
		h.updateFailed(w, r, contactFromRequest(r), "Zmeny sa nepodarilo uložiť, skús to znova.")

		return
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/contact")

		return
	}

	htmx.Toast(w, "Zmeny uložené.")
	admintemplates.AdminContactForm(contact.FromDB(updated), "").Render(r.Context(), w)
}

// UploadPhoto saves the already-cropped manager photo (exported by the
// browser canvas as JPEG/PNG/WebP) under a content-hashed file name, so
// every new photo gets a fresh URL and browsers never serve a stale
// cached copy. The previous photo file is removed.
// It answers JSON so the cropper fetch() flow can update the preview
// without a full page reload.
func (h *ContactHandler) UploadPhoto(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxContactPhotoBytes+(1<<20))

	if err := r.ParseMultipartForm(maxContactPhotoBytes); err != nil {
		h.photoFailed(w, r, "Fotka je príliš veľká (max 8 MB).")

		return
	}

	file, _, err := r.FormFile("photo")
	if err != nil {
		h.photoFailed(w, r, "Vyber fotku a skús to znova.")

		return
	}

	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxContactPhotoBytes+1))
	if err != nil || len(data) == 0 {
		h.photoFailed(w, r, "Fotku sa nepodarilo načítať.")

		return
	}

	if len(data) > maxContactPhotoBytes {
		h.photoFailed(w, r, "Fotka je príliš veľká (max 8 MB).")

		return
	}

	ext, ok := uploads.DetectImage(data, maxContactPhotoBytes)
	if !ok {
		h.photoFailed(w, r, "Podporované sú len JPG, PNG a WebP.")

		return
	}

	urlPath, err := h.uploads.Save("manager", data, ext)
	if err != nil {
		h.photoFailed(w, r, "Fotku sa nepodarilo uložiť.")

		return
	}

	old, err := h.queries.GetContact(r.Context())
	if err != nil {
		h.photoFailed(w, r, "Fotku sa nepodarilo uložiť.")

		return
	}

	if _, err := h.queries.UpdateContactPhoto(r.Context(), urlPath); err != nil {
		h.photoFailed(w, r, "Fotku sa nepodarilo uložiť.")

		return
	}

	// Best effort: drop the previous file so uploads do not accumulate.
	// A re-upload of identical bytes maps to the same name and is kept.
	h.uploads.Remove(old.PhotoPath, urlPath)

	if !htmx.IsHtmxRequest(r) && r.Header.Get("Accept") != "application/json" && r.URL.Query().Get("format") == "" {
		// Plain no-JS form fallback.
		htmx.Redirect(w, r, "/admin/contact")

		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"photoUrl": urlPath,
		"message":  "Fotka uložená.",
	})
}

// DeletePhoto removes the manager photo: the contact row loses its
// photo path (the site shows a silhouette placeholder instead) and the
// photo file is deleted. It answers JSON for the admin fetch() flow.
func (h *ContactHandler) DeletePhoto(w http.ResponseWriter, r *http.Request) {
	old, err := h.queries.GetContact(r.Context())
	if err != nil {
		h.photoFailed(w, r, "Fotku sa nepodarilo zmazať.")

		return
	}

	if _, err := h.queries.UpdateContactPhoto(r.Context(), ""); err != nil {
		h.photoFailed(w, r, "Fotku sa nepodarilo zmazať.")

		return
	}

	// Best effort: the file is just a cache of the DB state.
	h.uploads.Remove(old.PhotoPath, "")

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Fotka zmazaná."})
}

// updateFailed answers a failed update: the form with an inline error
// for htmx, plain error otherwise.
func (h *ContactHandler) updateFailed(w http.ResponseWriter, r *http.Request, data contact.Contact, msg string) {
	if !htmx.IsHtmxRequest(r) {
		http.Error(w, msg, http.StatusBadRequest)

		return
	}

	admintemplates.AdminContactForm(data, msg).Render(r.Context(), w)
}

// photoFailed answers a failed photo upload: JSON for the cropper fetch
// flow, plain error otherwise.
func (h *ContactHandler) photoFailed(w http.ResponseWriter, r *http.Request, msg string) {
	if htmx.IsHtmxRequest(r) || strings.Contains(r.Header.Get("Accept"), "json") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})

		return
	}

	http.Error(w, msg, http.StatusBadRequest)
}

// contactFormParams holds cleaned contact form values.
type contactFormParams struct {
	Name  string
	Role  string
	Phone string
	Email string
}

// parseContactForm validates the contact form fields.
// It returns the cleaned values or a Slovak error message.
func parseContactForm(r *http.Request) (contactFormParams, string) {
	name, ok := contact.ValidateName(r.FormValue("name"))
	if !ok {
		if strings.TrimSpace(r.FormValue("name")) == "" {
			return contactFormParams{}, "Vyplň meno."
		}

		return contactFormParams{}, "Meno je príliš dlhé (max 100 znakov)."
	}

	role, ok := contact.ValidateRole(r.FormValue("role"))
	if !ok {
		if strings.TrimSpace(r.FormValue("role")) == "" {
			return contactFormParams{}, "Vyplň pozíciu."
		}

		return contactFormParams{}, "Pozícia je príliš dlhá (max 100 znakov)."
	}

	phone, ok := contact.ValidatePhone(r.FormValue("phone"))
	if !ok {
		if strings.TrimSpace(r.FormValue("phone")) == "" {
			return contactFormParams{}, "Vyplň telefón."
		}

		return contactFormParams{}, "Telefónne číslo nie je platné – skús napr. +421 900 123 456."
	}

	email, ok := contact.ValidateEmail(r.FormValue("email"))
	if !ok {
		if strings.TrimSpace(r.FormValue("email")) == "" {
			return contactFormParams{}, "Vyplň e-mail."
		}

		return contactFormParams{}, "E-mail nie je platný – skontroluj preklep."
	}

	return contactFormParams{Name: name, Role: role, Phone: phone, Email: email}, ""
}

// contactFromRequest builds the display model from submitted values so a
// failed update does not wipe the editor.
func contactFromRequest(r *http.Request) contact.Contact {
	return contact.Contact{
		Name:  strings.TrimSpace(r.FormValue("name")),
		Role:  strings.TrimSpace(r.FormValue("role")),
		Phone: strings.TrimSpace(r.FormValue("phone")),
		Email: strings.TrimSpace(r.FormValue("email")),
	}
}
