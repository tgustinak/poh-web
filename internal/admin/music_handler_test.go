package admin

import (
	"bytes"
	"database/sql"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	dbmigrations "github.com/tgustinak/pohkapela.sk/db/migrations"
	"github.com/tgustinak/pohkapela.sk/internal/db"
)

func newTestMusicHandler(t *testing.T) (*MusicHandler, *db.Queries, string) {
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
	dir := t.TempDir()

	return NewMusicHandler(queries, dir), queries, dir
}

func seedCover(t *testing.T) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "web", "static", "images", "album-na-konci-sveta.jpg"))
	if err != nil {
		t.Fatalf("read seed cover: %v", err)
	}

	return data
}

// multipartMusicRequest builds a multipart htmx album request with an
// optional cover file and text fields.
func multipartMusicRequest(
	t *testing.T,
	method, target string,
	cover []byte,
	values url.Values,
	pathParams map[string]string,
) *http.Request {
	t.Helper()

	var buf bytes.Buffer

	w := multipart.NewWriter(&buf)

	if cover != nil {
		part, err := w.CreateFormFile("cover", "obal.jpg")
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}

		if _, err := part.Write(cover); err != nil {
			t.Fatalf("write part: %v", err)
		}
	}

	for key, vals := range values {
		for _, v := range vals {
			if err := w.WriteField(key, v); err != nil {
				t.Fatalf("write field: %v", err)
			}
		}
	}

	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(method, target, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("HX-Request", "true")

	for k, v := range pathParams {
		req.SetPathValue(k, v)
	}

	return req
}

func validAlbumValues() url.Values {
	return url.Values{
		"title":  {"E2E album"},
		"year":   {"2025"},
		"badge":  {"E2E"},
		"is_new": {"1"},
	}
}

func TestMusicListPageRendersSeed(t *testing.T) {
	handler, _, _ := newTestMusicHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/music", nil)
	rec := httptest.NewRecorder()

	handler.ListPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	for _, want := range []string{
		"Život",
		"Na konci sveta",
		"album-create-form",
		`href="/admin/social"`,
		"albumReorderInit",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected list page to contain %q", want)
		}
	}

	// The shared layout ships a demo-helper script mentioning data-demo;
	// assert the page itself has no demo form or demo toast buttons.
	if strings.Contains(body, "<form data-demo") || strings.Contains(body, `onclick="adminToast`) {
		t.Fatal("expected no demo controls on music page")
	}
}

func TestMusicCreateValidation(t *testing.T) {
	handler, queries, _ := newTestMusicHandler(t)

	before, err := queries.CountAlbums(t.Context())
	if err != nil {
		t.Fatalf("count albums: %v", err)
	}

	missingCover := validAlbumValues()
	badYear := validAlbumValues()
	badYear.Set("year", "99")
	emptyTitle := validAlbumValues()
	emptyTitle.Set("title", "  ")

	for name, req := range map[string]*http.Request{
		"missing cover": multipartMusicRequest(t, http.MethodPost, "/admin/music", nil, missingCover, nil),
		"bad year":      multipartMusicRequest(t, http.MethodPost, "/admin/music", seedCover(t), badYear, nil),
		"empty title":   multipartMusicRequest(t, http.MethodPost, "/admin/music", seedCover(t), emptyTitle, nil),
		"bad cover":     multipartMusicRequest(t, http.MethodPost, "/admin/music", []byte("%PDF-1.4 fake"), validAlbumValues(), nil),
	} {
		rec := httptest.NewRecorder()

		handler.Create(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s: expected 200 with toast, got %d", name, rec.Code)
		}

		if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "showToast") {
			t.Fatalf("%s: expected error toast, got %q", name, trigger)
		}
	}

	after, err := queries.CountAlbums(t.Context())
	if err != nil {
		t.Fatalf("count albums: %v", err)
	}

	if after != before {
		t.Fatalf("expected no new albums after invalid creates, before=%d after=%d", before, after)
	}
}

func TestMusicCreateSuccess(t *testing.T) {
	handler, queries, dir := newTestMusicHandler(t)

	req := multipartMusicRequest(t, http.MethodPost, "/admin/music", seedCover(t), validAlbumValues(), nil)
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	trigger := rec.Header().Get("HX-Trigger")
	if !strings.Contains(trigger, "showToast") || !strings.Contains(trigger, "album-create-form") {
		t.Fatalf("expected toast + form reset, got %q", trigger)
	}

	ordered, err := queries.ListAlbumsOrdered(t.Context())
	if err != nil {
		t.Fatalf("list albums: %v", err)
	}

	if len(ordered) == 0 || ordered[0].Title != "E2E album" {
		t.Fatalf("expected new album first, got %+v", ordered)
	}

	if ordered[0].Position != 1 || ordered[0].IsNew != 1 || ordered[0].Year != 2025 {
		t.Fatalf("unexpected new album row: %+v", ordered[0])
	}

	if !strings.HasPrefix(ordered[0].CoverPath, "/uploads/hudba-") {
		t.Fatalf("expected hashed cover path, got %q", ordered[0].CoverPath)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read upload dir: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 uploaded cover, got %d", len(entries))
	}
}

