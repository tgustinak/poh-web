package admin

import (
	"net/http"
	"strings"

	"github.com/tgustinak/pohkapela.sk/internal/about"
	"github.com/tgustinak/pohkapela.sk/internal/auth"
	"github.com/tgustinak/pohkapela.sk/internal/db"
	"github.com/tgustinak/pohkapela.sk/internal/htmx"
	admintemplates "github.com/tgustinak/pohkapela.sk/web/templates/admin"
)

// AboutHandler serves admin editing of the "O nás" section (singleton row).
type AboutHandler struct {
	queries *db.Queries
}

// NewAboutHandler creates an admin about handler.
func NewAboutHandler(queries *db.Queries) *AboutHandler {
	return &AboutHandler{queries: queries}
}

// EditPage renders the about edit page with the current text.
func (h *AboutHandler) EditPage(w http.ResponseWriter, r *http.Request) {
	csrf, _ := auth.CsrfFromRequest(r)

	row, err := h.queries.GetAbout(r.Context())
	if err != nil {
		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	admintemplates.AdminAbout(about.FromDB(row), csrf, "").Render(r.Context(), w)
}

// Update saves the about text. htmx requests get the re-rendered form
// plus a toast; plain form posts redirect back to the edit page.
func (h *AboutHandler) Update(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.updateFailed(w, r, fromRequest(r), "Nepodarilo sa spracovať formulár, skús to znova.")

		return
	}

	params, errMsg := parseAboutForm(r)
	if errMsg != "" {
		h.updateFailed(w, r, fromRequest(r), errMsg)

		return
	}

	updated, err := h.queries.UpdateAbout(r.Context(), db.UpdateAboutParams{
		P1:    params.P1,
		P2:    params.P2,
		Quote: params.Quote,
	})
	if err != nil {
		h.updateFailed(w, r, fromRequest(r), "Zmeny sa nepodarilo uložiť, skús to znova.")

		return
	}

	if !htmx.IsHtmxRequest(r) {
		htmx.Redirect(w, r, "/admin/about")

		return
	}

	htmx.Toast(w, "Zmeny uložené.")
	admintemplates.AdminAboutForm(about.FromDB(updated), "").Render(r.Context(), w)
}

// updateFailed answers a failed update: the form with an inline error
// for htmx, plain error otherwise.
func (h *AboutHandler) updateFailed(w http.ResponseWriter, r *http.Request, data about.About, msg string) {
	if !htmx.IsHtmxRequest(r) {
		http.Error(w, msg, http.StatusBadRequest)

		return
	}

	admintemplates.AdminAboutForm(data, msg).Render(r.Context(), w)
}

// aboutFormParams holds cleaned about form values.
type aboutFormParams struct {
	P1    string
	P2    string
	Quote string
}

// parseAboutForm validates the about form fields.
// It returns the cleaned values or a Slovak error message.
func parseAboutForm(r *http.Request) (aboutFormParams, string) {
	p1, ok := about.ValidateParagraph(r.FormValue("p1"))
	if !ok {
		if strings.TrimSpace(r.FormValue("p1")) == "" {
			return aboutFormParams{}, "Vyplň prvý odstavec."
		}

		return aboutFormParams{}, "Prvý odstavec je príliš dlhý (max 2000 znakov)."
	}

	p2, ok := about.ValidateParagraph(r.FormValue("p2"))
	if !ok {
		if strings.TrimSpace(r.FormValue("p2")) == "" {
			return aboutFormParams{}, "Vyplň druhý odstavec."
		}

		return aboutFormParams{}, "Druhý odstavec je príliš dlhý (max 2000 znakov)."
	}

	quote, ok := about.ValidateQuote(r.FormValue("quote"))
	if !ok {
		if strings.TrimSpace(r.FormValue("quote")) == "" {
			return aboutFormParams{}, "Vyplň citát."
		}

		return aboutFormParams{}, "Citát je príliš dlhý (max 500 znakov)."
	}

	return aboutFormParams{P1: p1, P2: p2, Quote: quote}, ""
}

// fromRequest builds the display model from submitted values so a failed
// update does not wipe the editor.
func fromRequest(r *http.Request) about.About {
	return about.About{
		P1:    strings.TrimSpace(r.FormValue("p1")),
		P2:    strings.TrimSpace(r.FormValue("p2")),
		Quote: strings.TrimSpace(r.FormValue("quote")),
	}
}
