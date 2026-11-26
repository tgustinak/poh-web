package admin

import (
	"bytes"
	"database/sql"
	"image/jpeg"
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

	dbmigrations "github.com/tgustinak/poh-web/db/migrations"
	"github.com/tgustinak/poh-web/internal/db"
	"github.com/tgustinak/poh-web/internal/gallery"
)

func newTestGalleryHandler(t *testing.T) (*GalleryHandler, *db.Queries, string) {
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

	return NewGalleryHandler(queries, dir), queries, dir
}

func galleryVideoForm(method, target, youtubeURL, title string) *http.Request {
	form := url.Values{
		"youtube_url": {youtubeURL},
		"title":       {title},
	}

	req := httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	return req
}

func createGalleryPhoto(t *testing.T, handler *GalleryHandler, alt string) {
	t.Helper()

	req := multipartOrganizersRequest(t, http.MethodPost, "/admin/gallery/photos",
		map[string]uploadFile{"photo": {"gallery.jpg", seedJPEG(t)}},
		url.Values{"alt": {alt}}, nil)
	rec := httptest.NewRecorder()

	handler.CreatePhoto(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("create photo: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func createGalleryVideo(t *testing.T, handler *GalleryHandler, youtubeURL, title string) {
	t.Helper()

	rec := httptest.NewRecorder()
	handler.CreateVideo(rec, galleryVideoForm(http.MethodPost, "/admin/gallery/videos", youtubeURL, title))

	if rec.Code != http.StatusOK {
		t.Fatalf("create video: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGalleryListPageRendersForms(t *testing.T) {
	handler, _, _ := newTestGalleryHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/gallery", nil)
	rec := httptest.NewRecorder()

	handler.ListPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	for _, want := range []string{
		"Galéria",
		"Pridať fotku",
		"Pridať YouTube video",
		`hx-post="/admin/gallery/photos"`,
		`hx-post="/admin/gallery/videos"`,
		"Zatiaľ žiadne položky",
		"gallery-photo-input",
		"gallery-crop-modal",
		"gallery-crop-canvas",
		"gallery-crop-zoom",
		"galleryCropperInit",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected list page to contain %q", want)
		}
	}
}

func TestGalleryCreatePhotoValidation(t *testing.T) {
	handler, queries, _ := newTestGalleryHandler(t)

	// PDF bytes are not a valid photo.
	req := multipartOrganizersRequest(t, http.MethodPost, "/admin/gallery/photos",
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
	req = multipartOrganizersRequest(t, http.MethodPost, "/admin/gallery/photos",
		nil, url.Values{"alt": {"Popis"}}, nil)
	rec = httptest.NewRecorder()

	handler.CreatePhoto(rec, req)

	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "showToast") {
		t.Fatalf("expected missing-file toast, got %q", trigger)
	}

	items, err := queries.ListGalleryItemsOrdered(t.Context())
	if err != nil {
		t.Fatalf("list gallery items: %v", err)
	}

	if len(items) != 0 {
		t.Fatalf("expected no items after validation errors, got %d", len(items))
	}
}

func TestGalleryCreatePhotoSuccess(t *testing.T) {
	handler, queries, dir := newTestGalleryHandler(t)

	createGalleryPhoto(t, handler, "Prvá fotka")
	createGalleryPhoto(t, handler, "Druhá fotka")

	items, err := queries.ListGalleryItemsOrdered(t.Context())
	if err != nil {
		t.Fatalf("list gallery items: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	// New items go first in the shared order.
	first := items[0]
	if first.Alt != "Druhá fotka" {
		t.Fatalf("expected newest photo first, got %q", first.Alt)
	}

	if first.Kind != gallery.KindPhoto {
		t.Fatalf("expected photo kind, got %q", first.Kind)
	}

	if first.Position != 1 {
		t.Fatalf("expected position 1, got %d", first.Position)
	}

	if !strings.HasPrefix(first.FilePath, "/uploads/galeria-foto-") {
		t.Fatalf("expected content-hashed upload path, got %q", first.FilePath)
	}

	if first.PreviewPath == first.FilePath {
		t.Fatalf("expected a generated preview path, got %q", first.PreviewPath)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read upload dir: %v", err)
	}

	// Both rows share content-addressed main and preview files.
	if len(entries) != 2 {
		t.Fatalf("expected main + preview upload files, got %d", len(entries))
	}
}

func TestGalleryCreatePhotoCompressesAndDownscales(t *testing.T) {
	handler, queries, dir := newTestGalleryHandler(t)

	createGalleryPhoto(t, handler, "Veľká fotka")

	items, err := queries.ListGalleryItemsOrdered(t.Context())
	if err != nil {
		t.Fatalf("list gallery items: %v", err)
	}

	item := items[0]
	if !strings.HasSuffix(item.FilePath, ".jpg") || !strings.HasSuffix(item.PreviewPath, ".jpg") {
		t.Fatalf("expected processed jpeg paths, got %q / %q", item.FilePath, item.PreviewPath)
	}

	if item.PreviewPath == item.FilePath {
		t.Fatal("expected a dedicated preview file")
	}

	mainData, err := os.ReadFile(filepath.Join(dir, gallery.FileName(item.FilePath)))
	if err != nil {
		t.Fatalf("read stored main: %v", err)
	}

	previewData, err := os.ReadFile(filepath.Join(dir, gallery.FileName(item.PreviewPath)))
	if err != nil {
		t.Fatalf("read stored preview: %v", err)
	}

	src := seedJPEG(t)
	if len(mainData) >= len(src) {
		t.Fatalf("expected compressed main (%d) smaller than source (%d)", len(mainData), len(src))
	}

	if len(previewData) >= len(mainData) {
		t.Fatalf("expected preview (%d) smaller than main (%d)", len(previewData), len(mainData))
	}

	mainCfg, err := jpeg.DecodeConfig(bytes.NewReader(mainData))
	if err != nil {
		t.Fatalf("decode stored main: %v", err)
	}

	if mainCfg.Width != gallery.MainMaxWidth || mainCfg.Height != gallery.MainMaxWidth*3/4 {
		t.Fatalf("expected main %d×%d, got %d×%d", gallery.MainMaxWidth, gallery.MainMaxWidth*3/4, mainCfg.Width, mainCfg.Height)
	}

	previewCfg, err := jpeg.DecodeConfig(bytes.NewReader(previewData))
	if err != nil {
		t.Fatalf("decode stored preview: %v", err)
	}

	if previewCfg.Width != gallery.PreviewMaxWidth || previewCfg.Height != gallery.PreviewMaxWidth*3/4 {
		t.Fatalf("expected preview %d×%d, got %d×%d", gallery.PreviewMaxWidth, gallery.PreviewMaxWidth*3/4, previewCfg.Width, previewCfg.Height)
	}
}

func TestGalleryCreateVideoValidation(t *testing.T) {
	handler, queries, _ := newTestGalleryHandler(t)

	rec := httptest.NewRecorder()
	handler.CreateVideo(rec, galleryVideoForm(http.MethodPost, "/admin/gallery/videos", "https://example.com/nope", "Video"))

	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "showToast") {
		t.Fatalf("expected invalid URL toast, got %q", trigger)
	}

	rec = httptest.NewRecorder()
	handler.CreateVideo(rec, galleryVideoForm(http.MethodPost, "/admin/gallery/videos", "https://youtu.be/7Msu0l_5Pi0", "   "))

	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "showToast") {
		t.Fatalf("expected missing title toast, got %q", trigger)
	}

	items, err := queries.ListGalleryItemsOrdered(t.Context())
	if err != nil {
		t.Fatalf("list gallery items: %v", err)
	}

	if len(items) != 0 {
		t.Fatalf("expected no items after validation errors, got %d", len(items))
	}
}

func TestGalleryCreateVideoSuccess(t *testing.T) {
	handler, queries, _ := newTestGalleryHandler(t)

	createGalleryVideo(t, handler, "https://www.youtube.com/watch?v=7Msu0l_5Pi0", "Nechcem zabudnúť")

	items, err := queries.ListGalleryItemsOrdered(t.Context())
	if err != nil {
		t.Fatalf("list gallery items: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	item := items[0]
	if item.Kind != gallery.KindVideo {
		t.Fatalf("expected video kind, got %q", item.Kind)
	}

	if item.YoutubeID != "7Msu0l_5Pi0" {
		t.Fatalf("expected parsed YouTube ID, got %q", item.YoutubeID)
	}

	if item.Title != "Nechcem zabudnúť" {
		t.Fatalf("expected title passthrough, got %q", item.Title)
	}

	if item.FilePath != "" || item.PreviewPath != "" {
		t.Fatalf("expected no photo paths for video, got %q / %q", item.FilePath, item.PreviewPath)
	}

	if item.Position != 1 {
		t.Fatalf("expected position 1, got %d", item.Position)
	}
}

func TestGalleryMixedOrder(t *testing.T) {
	handler, queries, _ := newTestGalleryHandler(t)

	createGalleryPhoto(t, handler, "Fotka")
	createGalleryVideo(t, handler, "7Msu0l_5Pi0", "Video")

	items, err := queries.ListGalleryItemsOrdered(t.Context())
	if err != nil {
		t.Fatalf("list gallery items: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	if items[0].Kind != gallery.KindVideo || items[1].Kind != gallery.KindPhoto {
		t.Fatalf("expected video before photo, got %q then %q", items[0].Kind, items[1].Kind)
	}
}

func TestGalleryUpdatePhotoAlt(t *testing.T) {
	handler, queries, _ := newTestGalleryHandler(t)

	createGalleryPhoto(t, handler, "Starý popis")

	items, err := queries.ListGalleryItemsOrdered(t.Context())
	if err != nil {
		t.Fatalf("list gallery items: %v", err)
	}

	id := items[0].ID
	target := "/admin/gallery/items/" + strconv.FormatInt(id, 10)

	form := url.Values{"alt": {"Nový popis"}}
	req := httptest.NewRequest(http.MethodPut, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.SetPathValue("id", strconv.FormatInt(id, 10))
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	updated, err := queries.GetGalleryItemByID(t.Context(), id)
	if err != nil {
		t.Fatalf("load item: %v", err)
	}

	if updated.Alt != "Nový popis" {
		t.Fatalf("expected alt update, got %q", updated.Alt)
	}

	// Overlong alt must be rejected.
	form = url.Values{"alt": {strings.Repeat("a", gallery.MaxAltLen+1)}}
	req = httptest.NewRequest(http.MethodPut, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.SetPathValue("id", strconv.FormatInt(id, 10))
	rec = httptest.NewRecorder()

	handler.Update(rec, req)

	if body := rec.Body.String(); !strings.Contains(body, "Popis je príliš dlhý") {
		t.Fatalf("expected inline alt error, got %q", body)
	}
}

func TestGalleryUpdateVideo(t *testing.T) {
	handler, queries, _ := newTestGalleryHandler(t)

	createGalleryVideo(t, handler, "7Msu0l_5Pi0", "Starý názov")

	items, err := queries.ListGalleryItemsOrdered(t.Context())
	if err != nil {
		t.Fatalf("list gallery items: %v", err)
	}

	id := items[0].ID
	target := "/admin/gallery/items/" + strconv.FormatInt(id, 10)

	req := galleryVideoForm(http.MethodPut, target, "https://youtu.be/ylJqEmM-Sjo", "Nový názov")
	req.SetPathValue("id", strconv.FormatInt(id, 10))
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	updated, err := queries.GetGalleryItemByID(t.Context(), id)
	if err != nil {
		t.Fatalf("load item: %v", err)
	}

	if updated.YoutubeID != "ylJqEmM-Sjo" || updated.Title != "Nový názov" {
		t.Fatalf("expected video update, got %+v", updated)
	}
}

func TestGalleryDeletePhotoCleansUpload(t *testing.T) {
	handler, queries, dir := newTestGalleryHandler(t)

	createGalleryPhoto(t, handler, "Na zmazanie")

	items, err := queries.ListGalleryItemsOrdered(t.Context())
	if err != nil {
		t.Fatalf("list gallery items: %v", err)
	}

	del := httptest.NewRequest(http.MethodDelete, "/admin/gallery/items/x", nil)
	del.Header.Set("HX-Request", "true")
	del.SetPathValue("id", strconv.FormatInt(items[0].ID, 10))
	rec := httptest.NewRecorder()

	handler.Delete(rec, del)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	remaining, err := queries.ListGalleryItemsOrdered(t.Context())
	if err != nil {
		t.Fatalf("list gallery items: %v", err)
	}

	if len(remaining) != 0 {
		t.Fatalf("expected item deleted, got %d", len(remaining))
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read upload dir: %v", err)
	}

	if len(entries) != 0 {
		t.Fatalf("expected upload cleaned up, got %d files", len(entries))
	}
}

func TestGalleryDeleteKeepsSharedUpload(t *testing.T) {
	handler, queries, dir := newTestGalleryHandler(t)

	createGalleryPhoto(t, handler, "Prvá")
	createGalleryPhoto(t, handler, "Druhá")

	items, err := queries.ListGalleryItemsOrdered(t.Context())
	if err != nil {
		t.Fatalf("list gallery items: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read upload dir: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected deduplicated main + preview, got %d files", len(entries))
	}

	del := httptest.NewRequest(http.MethodDelete, "/admin/gallery/items/x", nil)
	del.Header.Set("HX-Request", "true")
	del.SetPathValue("id", strconv.FormatInt(items[0].ID, 10))
	rec := httptest.NewRecorder()

	handler.Delete(rec, del)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// The other item still references both content-addressed files.
	entries, err = os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read upload dir: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected shared uploads to survive, got %d files", len(entries))
	}

	remaining, err := queries.ListGalleryItemsOrdered(t.Context())
	if err != nil {
		t.Fatalf("list gallery items: %v", err)
	}

	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining item, got %d", len(remaining))
	}

	for _, path := range []string{remaining[0].FilePath, remaining[0].PreviewPath} {
		if _, err := os.Stat(filepath.Join(dir, gallery.FileName(path))); err != nil {
			t.Fatalf("expected shared file %q to exist: %v", path, err)
		}
	}

	// Deleting the last reference removes the files.
	del = httptest.NewRequest(http.MethodDelete, "/admin/gallery/items/x", nil)
	del.Header.Set("HX-Request", "true")
	del.SetPathValue("id", strconv.FormatInt(remaining[0].ID, 10))
	rec = httptest.NewRecorder()

	handler.Delete(rec, del)

	entries, err = os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read upload dir: %v", err)
	}

	if len(entries) != 0 {
		t.Fatalf("expected uploads cleaned up, got %d files", len(entries))
	}
}

func TestGalleryReorder(t *testing.T) {
	handler, queries, _ := newTestGalleryHandler(t)

	createGalleryPhoto(t, handler, "Prvá")
	createGalleryVideo(t, handler, "7Msu0l_5Pi0", "Druhá")
	createGalleryPhoto(t, handler, "Tretia")

	items, err := queries.ListGalleryItemsOrdered(t.Context())
	if err != nil {
		t.Fatalf("list gallery items: %v", err)
	}

	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	// Reverse the current order.
	form := url.Values{}
	for i := len(items) - 1; i >= 0; i-- {
		form.Add("id", strconv.FormatInt(items[i].ID, 10))
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/gallery/reorder", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	handler.Reorder(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	after, err := queries.ListGalleryItemsOrdered(t.Context())
	if err != nil {
		t.Fatalf("list gallery items: %v", err)
	}

	for i, want := range []int64{items[2].ID, items[1].ID, items[0].ID} {
		if after[i].ID != want {
			t.Fatalf("expected id %d at index %d, got %d", want, i, after[i].ID)
		}
	}

	// Incomplete reorder must be rejected atomically.
	form = url.Values{"id": {strconv.FormatInt(items[0].ID, 10)}}
	req = httptest.NewRequest(http.MethodPost, "/admin/gallery/reorder", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rec = httptest.NewRecorder()

	handler.Reorder(rec, req)

	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "showToast") {
		t.Fatalf("expected reorder error toast, got %q", trigger)
	}
}

func TestGalleryUnknownID404(t *testing.T) {
	handler, _, _ := newTestGalleryHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/gallery/items/999999", nil)
	req.Header.Set("HX-Request", "true")
	req.SetPathValue("id", "999999")
	rec := httptest.NewRecorder()

	handler.Row(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}
