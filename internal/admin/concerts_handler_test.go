package admin

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	dbmigrations "github.com/tgustinak/pohkapela.sk/db/migrations"
	"github.com/tgustinak/pohkapela.sk/internal/db"
)

func newTestConcertsHandler(t *testing.T) (*ConcertsHandler, *db.Queries) {
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

	return NewConcertsHandler(queries), queries
}

func futureDate(days int) string {
	return time.Now().AddDate(0, 0, days).Format("2006-01-02")
}

func toastMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()

	var payload map[string]string
	if err := json.Unmarshal([]byte(rec.Header().Get("HX-Trigger")), &payload); err != nil {
		t.Fatalf("decode HX-Trigger: %v", err)
	}

	return payload["showToast"]
}

func pastDate(days int) string {
	return time.Now().AddDate(0, 0, -days).Format("2006-01-02")
}

func seedConcert(t *testing.T, queries *db.Queries, name, date string) db.Concert {
	t.Helper()

	concert, err := queries.CreateConcert(t.Context(), db.CreateConcertParams{
		Name:      name,
		EventDate: date,
	})
	if err != nil {
		t.Fatalf("seed concert: %v", err)
	}

	return concert
}

func concertForm(name, date string) *http.Request {
	form := url.Values{"name": {name}, "event_date": {date}}

	req := httptest.NewRequest(http.MethodPost, "/admin/concerts", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	return req
}

func TestConcertsListPageRenders(t *testing.T) {
	handler, queries := newTestConcertsHandler(t)
	seedConcert(t, queries, "Budúci koncert", futureDate(10))
	seedConcert(t, queries, "Minulý koncert", pastDate(10))

	req := httptest.NewRequest(http.MethodGet, "/admin/concerts", nil)
	rec := httptest.NewRecorder()

	handler.ListPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	for _, want := range []string{"Budúci koncert", "Minulý koncert", "Nadchádzajúce", "Odohrané", "Pridať koncert"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected list page to contain %q", want)
		}
	}
}

func TestConcertsCreate(t *testing.T) {
	handler, queries := newTestConcertsHandler(t)
	rec := httptest.NewRecorder()

	handler.Create(rec, concertForm("Nový koncert", futureDate(5)))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if toastMessage(t, rec) != "Koncert pridaný." {
		t.Fatalf("expected success toast, got %q", rec.Header().Get("HX-Trigger"))
	}

	if body := rec.Body.String(); !strings.Contains(body, "Nový koncert") {
		t.Fatal("expected refreshed tables to contain the new concert")
	}

	upcoming, err := queries.ListUpcomingConcerts(t.Context(), futureDate(-365))
	if err != nil {
		t.Fatalf("list concerts: %v", err)
	}

	if len(upcoming) != 1 || upcoming[0].Name != "Nový koncert" {
		t.Fatalf("expected concert in database, got %+v", upcoming)
	}
}

func TestConcertsCreateClassicFallback(t *testing.T) {
	handler, _ := newTestConcertsHandler(t)

	form := url.Values{"name": {"Klasika"}, "event_date": {futureDate(5)}}
	req := httptest.NewRequest(http.MethodPost, "/admin/concerts", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rec.Code)
	}

	if location := rec.Header().Get("Location"); location != "/admin/concerts" {
		t.Fatalf("expected redirect to /admin/concerts, got %s", location)
	}
}

