package admin

import (
	"bytes"
	"database/sql"
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
	"github.com/tgustinak/pohkapela.sk/internal/organizers"
)

func newTestOrganizersHandler(t *testing.T) (*OrganizersHandler, *db.Queries, string) {
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

	return NewOrganizersHandler(queries, dir), queries, dir
}

type uploadFile struct {
	filename string
	content  []byte
}

// multipartOrganizersRequest builds a multipart htmx request with files
// and extra form values, addressing path params via SetPathValue.
func multipartOrganizersRequest(
	t *testing.T,
	method, target string,
	files map[string]uploadFile,
	values url.Values,
	pathParams map[string]string,
) *http.Request {
	t.Helper()

	var buf bytes.Buffer

	w := multipart.NewWriter(&buf)

	for field, f := range files {
		part, err := w.CreateFormFile(field, f.filename)
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}

		if _, err := part.Write(f.content); err != nil {
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

func seedPDF() []byte {
	return []byte("%PDF-1.4\n%E2E test rider content\n")
}

func seedJPEG(t *testing.T) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "web", "static", "files", "poh-promo-2.jpg"))
	if err != nil {
		t.Fatalf("read seed photo: %v", err)
	}

	return data
}

func TestOrganizersListPageRendersSeed(t *testing.T) {
	handler, _, _ := newTestOrganizersHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/organizers", nil)
	rec := httptest.NewRecorder()

	handler.ListPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	for _, want := range []string{
		"Technický rider",
		"Input list",
		"Logo v krivkách",
		"poh-rider.pdf",
		"Upravené",
		"organizer-photo-create-form",
		"hx-put=\"/admin/organizers/documents/rider\"",
		"promo-1-preview",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected list page to contain %q", want)
		}
	}

	if strings.Contains(body, "Pridať súbor / fotku") || strings.Contains(body, `onchange="adminToast`) {
		t.Fatal("expected no demo controls on organizers page")
	}
}

func TestReplaceDocumentUnknownSlot404(t *testing.T) {
	handler, _, _ := newTestOrganizersHandler(t)

	req := multipartOrganizersRequest(t, http.MethodPut, "/admin/organizers/documents/nope",
		map[string]uploadFile{"file": {"rider.pdf", seedPDF()}}, nil, map[string]string{"slot": "nope"})
	rec := httptest.NewRecorder()

	handler.ReplaceDocument(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestReplaceDocumentValidation(t *testing.T) {
	handler, _, _ := newTestOrganizersHandler(t)

	// Non-PDF content must be rejected with an inline error.
	req := multipartOrganizersRequest(t, http.MethodPut, "/admin/organizers/documents/rider",
		map[string]uploadFile{"file": {"foto.jpg", seedJPEG(t)}}, nil, map[string]string{"slot": "rider"})
	rec := httptest.NewRecorder()

	handler.ReplaceDocument(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected htmx validation 200, got %d", rec.Code)
	}

	if body := rec.Body.String(); !strings.Contains(body, "Dokument musí byť PDF.") {
		t.Fatalf("expected inline PDF error, got %q", body)
	}

	// Missing file must be rejected.
	req = multipartOrganizersRequest(t, http.MethodPut, "/admin/organizers/documents/rider",
		nil, nil, map[string]string{"slot": "rider"})
	rec = httptest.NewRecorder()

	handler.ReplaceDocument(rec, req)

	if body := rec.Body.String(); !strings.Contains(body, "Vyber PDF súbor") {
		t.Fatalf("expected missing file error, got %q", body)
	}
}

func TestReplaceDocumentSuccess(t *testing.T) {
	handler, queries, dir := newTestOrganizersHandler(t)

	req := multipartOrganizersRequest(t, http.MethodPut, "/admin/organizers/documents/rider",
		map[string]uploadFile{"file": {"rider.pdf", seedPDF()}}, nil, map[string]string{"slot": "rider"})
	rec := httptest.NewRecorder()

	handler.ReplaceDocument(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "showToast") {
		t.Fatalf("expected toast trigger, got %q", trigger)
	}

	updated, err := queries.GetOrganizerDocumentBySlot(t.Context(), "rider")
	if err != nil {
		t.Fatalf("load updated document: %v", err)
	}

	if !strings.HasPrefix(updated.FilePath, "/uploads/organizatori-rider-") || !strings.HasSuffix(updated.FilePath, ".pdf") {
		t.Fatalf("expected content-hashed upload path, got %q", updated.FilePath)
	}

	if updated.SizeBytes != int64(len(seedPDF())) {
		t.Fatalf("expected size %d, got %d", len(seedPDF()), updated.SizeBytes)
	}

	if updated.DisplayName != "Technický rider" {
		t.Fatalf("expected display name unchanged, got %q", updated.DisplayName)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read upload dir: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 uploaded file, got %d", len(entries))
	}
}

func TestCreatePhotoValidation(t *testing.T) {
	handler, _, _ := newTestOrganizersHandler(t)

	// PDF bytes are not a valid photo.
	req := multipartOrganizersRequest(t, http.MethodPost, "/admin/organizers/photos",
		map[string]uploadFile{"photo": {"doc.pdf", seedPDF()}},
		url.Values{"alt": {"Popis"}}, nil)
	rec := httptest.NewRecorder()

	handler.CreatePhoto(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected htmx validation 200, got %d", rec.Code)
	}

	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "showToast") {
		t.Fatalf("expected error toast, got %q", trigger)
	}

	// Missing photo must be rejected.
	req = multipartOrganizersRequest(t, http.MethodPost, "/admin/organizers/photos",
		nil, url.Values{"alt": {"Popis"}}, nil)
	rec = httptest.NewRecorder()

	handler.CreatePhoto(rec, req)

	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "showToast") {
		t.Fatalf("expected error toast, got %q", trigger)
	}
}

