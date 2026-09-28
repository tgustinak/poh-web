package admin

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/tgustinak/pohkapela.sk/internal/auth"
	"github.com/tgustinak/pohkapela.sk/internal/db"
	"github.com/tgustinak/pohkapela.sk/internal/htmx"
	"github.com/tgustinak/pohkapela.sk/internal/social"
	admintemplates "github.com/tgustinak/pohkapela.sk/web/templates/admin"
)

// SocialHandler serves admin editing of the "Social" section.
// The link set is fixed (6 platforms, no add/delete): admin edits
// only handle and url per platform.
type SocialHandler struct {
	queries *db.Queries
}

// NewSocialHandler creates an admin social handler.
func NewSocialHandler(queries *db.Queries) *SocialHandler {
	return &SocialHandler{queries: queries}
}

// ListPage renders the social link management page in display order.
func (h *SocialHandler) ListPage(w http.ResponseWriter, r *http.Request) {
	csrf, _ := auth.CsrfFromRequest(r)

	rows, err := h.queries.ListSocialLinksOrdered(r.Context())
	if err != nil {
		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	admintemplates.AdminSocial(rows, csrf).Render(r.Context(), w)
}

// Row renders a single social table row (also the cancel target
// for inline editing).
func (h *SocialHandler) Row(w http.ResponseWriter, r *http.Request) {
	link, ok := h.findLink(w, r)
	if !ok {
		return
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/social")

		return
	}

	admintemplates.AdminSocialRow(link, "").Render(r.Context(), w)
}

// EditForm renders the inline edit row for a social link.
func (h *SocialHandler) EditForm(w http.ResponseWriter, r *http.Request) {
	link, ok := h.findLink(w, r)
	if !ok {
		return
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/social")

		return
	}

	admintemplates.AdminSocialEditRow(link, "").Render(r.Context(), w)
}

// Update saves an edited social link. On success it returns the updated
// row plus a toast; on validation error the edit form with the message.
func (h *SocialHandler) Update(w http.ResponseWriter, r *http.Request) {
	link, ok := h.findLink(w, r)
	if !ok {
		return
	}

	if err := r.ParseForm(); err != nil {
		h.updateFailed(w, r, link, "Nepodarilo sa spracovať formulár, skús to znova.")

		return
	}

	params, errMsg := parseSocialForm(r)
	if errMsg != "" {
		h.updateFailed(w, r, link, errMsg)

		return
	}

	updated, err := h.queries.UpdateSocialLink(r.Context(), db.UpdateSocialLinkParams{
		Handle:   params.Handle,
		Url:      params.URL,
		Platform: link.Platform,
	})
	if err != nil {
		h.updateFailed(w, r, link, "Zmeny sa nepodarilo uložiť, skús to znova.")

		return
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/social")

		return
	}

	htmx.Toast(w, "Zmeny uložené.")
	admintemplates.AdminSocialRow(updated, "").Render(r.Context(), w)
}

// updateFailed answers a failed update: the edit form with an inline
// error for htmx, plain error otherwise.
func (h *SocialHandler) updateFailed(w http.ResponseWriter, r *http.Request, link db.SocialLink, msg string) {
	if !htmx.IsHtmxRequest(r) {
		http.Error(w, msg, http.StatusBadRequest)

		return
	}

	// Echo submitted values so a failed update does not wipe the editor.
	link.Handle = strings.TrimSpace(r.FormValue("handle"))
	link.Url = strings.TrimSpace(r.FormValue("url"))

	admintemplates.AdminSocialEditRow(link, msg).Render(r.Context(), w)
}

// findLink loads the social link addressed by the {platform} path value.
// It writes 404 when the platform is unknown or the row does not exist.
func (h *SocialHandler) findLink(w http.ResponseWriter, r *http.Request) (db.SocialLink, bool) {
	platform := r.PathValue("platform")
	if !social.IsPlatform(platform) {
		http.Error(w, "Sieť sa nenašla.", http.StatusNotFound)

		return db.SocialLink{}, false
	}

	link, err := h.queries.GetSocialLinkByPlatform(r.Context(), platform)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Sieť sa nenašla.", http.StatusNotFound)

			return db.SocialLink{}, false
		}

		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return db.SocialLink{}, false
	}

	return link, true
}

// socialFormParams holds cleaned social form values.
type socialFormParams struct {
	Handle string
	URL    string
}

// parseSocialForm validates the social form fields.
// It returns the cleaned values or a Slovak error message.
func parseSocialForm(r *http.Request) (socialFormParams, string) {
	handle, ok := social.ValidateHandle(r.FormValue("handle"))
	if !ok {
		if strings.TrimSpace(r.FormValue("handle")) == "" {
			return socialFormParams{}, "Vyplň zobrazovaný názov."
		}

		return socialFormParams{}, "Zobrazovaný názov je príliš dlhý (max 100 znakov)."
	}

	url, ok := social.ValidateURL(r.FormValue("url"))
	if !ok {
		if strings.TrimSpace(r.FormValue("url")) == "" {
			return socialFormParams{}, "Vyplň URL."
		}

		return socialFormParams{}, "URL musí začínať https:// a byť platná adresa."
	}

	return socialFormParams{Handle: handle, URL: url}, ""
}
