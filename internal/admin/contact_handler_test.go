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
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	dbmigrations "github.com/tgustinak/pohkapela.sk/db/migrations"
	"github.com/tgustinak/pohkapela.sk/internal/db"
	"github.com/tgustinak/pohkapela.sk/internal/scripts"
)

func newTestContactHandler(t *testing.T) (*ContactHandler, *db.Queries) {
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
	handler := NewContactHandler(queries, t.TempDir())

	return handler, queries
}

func contactForm(values url.Values) *http.Request {
	req := httptest.NewRequest(http.MethodPut, "/admin/contact", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	return req
}

func validContactValues() url.Values {
	return url.Values{
		"name":  {"Jana Testová"},
		"role":  {"Manažérka"},
		"phone": {"+421 900 123 456"},
		"email": {"kontakt@example.com"},
	}
}

func TestContactEditPageRendersSeed(t *testing.T) {
	handler, _ := newTestContactHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/contact", nil)
	ctx, _ := scripts.WithCollector(req.Context())
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.EditPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	for _, want := range []string{"Jana Testová", "contact-form", "contact-crop-modal", "contactCropperInit", "contact-photo-input"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected edit page to contain %q", want)
		}
	}
}

func TestContactUpdate(t *testing.T) {
	handler, queries := newTestContactHandler(t)
	rec := httptest.NewRecorder()

	values := validContactValues()
	values.Set("phone", "0900 123 456")

	handler.Update(rec, contactForm(values))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "showToast") {
		t.Fatalf("expected toast trigger, got %q", trigger)
	}

	row, err := queries.GetContact(t.Context())
	if err != nil {
		t.Fatalf("get contact: %v", err)
	}

	if row.Phone != "+421900123456" {
		t.Fatalf("expected normalized phone, got %q", row.Phone)
	}
}

func TestContactUpdateValidation(t *testing.T) {
	handler, _ := newTestContactHandler(t)

	cases := []struct {
		name   string
		mutate func(url.Values)
		want   string
	}{
		{"empty name", func(v url.Values) { v.Set("name", "  ") }, "Vyplň meno."},
		{"empty role", func(v url.Values) { v.Set("role", "") }, "Vyplň pozíciu."},
		{"bad phone", func(v url.Values) { v.Set("phone", "0907") }, "Telefónne číslo nie je platné"},
		{"bad email", func(v url.Values) { v.Set("email", "kontakt@example") }, "E-mail nie je platný"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			values := validContactValues()
			tc.mutate(values)

			rec := httptest.NewRecorder()
			handler.Update(rec, contactForm(values))

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200 with inline error, got %d", rec.Code)
			}

			if body := rec.Body.String(); !strings.Contains(body, tc.want) {
				t.Fatalf("expected inline error %q in %q", tc.want, body)
			}
		})
	}
}