func TestCreatePhotoSuccess(t *testing.T) {
	handler, queries, dir := newTestOrganizersHandler(t)

	req := multipartOrganizersRequest(t, http.MethodPost, "/admin/organizers/photos",
		map[string]uploadFile{"photo": {"promo.jpg", seedJPEG(t)}},
		url.Values{"alt": {"E2E promo"}}, nil)
	rec := httptest.NewRecorder()

	handler.CreatePhoto(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// The HX-Trigger header is ASCII-only (non-ASCII is \u-escaped),
	// so match the ASCII-safe prefix.
	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "Fotka pridan") {
		t.Fatalf("expected success toast, got %q", trigger)
	}

	photos, err := queries.ListOrganizerPhotosOrdered(t.Context())
	if err != nil {
		t.Fatalf("list photos: %v", err)
	}

	if len(photos) != 3 {
		t.Fatalf("expected 2 seed + 1 new photo, got %d", len(photos))
	}

	last := photos[len(photos)-1]
	if last.Alt != "E2E promo" {
		t.Fatalf("expected alt passthrough, got %q", last.Alt)
	}

	if !strings.HasPrefix(last.FilePath, "/uploads/organizatori-foto-") {
		t.Fatalf("expected upload path, got %q", last.FilePath)
	}

	// No separate preview uploaded: preview falls back to the full file.
	if last.PreviewPath != last.FilePath {
		t.Fatalf("expected preview fallback to full file, got %q", last.PreviewPath)
	}

	if want := organizers.MetaLabel(".jpg", int64(len(seedJPEG(t)))); last.MetaLabel != want {
		t.Fatalf("expected meta %q, got %q", want, last.MetaLabel)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read upload dir: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 uploaded file (preview deduped), got %d", len(entries))
	}
}

func TestUpdatePhotoAlt(t *testing.T) {
	handler, queries, _ := newTestOrganizersHandler(t)

	photos, err := queries.ListOrganizerPhotosOrdered(t.Context())
	if err != nil {
		t.Fatalf("list photos: %v", err)
	}

	id := photos[0].ID
	target := "/admin/organizers/photos/" + itoa(id)

	form := url.Values{"alt": {"Nový popis"}}
	req := httptest.NewRequest(http.MethodPut, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.SetPathValue("id", itoa(id))
	rec := httptest.NewRecorder()

	handler.UpdatePhotoAlt(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	updated, err := queries.GetOrganizerPhotoByID(t.Context(), id)
	if err != nil {
		t.Fatalf("load photo: %v", err)
	}

	if updated.Alt != "Nový popis" {
		t.Fatalf("expected alt update, got %q", updated.Alt)
	}

	// Overlong alt must be rejected.
	form = url.Values{"alt": {strings.Repeat("a", 201)}}
	req = httptest.NewRequest(http.MethodPut, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.SetPathValue("id", itoa(id))
	rec = httptest.NewRecorder()

	handler.UpdatePhotoAlt(rec, req)

	if body := rec.Body.String(); !strings.Contains(body, "Popis je príliš dlhý") {
		t.Fatalf("expected inline alt error, got %q", body)
	}
}

func TestDeletePhoto(t *testing.T) {
	handler, queries, dir := newTestOrganizersHandler(t)

	// First upload a photo so there is an upload file to clean up.
	req := multipartOrganizersRequest(t, http.MethodPost, "/admin/organizers/photos",
		map[string]uploadFile{"photo": {"promo.jpg", seedJPEG(t)}},
		url.Values{"alt": {"Na zmazanie"}}, nil)
	handler.CreatePhoto(httptest.NewRecorder(), req)

	photos, err := queries.ListOrganizerPhotosOrdered(t.Context())
	if err != nil {
		t.Fatalf("list photos: %v", err)
	}

	last := photos[len(photos)-1]

	del := httptest.NewRequest(http.MethodDelete, "/admin/organizers/photos/x", nil)
	del.Header.Set("HX-Request", "true")
	del.SetPathValue("id", itoa(last.ID))
	rec := httptest.NewRecorder()

	handler.DeletePhoto(rec, del)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	remaining, err := queries.ListOrganizerPhotosOrdered(t.Context())
	if err != nil {
		t.Fatalf("list photos: %v", err)
	}

	if len(remaining) != len(photos)-1 {
		t.Fatalf("expected one photo deleted, got %d -> %d", len(photos), len(remaining))
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read upload dir: %v", err)
	}

	if len(entries) != 0 {
		t.Fatalf("expected upload cleaned up, got %d files", len(entries))
	}
}

func TestPhotoUnknownID404(t *testing.T) {
	handler, _, _ := newTestOrganizersHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/organizers/photos/999999", nil)
	req.Header.Set("HX-Request", "true")
	req.SetPathValue("id", "999999")
	rec := httptest.NewRecorder()

	handler.PhotoRow(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