func TestMusicCreateWithoutBadge(t *testing.T) {
	handler, queries, _ := newTestMusicHandler(t)

	values := validAlbumValues()
	values.Set("badge", "  ")

	req := multipartMusicRequest(t, http.MethodPost, "/admin/music", seedCover(t), values, nil)
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	ordered, err := queries.ListAlbumsOrdered(t.Context())
	if err != nil {
		t.Fatalf("list albums: %v", err)
	}

	if len(ordered) == 0 || ordered[0].Badge != "" {
		t.Fatalf("expected empty badge to persist, got %+v", ordered)
	}
}

func TestMusicUpdate(t *testing.T) {
	handler, queries, dir := newTestMusicHandler(t)

	rows, err := queries.ListAlbumsOrdered(t.Context())
	if err != nil {
		t.Fatalf("list albums: %v", err)
	}

	id := strconv.FormatInt(rows[0].ID, 10)
	target := "/admin/music/" + id

	// Text-only update keeps the cover.
	values := url.Values{"title": {"Premenovaný"}, "year": {"2024"}, "badge": {"Reedícia"}}
	req := multipartMusicRequest(t, http.MethodPut, target, nil, values, map[string]string{"id": id})
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	updated, err := queries.GetAlbumByID(t.Context(), rows[0].ID)
	if err != nil {
		t.Fatalf("load album: %v", err)
	}

	if updated.Title != "Premenovaný" || updated.Year != 2024 || updated.Badge != "Reedícia" {
		t.Fatalf("expected update to persist, got %+v", updated)
	}

	if updated.CoverPath != rows[0].CoverPath {
		t.Fatalf("expected cover unchanged, got %q", updated.CoverPath)
	}

	if updated.IsNew != 0 {
		t.Fatalf("expected unchecked is_new to clear, got %d", updated.IsNew)
	}

	// Invalid year returns the edit form with an inline error.
	values = url.Values{"title": {"X"}, "year": {"99999"}, "badge": {"Y"}}
	req = multipartMusicRequest(t, http.MethodPut, target, nil, values, map[string]string{"id": id})
	rec = httptest.NewRecorder()

	handler.Update(rec, req)

	if body := rec.Body.String(); !strings.Contains(body, "Rok musí byť") {
		t.Fatalf("expected inline year error, got %q", body)
	}

	// Cover replacement stores a new upload.
	req = multipartMusicRequest(t, http.MethodPut, target, seedCover(t), validAlbumValues(), map[string]string{"id": id})
	rec = httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected cover update 200, got %d", rec.Code)
	}

	withCover, err := queries.GetAlbumByID(t.Context(), rows[0].ID)
	if err != nil {
		t.Fatalf("load album: %v", err)
	}

	if !strings.HasPrefix(withCover.CoverPath, "/uploads/hudba-") {
		t.Fatalf("expected new cover path, got %q", withCover.CoverPath)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read upload dir: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 uploaded cover, got %d", len(entries))
	}
}

func TestMusicDelete(t *testing.T) {
	handler, queries, dir := newTestMusicHandler(t)

	// Upload-backed album so cleanup is verifiable.
	req := multipartMusicRequest(t, http.MethodPost, "/admin/music", seedCover(t), validAlbumValues(), nil)
	handler.Create(httptest.NewRecorder(), req)

	rows, err := queries.ListAlbumsOrdered(t.Context())
	if err != nil {
		t.Fatalf("list albums: %v", err)
	}

	target := rows[0]

	del := httptest.NewRequest(http.MethodDelete, "/admin/music/x", nil)
	del.Header.Set("HX-Request", "true")
	del.SetPathValue("id", strconv.FormatInt(target.ID, 10))
	rec := httptest.NewRecorder()

	handler.Delete(rec, del)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if _, err := queries.GetAlbumByID(t.Context(), target.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected album deleted, got %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read upload dir: %v", err)
	}

	if len(entries) != 0 {
		t.Fatalf("expected cover cleaned up, got %d files", len(entries))
	}
}

func TestMusicReorder(t *testing.T) {
	handler, queries, _ := newTestMusicHandler(t)

	rows, err := queries.ListAlbumsOrdered(t.Context())
	if err != nil {
		t.Fatalf("list albums: %v", err)
	}

	if len(rows) < 2 {
		t.Fatal("expected seeded albums")
	}

	reversed := url.Values{}
	for i := len(rows) - 1; i >= 0; i-- {
		reversed.Add("id", strconv.FormatInt(rows[i].ID, 10))
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/music/reorder", strings.NewReader(reversed.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	handler.Reorder(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	ordered, err := queries.ListAlbumsOrdered(t.Context())
	if err != nil {
		t.Fatalf("list albums: %v", err)
	}

	if ordered[0].ID != rows[len(rows)-1].ID {
		t.Fatalf("expected reversed order, got %+v", ordered)
	}
}

func TestMusicUnknownID404(t *testing.T) {
	handler, _, _ := newTestMusicHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/music/999999", nil)
	req.Header.Set("HX-Request", "true")
	req.SetPathValue("id", "999999")
	rec := httptest.NewRecorder()

	handler.Row(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}