func TestContactUpdateNonHtmxInvalid(t *testing.T) {
	handler, _ := newTestContactHandler(t)

	values := validContactValues()
	values.Set("email", "preklep")

	req := httptest.NewRequest(http.MethodPut, "/admin/contact", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// photoUploadRequest builds a multipart PUT/DELETE photo request.
func photoUploadRequest(t *testing.T, method, filename string, content []byte) *http.Request {
	t.Helper()

	var buf bytes.Buffer

	w := multipart.NewWriter(&buf)

	part, err := w.CreateFormFile("photo", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}

	if _, err := part.Write(content); err != nil {
		t.Fatalf("write part: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(method, "/admin/contact/photo", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("HX-Request", "true")

	return req
}

// minimalPNG is a 1x1 transparent PNG.
var minimalPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
	0x42, 0x60, 0x82,
}

func TestContactUploadPhoto(t *testing.T) {
	handler, queries := newTestContactHandler(t)
	rec := httptest.NewRecorder()

	handler.UploadPhoto(rec, photoUploadRequest(t, http.MethodPut, "crop.png", minimalPNG))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if body := rec.Body.String(); !strings.Contains(body, "photoUrl") {
		t.Fatalf("expected photoUrl in %q", body)
	}

	row, err := queries.GetContact(t.Context())
	if err != nil {
		t.Fatalf("get contact: %v", err)
	}

	if !strings.HasPrefix(row.PhotoPath, "/uploads/manager-") || !strings.HasSuffix(row.PhotoPath, ".png") {
		t.Fatalf("expected content-hashed photo path, got %q", row.PhotoPath)
	}

	if _, err := os.Stat(filepath.Join(handler.photoDir, strings.TrimPrefix(row.PhotoPath, "/uploads/"))); err != nil {
		t.Fatalf("expected photo file to exist: %v", err)
	}

	// A re-upload of identical bytes maps to the same URL (cache stays valid).
	rec2 := httptest.NewRecorder()
	handler.UploadPhoto(rec2, photoUploadRequest(t, http.MethodPut, "crop.png", minimalPNG))

	row2, err := queries.GetContact(t.Context())
	if err != nil {
		t.Fatalf("get contact: %v", err)
	}

	if row2.PhotoPath != row.PhotoPath {
		t.Fatalf("expected stable path for identical bytes, got %q then %q", row.PhotoPath, row2.PhotoPath)
	}
}

func TestContactUploadPhotoReplacesOldFile(t *testing.T) {
	handler, queries := newTestContactHandler(t)

	// Pretend a previous upload exists.
	oldName := "manager-0123456789abcdef.webp"
	if err := os.WriteFile(filepath.Join(handler.photoDir, oldName), minimalPNG, 0o644); err != nil {
		t.Fatalf("seed photo file: %v", err)
	}

	if _, err := queries.UpdateContactPhoto(t.Context(), "/uploads/"+oldName); err != nil {
		t.Fatalf("seed photo path: %v", err)
	}

	rec := httptest.NewRecorder()
	handler.UploadPhoto(rec, photoUploadRequest(t, http.MethodPut, "crop.png", minimalPNG))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	row, err := queries.GetContact(t.Context())
	if err != nil {
		t.Fatalf("get contact: %v", err)
	}

	if row.PhotoPath == "/uploads/"+oldName {
		t.Fatalf("expected new photo path, got unchanged %q", row.PhotoPath)
	}

	if _, err := os.Stat(filepath.Join(handler.photoDir, oldName)); !os.IsNotExist(err) {
		t.Fatalf("expected old photo file removed, stat err = %v", err)
	}
}

func TestContactUploadPhotoRejectsNonImage(t *testing.T) {
	handler, _ := newTestContactHandler(t)
	rec := httptest.NewRecorder()

	handler.UploadPhoto(rec, photoUploadRequest(t, http.MethodPut, "note.txt", []byte("nie som obrazok")))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	if body := rec.Body.String(); !strings.Contains(body, "JPG") {
		t.Fatalf("expected image-type error in %q", body)
	}
}

func TestContactDeletePhoto(t *testing.T) {
	handler, queries := newTestContactHandler(t)

	// Pretend a photo exists first.
	name := "manager-0123456789abcdef.webp"
	if err := os.WriteFile(filepath.Join(handler.photoDir, name), minimalPNG, 0o644); err != nil {
		t.Fatalf("seed photo file: %v", err)
	}

	if _, err := queries.UpdateContactPhoto(t.Context(), "/uploads/"+name); err != nil {
		t.Fatalf("seed photo path: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/admin/contact/photo", nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	handler.DeletePhoto(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	row, err := queries.GetContact(t.Context())
	if err != nil {
		t.Fatalf("get contact: %v", err)
	}

	if row.PhotoPath != "" {
		t.Fatalf("expected empty photo path, got %q", row.PhotoPath)
	}

	if _, err := os.Stat(filepath.Join(handler.photoDir, name)); !os.IsNotExist(err) {
		t.Fatalf("expected photo file removed, stat err = %v", err)
	}
}

func TestContactGetContactFallsBackToSilhouette(t *testing.T) {
	handler, queries := newTestContactHandler(t)

	// DB points at a photo whose file does not exist (fresh clone).
	if _, err := queries.UpdateContactPhoto(t.Context(), "/uploads/manager-0123456789abcdef.webp"); err != nil {
		t.Fatalf("seed photo path: %v", err)
	}

	data, err := handler.GetContact(t.Context())
	if err != nil {
		t.Fatalf("get contact: %v", err)
	}

	if data.PhotoPath != "" {
		t.Fatalf("expected missing file to fall back to empty photo, got %q", data.PhotoPath)
	}

	if data.Name == "" {
		t.Fatal("expected other fields to survive the fallback")
	}
}
