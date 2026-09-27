package admin

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	dbmigrations "github.com/tgustinak/pohkapela.sk/db/migrations"
	"github.com/tgustinak/pohkapela.sk/internal/db"
	"github.com/tgustinak/pohkapela.sk/internal/scripts"
)

func newTestVideosHandler(t *testing.T) (*VideosHandler, *db.Queries) {
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

	return NewVideosHandler(queries), queries
}

func seedVideo(t *testing.T, queries *db.Queries, youtubeID, title, date, duration string, position int64) db.Video {
	t.Helper()

	video, err := queries.CreateVideo(t.Context(), db.CreateVideoParams{
		YoutubeID:     youtubeID,
		Title:         title,
		PublishedDate: date,
		Duration:      duration,
		Position:      position,
	})
	if err != nil {
		t.Fatalf("seed video: %v", err)
	}

	return video
}

func videoForm(youtubeURL, title, date, duration string) *http.Request {
	form := url.Values{
		"youtube_url":    {youtubeURL},
		"title":          {title},
		"published_date": {date},
		"duration":       {duration},
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/videos", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	return req
}

func TestVideosListPageRendersSeed(t *testing.T) {
	handler, _ := newTestVideosHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/videos", nil)
	rec := httptest.NewRecorder()

	handler.ListPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	for _, want := range []string{"Nechcem zabudnúť", "Kubín", "Pridať videoklip", "video-create-form"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected list page to contain %q", want)
		}
	}
}

// TestVideosListPageIncludesReorderScript renders the list page the same way
// the production mux does (with a script collector in the context) and
// asserts the drag & drop script made it into the HTML. A Defer placed
// outside AdminLayout would silently drop the script (Flush already ran).
func TestVideosListPageIncludesReorderScript(t *testing.T) {
	handler, _ := newTestVideosHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/videos", nil)
	ctx, _ := scripts.WithCollector(req.Context())
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ListPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if body := rec.Body.String(); !strings.Contains(body, "videoReorderInit") {
		t.Fatal("expected drag & drop reorder script in list page HTML")
	}
}

func TestVideosCreate(t *testing.T) {
	handler, queries := newTestVideosHandler(t)
	rec := httptest.NewRecorder()

	before, err := queries.CountVideos(t.Context())
	if err != nil {
		t.Fatalf("count videos: %v", err)
	}

	handler.Create(rec, videoForm("https://www.youtube.com/watch?v=dQw4w9WgXcQ", "Nový klip", "2026-01-15", "3:33"))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if toastMessage(t, rec) != "Videoklip pridaný." {
		t.Fatalf("expected success toast, got %q", rec.Header().Get("HX-Trigger"))
	}

	if formReset(t, rec) != "video-create-form" {
		t.Fatalf("expected form reset trigger, got %q", rec.Header().Get("HX-Trigger"))
	}

	after, err := queries.CountVideos(t.Context())
	if err != nil {
		t.Fatalf("count videos: %v", err)
	}

	if after != before+1 {
		t.Fatalf("expected one new video, before=%d after=%d", before, after)
	}

	ordered, err := queries.ListVideosOrdered(t.Context())
	if err != nil {
		t.Fatalf("list videos: %v", err)
	}

	if len(ordered) == 0 || ordered[0].Title != "Nový klip" {
		t.Fatalf("expected new clip first in playlist, got %+v", ordered)
	}

	if ordered[0].Position != 1 {
		t.Fatalf("expected new clip at position 1, got %d", ordered[0].Position)
	}
}

func TestVideosCreateValidation(t *testing.T) {
	handler, queries := newTestVideosHandler(t)

	before, err := queries.CountVideos(t.Context())
	if err != nil {
		t.Fatalf("count videos: %v", err)
	}

	for name, form := range map[string]*http.Request{
		"bad url":      videoForm("nie-url", "X", "2026-01-15", "3:33"),
		"empty title":  videoForm("https://youtu.be/dQw4w9WgXcQ", "", "2026-01-15", "3:33"),
		"bad date":     videoForm("https://youtu.be/dQw4w9WgXcQ", "X", "nie-dátum", "3:33"),
		"bad duration": videoForm("https://youtu.be/dQw4w9WgXcQ", "X", "2026-01-15", "zle"),
	} {
		rec := httptest.NewRecorder()

		handler.Create(rec, form)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s: expected 200 with toast, got %d", name, rec.Code)
		}

		if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "showToast") {
			t.Fatalf("%s: expected error toast, got %q", name, trigger)
		}
	}

	after, err := queries.CountVideos(t.Context())
	if err != nil {
		t.Fatalf("count videos: %v", err)
	}

	if after != before {
		t.Fatalf("expected no new videos after invalid creates, before=%d after=%d", before, after)
	}
}

func TestVideosCreateDuplicate(t *testing.T) {
	handler, _ := newTestVideosHandler(t)
	rec := httptest.NewRecorder()

	// Seeded video from migration.
	handler.Create(rec, videoForm("7Msu0l_5Pi0", "Duplikát", "2026-01-15", "3:33"))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "showToast") {
		t.Fatalf("expected error toast for duplicate, got %q", trigger)
	}

	if body := rec.Body.String(); strings.Contains(body, "Duplikát") {
		t.Fatal("duplicate video must not be rendered in list")
	}
}

