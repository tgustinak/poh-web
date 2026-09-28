package admin

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	dbmigrations "github.com/tgustinak/pohkapela.sk/db/migrations"
	"github.com/tgustinak/pohkapela.sk/internal/db"
)

func newTestSocialHandler(t *testing.T) (*SocialHandler, *db.Queries) {
	t.Helper()

	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	goose.SetBaseFS(dbmigrations.FS)

	if err := goose.SetDialect("sqlite"); err != nil {
		t.Fatalf("set goose dialect: %v", err)
	}

	if err := goose.Up(conn, "."); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	queries := db.New(conn)

	return NewSocialHandler(queries), queries
}

func socialUpdateRequest(platform string, values url.Values, htmx bool) *http.Request {
	req := httptest.NewRequest(
		http.MethodPut,
		"/admin/social/"+platform,
		strings.NewReader(values.Encode()),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	req.SetPathValue("platform", platform)

	return req
}

func TestSocialListPageRendersSeed(t *testing.T) {
	handler, _ := newTestSocialHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/social", nil)
	rec := httptest.NewRecorder()

	handler.ListPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	for _, want := range []string{
		"kapelaPOH",
		"@kapela_p.o.h",
		"https://www.facebook.com/kapelaPOH/",
		"hx-get=\"/admin/social/facebook/edit\"",
		"Upraviť",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected list page to contain %q", want)
		}
	}

	if strings.Contains(body, "Pridať") || strings.Contains(body, "Odobrať") {
		t.Fatal("expected no add/remove controls on social page")
	}
}

func TestSocialUpdateValidation(t *testing.T) {
	handler, _ := newTestSocialHandler(t)

	// Empty handle must be rejected with an inline error.
	rec := httptest.NewRecorder()
	handler.Update(rec, socialUpdateRequest("facebook",
		url.Values{"handle": {""}, "url": {"https://www.facebook.com/kapelaPOH/"}}, true))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected htmx validation 200, got %d", rec.Code)
	}

	if body := rec.Body.String(); !strings.Contains(body, "Vyplň zobrazovaný názov.") {
		t.Fatalf("expected inline handle error, got %q", body)
	}

	// Non-https URL must be rejected.
	rec = httptest.NewRecorder()
	handler.Update(rec, socialUpdateRequest("facebook",
		url.Values{"handle": {"kapelaPOH"}, "url": {"http://example.com/x"}}, true))

	if body := rec.Body.String(); !strings.Contains(body, "URL musí začínať https://") {
		t.Fatalf("expected inline URL error, got %q", body)
	}
}

func TestSocialUpdateSuccess(t *testing.T) {
	handler, queries := newTestSocialHandler(t)

	rec := httptest.NewRecorder()
	handler.Update(rec, socialUpdateRequest("spotify",
		url.Values{"handle": {"E2E handle"}, "url": {"https://open.spotify.com/artist/e2e"}}, true))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "showToast") {
		t.Fatalf("expected toast trigger, got %q", trigger)
	}

	updated, err := queries.GetSocialLinkByPlatform(t.Context(), "spotify")
	if err != nil {
		t.Fatalf("load updated link: %v", err)
	}

	if updated.Handle != "E2E handle" || updated.Url != "https://open.spotify.com/artist/e2e" {
		t.Fatalf("expected update to persist, got %+v", updated)
	}

	if updated.DisplayName != "Spotify" {
		t.Fatalf("expected display name unchanged, got %q", updated.DisplayName)
	}
}

func TestSocialUnknownPlatform404(t *testing.T) {
	handler, _ := newTestSocialHandler(t)

	for _, tc := range []struct {
		name    string
		method  func(rec *httptest.ResponseRecorder, req *http.Request)
		target  string
		htmx    bool
		want404 bool
	}{
		{"row", func(rec *httptest.ResponseRecorder, req *http.Request) { handler.Row(rec, req) }, "tiktok", true, true},
		{"edit", func(rec *httptest.ResponseRecorder, req *http.Request) { handler.EditForm(rec, req) }, "tiktok", true, true},
		{"update", func(rec *httptest.ResponseRecorder, req *http.Request) { handler.Update(rec, req) }, "tiktok", true, true},
	} {
		req := httptest.NewRequest(http.MethodGet, "/admin/social/"+tc.target, nil)
		req.SetPathValue("platform", tc.target)
		if tc.htmx {
			req.Header.Set("HX-Request", "true")
		}
		rec := httptest.NewRecorder()

		tc.method(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: expected 404, got %d", tc.name, rec.Code)
		}
	}
}