func TestConcertsCreateValidation(t *testing.T) {
	handler, queries := newTestConcertsHandler(t)

	for name, form := range map[string]*http.Request{
		"empty name": concertForm("", futureDate(5)),
		"bad date":   concertForm("X", "nie-dátum"),
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

	count, err := queries.CountUpcomingConcerts(t.Context(), pastDate(365))
	if err != nil {
		t.Fatalf("count concerts: %v", err)
	}

	if count != 0 {
		t.Fatalf("expected no concerts after invalid creates, got %d", count)
	}
}

func TestConcertsRowAndEditForm(t *testing.T) {
	handler, queries := newTestConcertsHandler(t)
	concert := seedConcert(t, queries, "Na úpravu", futureDate(7))

	rowReq := httptest.NewRequest(http.MethodGet, "/admin/concerts/"+strconv.FormatInt(concert.ID, 10), nil)
	rowReq.Header.Set("HX-Request", "true")
	rowReq.SetPathValue("id", strconv.FormatInt(concert.ID, 10))
	rec := httptest.NewRecorder()

	handler.Row(rec, rowReq)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected row 200, got %d", rec.Code)
	}

	if body := rec.Body.String(); !strings.Contains(body, "Na úpravu") {
		t.Fatal("expected row to contain concert name")
	}

	editReq := httptest.NewRequest(http.MethodGet, "/admin/concerts/"+strconv.FormatInt(concert.ID, 10)+"/edit", nil)
	editReq.Header.Set("HX-Request", "true")
	editReq.SetPathValue("id", strconv.FormatInt(concert.ID, 10))
	rec = httptest.NewRecorder()

	handler.EditForm(rec, editReq)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected edit form 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `type="date"`) || !strings.Contains(body, "Na úpravu") {
		t.Fatalf("expected edit form with values, got %q", body)
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/admin/concerts/9999", nil)
	missingReq.Header.Set("HX-Request", "true")
	missingReq.SetPathValue("id", "9999")
	rec = httptest.NewRecorder()

	handler.Row(rec, missingReq)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown concert, got %d", rec.Code)
	}
}

func TestConcertsUpdate(t *testing.T) {
	handler, queries := newTestConcertsHandler(t)
	concert := seedConcert(t, queries, "Starý názov", futureDate(7))

	form := url.Values{"name": {"Nový názov"}, "event_date": {futureDate(8)}}
	req := httptest.NewRequest(http.MethodPut, "/admin/concerts/"+strconv.FormatInt(concert.ID, 10), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.SetPathValue("id", strconv.FormatInt(concert.ID, 10))
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if toastMessage(t, rec) != "Koncert uložený." {
		t.Fatalf("expected success toast, got %q", rec.Header().Get("HX-Trigger"))
	}

	if body := rec.Body.String(); !strings.Contains(body, "Nový názov") {
		t.Fatal("expected updated row in response")
	}

	updated, err := queries.GetConcertByID(t.Context(), concert.ID)
	if err != nil {
		t.Fatalf("find concert: %v", err)
	}

	if updated.Name != "Nový názov" {
		t.Fatalf("expected updated name in database, got %+v", updated)
	}
}

func TestConcertsUpdateValidation(t *testing.T) {
	handler, queries := newTestConcertsHandler(t)
	concert := seedConcert(t, queries, "Pôvodný", futureDate(7))

	form := url.Values{"name": {""}, "event_date": {futureDate(8)}}
	req := httptest.NewRequest(http.MethodPut, "/admin/concerts/"+strconv.FormatInt(concert.ID, 10), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.SetPathValue("id", strconv.FormatInt(concert.ID, 10))
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with inline error, got %d", rec.Code)
	}

	if body := rec.Body.String(); !strings.Contains(body, "Zadaj názov koncertu.") {
		t.Fatalf("expected inline validation error, got %q", body)
	}

	unchanged, err := queries.GetConcertByID(t.Context(), concert.ID)
	if err != nil {
		t.Fatalf("find concert: %v", err)
	}

	if unchanged.Name != "Pôvodný" {
		t.Fatalf("expected unchanged concert, got %+v", unchanged)
	}
}

func TestConcertsDelete(t *testing.T) {
	handler, queries := newTestConcertsHandler(t)
	concert := seedConcert(t, queries, "Na zmazanie", futureDate(7))

	req := httptest.NewRequest(http.MethodDelete, "/admin/concerts/"+strconv.FormatInt(concert.ID, 10), nil)
	req.Header.Set("HX-Request", "true")
	req.SetPathValue("id", strconv.FormatInt(concert.ID, 10))
	rec := httptest.NewRecorder()

	handler.Delete(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if toastMessage(t, rec) != "Koncert zmazaný." {
		t.Fatalf("expected delete toast, got %q", rec.Header().Get("HX-Trigger"))
	}

	if _, err := queries.GetConcertByID(t.Context(), concert.ID); err != sql.ErrNoRows {
		t.Fatalf("expected concert deleted, got %v", err)
	}

	missingReq := httptest.NewRequest(http.MethodDelete, "/admin/concerts/9999", nil)
	missingReq.Header.Set("HX-Request", "true")
	missingReq.SetPathValue("id", "9999")
	rec = httptest.NewRecorder()

	handler.Delete(rec, missingReq)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown concert, got %d", rec.Code)
	}
}