func TestVideosRowAndEditForm(t *testing.T) {
	handler, queries := newTestVideosHandler(t)
	video := seedVideo(t, queries, "dQw4w9WgXcQ", "Na úpravu", "2026-02-01", "3:33", 99)
	id := strconv.FormatInt(video.ID, 10)

	rowReq := httptest.NewRequest(http.MethodGet, "/admin/videos/"+id, nil)
	rowReq.Header.Set("HX-Request", "true")
	rowReq.SetPathValue("id", id)
	rec := httptest.NewRecorder()

	handler.Row(rec, rowReq)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected row 200, got %d", rec.Code)
	}

	if body := rec.Body.String(); !strings.Contains(body, "Na úpravu") {
		t.Fatal("expected row to contain video title")
	}

	editReq := httptest.NewRequest(http.MethodGet, "/admin/videos/"+id+"/edit", nil)
	editReq.Header.Set("HX-Request", "true")
	editReq.SetPathValue("id", id)
	rec = httptest.NewRecorder()

	handler.EditForm(rec, editReq)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected edit form 200, got %d", rec.Code)
	}

	if body := rec.Body.String(); !strings.Contains(body, "Na úpravu") {
		t.Fatalf("expected edit form with values, got %q", body)
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/admin/videos/999999", nil)
	missingReq.Header.Set("HX-Request", "true")
	missingReq.SetPathValue("id", "999999")
	rec = httptest.NewRecorder()

	handler.Row(rec, missingReq)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown video, got %d", rec.Code)
	}
}

func TestVideosUpdate(t *testing.T) {
	handler, queries := newTestVideosHandler(t)
	video := seedVideo(t, queries, "dQw4w9WgXcQ", "Starý názov", "2026-02-01", "3:33", 99)
	id := strconv.FormatInt(video.ID, 10)

	form := url.Values{
		"youtube_url":    {"https://youtu.be/eY52Zsg-KVI"},
		"title":          {"Nový názov"},
		"published_date": {"2026-03-01"},
		"duration":       {"4:44"},
	}
	req := httptest.NewRequest(http.MethodPut, "/admin/videos/"+id, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if toastMessage(t, rec) != "Klip uložený." {
		t.Fatalf("expected success toast, got %q", rec.Header().Get("HX-Trigger"))
	}

	updated, err := queries.GetVideoByID(t.Context(), video.ID)
	if err != nil {
		t.Fatalf("find video: %v", err)
	}

	if updated.Title != "Nový názov" || updated.YoutubeID != "eY52Zsg-KVI" {
		t.Fatalf("expected updated video in database, got %+v", updated)
	}
}

func TestVideosDelete(t *testing.T) {
	handler, queries := newTestVideosHandler(t)
	video := seedVideo(t, queries, "dQw4w9WgXcQ", "Na zmazanie", "2026-02-01", "3:33", 99)
	id := strconv.FormatInt(video.ID, 10)

	req := httptest.NewRequest(http.MethodDelete, "/admin/videos/"+id, nil)
	req.Header.Set("HX-Request", "true")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()

	handler.Delete(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if toastMessage(t, rec) != "Klip zmazaný." {
		t.Fatalf("expected delete toast, got %q", rec.Header().Get("HX-Trigger"))
	}

	if _, err := queries.GetVideoByID(t.Context(), video.ID); err != sql.ErrNoRows {
		t.Fatalf("expected video deleted, got %v", err)
	}
}

func TestVideosReorder(t *testing.T) {
	handler, queries := newTestVideosHandler(t)

	rows, err := queries.ListVideosOrdered(t.Context())
	if err != nil {
		t.Fatalf("list videos: %v", err)
	}

	if len(rows) < 2 {
		t.Fatalf("expected seeded videos, got %d", len(rows))
	}

	// Reverse the seeded order.
	reversed := make([]string, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		reversed = append(reversed, strconv.FormatInt(rows[i].ID, 10))
	}

	form := url.Values{"id": reversed}
	req := httptest.NewRequest(http.MethodPost, "/admin/videos/reorder", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	handler.Reorder(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	after, err := queries.ListVideosOrdered(t.Context())
	if err != nil {
		t.Fatalf("list videos: %v", err)
	}

	for i, v := range after {
		want, _ := strconv.ParseInt(reversed[i], 10, 64)
		if v.ID != want {
			t.Fatalf("position %d: expected video %d, got %d", i+1, want, v.ID)
		}

		if v.Position != int64(i+1) {
			t.Fatalf("expected position %d, got %d", i+1, v.Position)
		}
	}
}

func TestVideosReorderValidation(t *testing.T) {
	handler, queries := newTestVideosHandler(t)

	rows, err := queries.ListVideosOrdered(t.Context())
	if err != nil {
		t.Fatalf("list videos: %v", err)
	}

	// Missing one id must fail atomically.
	partial := url.Values{"id": {strconv.FormatInt(rows[0].ID, 10)}}
	req := httptest.NewRequest(http.MethodPost, "/admin/videos/reorder", strings.NewReader(partial.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	handler.Reorder(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with toast, got %d", rec.Code)
	}

	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "showToast") {
		t.Fatalf("expected error toast, got %q", trigger)
	}
}
