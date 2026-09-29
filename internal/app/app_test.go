// Package app_test contains integration tests that boot the full
// application mux (routing, middleware, handlers, templates) against
// a real SQLite database and exercise it over HTTP, mirroring the
// manual end-to-end checks from development.
package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/tgustinak/pohkapela.sk/internal/admin"
	"github.com/tgustinak/pohkapela.sk/internal/app"
	"github.com/tgustinak/pohkapela.sk/internal/auth"
	"github.com/tgustinak/pohkapela.sk/internal/database"
	"github.com/tgustinak/pohkapela.sk/internal/db"
	"github.com/tgustinak/pohkapela.sk/internal/logbuffer"
)

const (
	testAdminEmail    = "admin@example.com"
	testAdminPassword = "secret123"
)

// testServer boots the full app mux with a fresh database
// and returns the server, its log buffer and the uploads directory.
func testServer(t *testing.T) (*httptest.Server, *logbuffer.Buffer, string) {
	t.Helper()

	ctx := context.Background()

	sqlDB, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	queries := db.New(sqlDB)
	authSvc := auth.NewService(queries)

	created, err := authSvc.EnsureAdminUser(ctx, testAdminEmail, testAdminPassword)
	if err != nil {
		t.Fatalf("seed admin user: %v", err)
	}

	if !created {
		t.Fatal("expected admin user to be created")
	}

	logs := logbuffer.New(10)

	uploadDir := t.TempDir()

	mux := app.NewMux(app.Deps{
		Auth:              authSvc,
		Queries:           queries,
		UploadDir:         uploadDir,
		AuthHandler:       admin.NewAuthHandler(authSvc),
		LogsHandler:       admin.NewLogsHandler(logs),
		ConcertsHandler:   admin.NewConcertsHandler(queries),
		VideosHandler:     admin.NewVideosHandler(queries),
		MusicHandler:      admin.NewMusicHandler(queries, uploadDir),
		AboutHandler:      admin.NewAboutHandler(queries),
		SocialHandler:     admin.NewSocialHandler(queries),
		OrganizersHandler: admin.NewOrganizersHandler(queries, uploadDir),
		ContactHandler:    admin.NewContactHandler(queries, uploadDir),
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server, logs, uploadDir
}

// testClient returns an HTTP client that stores cookies
// but does not follow redirects, so tests can assert on them.
func testClient(t *testing.T) *http.Client {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookie jar: %v", err)
	}

	return &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// loginCsrf fetches the login page (storing the anonymous CSRF cookie
// in the jar) and returns the token rendered in the form.
func loginCsrf(t *testing.T, client *http.Client, base string) string {
	t.Helper()

	resp, err := client.Get(base + "/admin")
	if err != nil {
		t.Fatalf("get login page: %v", err)
	}

	body := readBody(t, resp)

	const prefix = `name="csrf_token" value="`
	idx := strings.Index(body, prefix)
	if idx < 0 {
		t.Fatal("expected csrf_token field in login form")
	}

	rest := body[idx+len(prefix):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		t.Fatal("malformed csrf_token field")
	}

	return rest[:end]
}

// jarCookie returns a cookie value stored in the client jar.
func jarCookie(t *testing.T, client *http.Client, base, name string) string {
	t.Helper()

	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse base url: %v", err)
	}

	for _, cookie := range client.Jar.Cookies(u) {
		if cookie.Name == name {
			return cookie.Value
		}
	}

	t.Fatalf("expected %s cookie in jar", name)

	return ""
}

// postLogin submits the login form, optionally as an htmx request.
func postLogin(t *testing.T, client *http.Client, base string, email, password string, htmx bool) *http.Response {
	t.Helper()

	csrf := loginCsrf(t, client, base)

	req, err := http.NewRequest(
		http.MethodPost,
		base+"/admin/login",
		strings.NewReader(url.Values{"email": {email}, "password": {password}, "csrf_token": {csrf}}.Encode()),
	)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if htmx {
		req.Header.Set("HX-Request", "true")
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("post login: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	return resp
}

// postLogout submits the logout form with the session CSRF token.
func postLogout(t *testing.T, client *http.Client, base string, htmx bool) *http.Response {
	t.Helper()

	csrf := jarCookie(t, client, base, auth.CsrfCookieName)

	var req *http.Request
	var err error
	if htmx {
		req, err = http.NewRequest(http.MethodPost, base+"/admin/logout", nil)
		if err != nil {
			t.Fatalf("create request: %v", err)
		}
		req.Header.Set("HX-Request", "true")
		req.Header.Set(auth.CsrfHeaderName, csrf)
	} else {
		req, err = http.NewRequest(
			http.MethodPost,
			base+"/admin/logout",
			strings.NewReader(url.Values{"csrf_token": {csrf}}.Encode()),
		)
		if err != nil {
			t.Fatalf("create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("post logout: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	return string(body)
}

func assertRedirect(t *testing.T, resp *http.Response, target string) {
	t.Helper()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", resp.StatusCode)
	}

	if location := resp.Header.Get("Location"); location != target {
		t.Fatalf("expected redirect to %s, got %s", target, location)
	}
}

// TestAdminLoginFlow covers the classic (no-JS) login flow end to end:
// login page -> failed login -> successful login -> protected page ->
// logout -> protected page denied again.
func TestAdminLoginFlow(t *testing.T) {
	server, _, _ := testServer(t)
	client := testClient(t)

	resp, err := client.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get home: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected home 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp, err = client.Get(server.URL + "/admin")
	if err != nil {
		t.Fatalf("get login page: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected login 200, got %d", resp.StatusCode)
	}

	if body := readBody(t, resp); !strings.Contains(body, `hx-post="/admin/login"`) {
		t.Fatal("expected login form to use htmx")
	}

	resp, err = client.Get(server.URL + "/admin/dashboard")
	if err != nil {
		t.Fatalf("get dashboard: %v", err)
	}
	resp.Body.Close()
	assertRedirect(t, resp, "/admin")

	resp = postLogin(t, client, server.URL, testAdminEmail, "wrong", false)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected failed login 401, got %d", resp.StatusCode)
	}

	resp = postLogin(t, client, server.URL, testAdminEmail, testAdminPassword, false)
	assertRedirect(t, resp, "/admin/dashboard")

	resp, err = client.Get(server.URL + "/admin/dashboard")
	if err != nil {
		t.Fatalf("get dashboard after login: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected dashboard 200, got %d", resp.StatusCode)
	}

	if body := readBody(t, resp); !strings.Contains(body, "Prehľad") {
		t.Fatal("expected dashboard content")
	}

	resp = postLogout(t, client, server.URL, false)
	assertRedirect(t, resp, "/admin")

	resp, err = client.Get(server.URL + "/admin/dashboard")
	if err != nil {
		t.Fatalf("get dashboard after logout: %v", err)
	}
	resp.Body.Close()
	assertRedirect(t, resp, "/admin")
}

// TestAdminLoginFlowHtmx covers the htmx login flow end to end:
// toast trigger on failure, HX-Redirect on success and logout.
func TestAdminLoginFlowHtmx(t *testing.T) {
	server, _, _ := testServer(t)
	client := testClient(t)

	resp := postLogin(t, client, server.URL, testAdminEmail, "wrong", true)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected failed htmx login 200, got %d", resp.StatusCode)
	}

	trigger := resp.Header.Get("HX-Trigger")

	// The header must be pure ASCII (browsers decode headers as Latin-1,
	// so raw UTF-8 would render as mojibake in the toast).
	for _, r := range trigger {
		if r > unicode.MaxASCII {
			t.Fatalf("expected ASCII-only HX-Trigger header, got %q", trigger)
		}
	}

	var payload map[string]string
	if err := json.Unmarshal([]byte(trigger), &payload); err != nil {
		t.Fatalf("decode HX-Trigger: %v", err)
	}

	if payload["showToast"] != "Nesprávny e-mail alebo heslo." {
		t.Fatalf("expected toast trigger with error message, got %q", payload["showToast"])
	}

	resp = postLogin(t, client, server.URL, testAdminEmail, testAdminPassword, true)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected htmx login 200, got %d", resp.StatusCode)
	}

	if location := resp.Header.Get("HX-Redirect"); location != "/admin/dashboard" {
		t.Fatalf("expected HX-Redirect to /admin/dashboard, got %q", location)
	}

	resp, err := client.Get(server.URL + "/admin/dashboard")
	if err != nil {
		t.Fatalf("get dashboard after htmx login: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected dashboard 200, got %d", resp.StatusCode)
	}

	resp = postLogout(t, client, server.URL, true)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected htmx logout 200, got %d", resp.StatusCode)
	}

	if location := resp.Header.Get("HX-Redirect"); location != "/admin" {
		t.Fatalf("expected HX-Redirect to /admin, got %q", location)
	}

	resp, err = client.Get(server.URL + "/admin/dashboard")
	if err != nil {
		t.Fatalf("get dashboard after htmx logout: %v", err)
	}
	resp.Body.Close()
	assertRedirect(t, resp, "/admin")
}

func TestLogoutWithoutCsrfRejected(t *testing.T) {
	server, _, _ := testServer(t)
	client := testClient(t)

	resp := postLogin(t, client, server.URL, testAdminEmail, testAdminPassword, false)
	assertRedirect(t, resp, "/admin/dashboard")

	req, err := http.NewRequest(http.MethodPost, server.URL+"/admin/logout", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("post logout: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 without CSRF token, got %d", resp.StatusCode)
	}
}

func TestAdminLogsPage(t *testing.T) {
	server, logs, _ := testServer(t)
	client := testClient(t)

	resp, err := client.Get(server.URL + "/admin/logs")
	if err != nil {
		t.Fatalf("get logs: %v", err)
	}
	resp.Body.Close()
	assertRedirect(t, resp, "/admin")

	logs.Append(logbuffer.Entry{Level: "WARN", Message: "test warning"})

	resp = postLogin(t, client, server.URL, testAdminEmail, testAdminPassword, false)
	assertRedirect(t, resp, "/admin/dashboard")

	resp, err = client.Get(server.URL + "/admin/logs")
	if err != nil {
		t.Fatalf("get logs after login: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected logs 200, got %d", resp.StatusCode)
	}

	body := readBody(t, resp)
	// A single entry fits on one page, so no infinite-scroll sentinel
	// is rendered (pagination is covered by TestAdminLogsEntriesPagination).
	for _, want := range []string{"Logy", "test warning", "/admin/logs/entries"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected logs page to contain %q", want)
		}
	}
}

func TestAdminLogsEntriesPagination(t *testing.T) {
	server, logs, _ := testServer(t)
	client := testClient(t)

	resp := postLogin(t, client, server.URL, testAdminEmail, testAdminPassword, false)
	assertRedirect(t, resp, "/admin/dashboard")

	for i := 0; i < 5; i++ {
		logs.Append(logbuffer.Entry{Level: "ERROR", Message: "boom"})
	}

	getEntries := func(query string) string {
		t.Helper()

		resp, err := client.Get(server.URL + "/admin/logs/entries" + query)
		if err != nil {
			t.Fatalf("get entries %s: %v", query, err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected entries 200, got %d", resp.StatusCode)
		}

		return readBody(t, resp)
	}

	page1 := getEntries("?limit=2")
	if strings.Count(page1, "boom") != 2 {
		t.Fatalf("expected 2 rows on first page, got %q", page1)
	}

	before := entriesCursor(t, page1)

	page2 := getEntries("?limit=2&before=" + before)
	if strings.Count(page2, "boom") != 2 {
		t.Fatalf("expected 2 rows on second page, got %q", page2)
	}

	before2 := entriesCursor(t, page2)

	page3 := getEntries("?limit=2&before=" + before2)
	if strings.Count(page3, "boom") != 1 {
		t.Fatalf("expected 1 row on last page, got %q", page3)
	}

	if strings.Contains(page3, "log-more") {
		t.Fatalf("expected no sentinel on last page, got %q", page3)
	}

	if !strings.Contains(page3, "Koniec záznamov.") {
		t.Fatalf("expected end marker on last page, got %q", page3)
	}
}

func entriesCursor(t *testing.T, body string) string {
	t.Helper()

	const prefix = "before="
	idx := strings.Index(body, prefix)
	if idx < 0 {
		t.Fatalf("expected pagination cursor, got %q", body)
	}

	rest := body[idx+len(prefix):]
	end := strings.IndexFunc(rest, func(r rune) bool {
		return r < '0' || r > '9'
	})
	if end < 0 {
		end = len(rest)
	}

	return rest[:end]
}

func TestScriptsFlushedBeforeBodyEnd(t *testing.T) {
	server, _, _ := testServer(t)
	client := testClient(t)

	resp, err := client.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get home: %v", err)
	}

	body := readBody(t, resp)

	// Deferred page scripts must render grouped right before </body>,
	// not scattered between sections.
	lastSection := strings.LastIndex(body, "</section>")
	firstScript := strings.Index(body, "<script")
	if firstScript < 0 {
		t.Fatal("expected deferred scripts in home page")
	}

	if firstScript < lastSection {
		t.Fatal("expected all scripts after the last section")
	}

	for _, want := range []string{"past-accordion", "klipPlay", "merchShow", "pohConsentAccept", "sectionObserver", "albumScroll"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected flushed script %q in home page", want)
		}
	}
}

func TestCookieConsentBanner(t *testing.T) {
	server, _, _ := testServer(t)
	client := testClient(t)

	for path, name := range map[string]string{"/": "home", "/pre-organizatorov": "organizers"} {
		resp, err := client.Get(server.URL + path)
		if err != nil {
			t.Fatalf("get %s: %v", name, err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected %s 200, got %d", name, resp.StatusCode)
		}

		body := readBody(t, resp)
		for _, want := range []string{`id="cookie-consent"`, "poh-cookie-consent", "pohConsentReset", "cookie-banner-enter", "cookie-banner-leave"} {
			if !strings.Contains(body, want) {
				t.Fatalf("expected %s to contain %q", name, want)
			}
		}

		// Shared footer: exactly one <footer> element and no unrendered
		// templ expressions leaking into the HTML.
		if count := strings.Count(body, "<footer"); count != 1 {
			t.Fatalf("expected %s to have exactly one footer, got %d", name, count)
		}

		if strings.Contains(body, "@ConsentResetLink") {
			t.Fatalf("expected %s footer to render the cookies link", name)
		}
	}
}

func TestMediaEmbedsGatedBehindConsent(t *testing.T) {
	server, _, _ := testServer(t)
	client := testClient(t)

	resp, err := client.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get home: %v", err)
	}

	body := readBody(t, resp)

	// Players must not load until the visitor consents: no direct
	// iframe src (leading space excludes the data-media-src attribute),
	// URLs only in data attributes plus loader buttons.
	for _, direct := range []string{` src="https://open.spotify.com/embed`, ` src="https://www.youtube-nocookie.com/embed`} {
		if strings.Contains(body, direct) {
			t.Fatalf("expected no direct media iframe embed before consent (%q)", direct)
		}
	}

	for _, want := range []string{"data-media-embed", "data-media-src=\"https://open.spotify.com/embed", "pohLoadMediaOne", "data-yt-embed", "data-yt-placeholder", "klipEnsurePlayer"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected home to contain %q", want)
		}
	}
}

// TestAdminConcertsFlow covers concert management end to end:
// login -> create upcoming and past concerts -> both visible
// appropriately on the web -> delete -> gone everywhere.
func TestAdminConcertsFlow(t *testing.T) {
	server, _, _ := testServer(t)
	client := testClient(t)

	resp := postLogin(t, client, server.URL, testAdminEmail, testAdminPassword, false)
	assertRedirect(t, resp, "/admin/dashboard")

	future := time.Now().AddDate(0, 0, 30).Format("2006-01-02")
	past := time.Now().AddDate(0, 0, -30).Format("2006-01-02")

	createConcert := func(name, date string) {
		t.Helper()

		csrf := jarCookie(t, client, server.URL, auth.CsrfCookieName)
		form := url.Values{"name": {name}, "event_date": {date}, "csrf_token": {csrf}}

		req, err := http.NewRequest(http.MethodPost, server.URL+"/admin/concerts", strings.NewReader(form.Encode()))
		if err != nil {
			t.Fatalf("create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("post concert: %v", err)
		}
		resp.Body.Close()
		assertRedirect(t, resp, "/admin/concerts")
	}

	createConcert("Budúci koncert", future)
	createConcert("Minulý koncert", past)

	resp, err := client.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get home: %v", err)
	}

	body := readBody(t, resp)
	for _, want := range []string{"Budúci koncert", "Odohrané koncerty (1)", "Minulý koncert"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected home to contain %q", want)
		}
	}

	resp, err = client.Get(server.URL + "/admin/dashboard")
	if err != nil {
		t.Fatalf("get dashboard: %v", err)
	}

	if body := readBody(t, resp); !strings.Contains(body, "1 koncert") {
		t.Fatalf("expected dashboard concert count, got %q", body)
	}

	// Scrape the concert id from the admin list and delete it.
	resp, err = client.Get(server.URL + "/admin/concerts")
	if err != nil {
		t.Fatalf("get admin concerts: %v", err)
	}

	adminBody := readBody(t, resp)
	id := scrapeConcertID(t, adminBody)

	deleteReq, err := http.NewRequest(http.MethodDelete, server.URL+"/admin/concerts/"+id, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	deleteReq.Header.Set("HX-Request", "true")
	deleteReq.Header.Set(auth.CsrfHeaderName, jarCookie(t, client, server.URL, auth.CsrfCookieName))

	resp, err = client.Do(deleteReq)
	if err != nil {
		t.Fatalf("delete concert: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected delete 200, got %d", resp.StatusCode)
	}

	resp, err = client.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get home after delete: %v", err)
	}

	if body := readBody(t, resp); strings.Contains(body, "Budúci koncert") {
		t.Fatal("expected deleted concert to disappear from home")
	}
}

// TestHomeShowsSeededClips verifies the public clips section is DB-driven:
// seeded videos from the migration render on the home page.
func TestHomeShowsSeededClips(t *testing.T) {
	server, _, _ := testServer(t)
	client := testClient(t)

	resp, err := client.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get home: %v", err)
	}

	body := readBody(t, resp)
	for _, want := range []string{"Nechcem zabudnúť", "Kubín", "Všetky klipy", "klipPlay"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected home to contain %q", want)
		}
	}
}

// TestAdminVideosFlow covers videoclip management end to end:
// login -> admin list shows seed -> create clip -> visible on web ->
// reorder -> delete -> gone everywhere.
func TestAdminVideosFlow(t *testing.T) {
	server, _, _ := testServer(t)
	client := testClient(t)

	resp := postLogin(t, client, server.URL, testAdminEmail, testAdminPassword, false)
	assertRedirect(t, resp, "/admin/dashboard")

	resp, err := client.Get(server.URL + "/admin/videos")
	if err != nil {
		t.Fatalf("get admin videos: %v", err)
	}

	if body := readBody(t, resp); !strings.Contains(body, "Nechcem zabudnúť") {
		t.Fatal("expected admin videos to list seeded clips")
	}

	csrf := jarCookie(t, client, server.URL, auth.CsrfCookieName)
	form := url.Values{
		"youtube_url":    {"https://youtu.be/dQw4w9WgXcQ"},
		"title":          {"E2E klip"},
		"published_date": {"2026-02-02"},
		"duration":       {"3:33"},
		"csrf_token":     {csrf},
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/admin/videos", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("post video: %v", err)
	}
	resp.Body.Close()
	assertRedirect(t, resp, "/admin/videos")

	resp, err = client.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get home: %v", err)
	}

	if body := readBody(t, resp); !strings.Contains(body, "E2E klip") {
		t.Fatal("expected new clip to appear on home page")
	}

	resp, err = client.Get(server.URL + "/admin/videos")
	if err != nil {
		t.Fatalf("get admin videos: %v", err)
	}

	id := scrapeVideoID(t, readBody(t, resp), "E2E klip")

	deleteReq, err := http.NewRequest(http.MethodDelete, server.URL+"/admin/videos/"+id, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	deleteReq.Header.Set("HX-Request", "true")
	deleteReq.Header.Set(auth.CsrfHeaderName, jarCookie(t, client, server.URL, auth.CsrfCookieName))

	resp, err = client.Do(deleteReq)
	if err != nil {
		t.Fatalf("delete video: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected delete 200, got %d", resp.StatusCode)
	}

	resp, err = client.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get home after delete: %v", err)
	}

	if body := readBody(t, resp); strings.Contains(body, "E2E klip") {
		t.Fatal("expected deleted clip to disappear from home")
	}
}

func scrapeVideoID(t *testing.T, body, title string) string {
	t.Helper()

	// The delete button follows the title within the same row,
	// so anchor at the title to target the right clip.
	anchor := strings.Index(body, title)
	if anchor < 0 {
		t.Fatalf("expected clip %q in admin list", title)
	}

	const prefix = `hx-delete="/admin/videos/`
	idx := strings.Index(body[anchor:], prefix)
	if idx < 0 {
		t.Fatalf("expected delete button for %q", title)
	}

	rest := body[anchor+idx+len(prefix):]
	end := strings.IndexFunc(rest, func(r rune) bool {
		return r < '0' || r > '9'
	})
	if end <= 0 {
		t.Fatalf("expected numeric video id, got %q", body)
	}

	return rest[:end]
}

func scrapeConcertID(t *testing.T, body string) string {
	t.Helper()

	const prefix = `hx-delete="/admin/concerts/`
	idx := strings.Index(body, prefix)
	if idx < 0 {
		t.Fatalf("expected delete button, got %q", body)
	}

	rest := body[idx+len(prefix):]
	end := strings.IndexFunc(rest, func(r rune) bool {
		return r < '0' || r > '9'
	})
	if end <= 0 {
		t.Fatalf("expected numeric concert id, got %q", body)
	}

	return rest[:end]
}

// TestHomeShowsSeededAbout verifies the public about section is DB-driven:
// the seeded text from the migration renders on the home page.
func TestHomeShowsSeededAbout(t *testing.T) {
	server, _, _ := testServer(t)
	client := testClient(t)

	resp, err := client.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get home: %v", err)
	}

	body := readBody(t, resp)
	for _, want := range []string{"Dolného Kubína", "Na konci sveta", "kto to zažil, ten vie"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected home to contain %q", want)
		}
	}
}

// TestAdminAboutFlow covers about editing end to end:
// login -> admin form shows seed -> invalid update rejected ->
// valid htmx update visible on web and in admin -> plain update redirects.
func TestAdminAboutFlow(t *testing.T) {
	server, _, _ := testServer(t)
	client := testClient(t)

	resp := postLogin(t, client, server.URL, testAdminEmail, testAdminPassword, false)
	assertRedirect(t, resp, "/admin/dashboard")

	resp, err := client.Get(server.URL + "/admin/about")
	if err != nil {
		t.Fatalf("get admin about: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected about 200, got %d", resp.StatusCode)
	}

	if body := readBody(t, resp); !strings.Contains(body, `hx-put="/admin/about"`) {
		t.Fatal("expected about form to use htmx")
	}

	putAbout := func(p1, p2, quote string, useHtmx bool) *http.Response {
		t.Helper()

		csrf := jarCookie(t, client, server.URL, auth.CsrfCookieName)
		form := url.Values{"p1": {p1}, "p2": {p2}, "quote": {quote}}

		var req *http.Request
		var err error
		if useHtmx {
			req, err = http.NewRequest(http.MethodPut, server.URL+"/admin/about", strings.NewReader(form.Encode()))
			if err != nil {
				t.Fatalf("create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("HX-Request", "true")
			req.Header.Set(auth.CsrfHeaderName, csrf)
		} else {
			form.Set("csrf_token", csrf)
			req, err = http.NewRequest(http.MethodPut, server.URL+"/admin/about", strings.NewReader(form.Encode()))
			if err != nil {
				t.Fatalf("create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("put about: %v", err)
		}

		return resp
	}

	// Empty paragraph must be rejected with an inline error for htmx.
	resp = putAbout("", "Druhý", "Citát", true)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected htmx validation 200, got %d", resp.StatusCode)
	}

	if body := readBody(t, resp); !strings.Contains(body, "Vyplň prvý odstavec.") {
		t.Fatalf("expected inline validation error, got %q", body)
	}

	// Valid htmx update returns the form plus a toast.
	resp = putAbout("E2E odstavec jedna", "E2E odstavec dva", "E2E citát", true)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected update 200, got %d", resp.StatusCode)
	}

	if trigger := resp.Header.Get("HX-Trigger"); !strings.Contains(trigger, "showToast") {
		t.Fatalf("expected toast trigger, got %q", trigger)
	}
	resp.Body.Close()

	resp, err = client.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get home: %v", err)
	}

	if body := readBody(t, resp); !strings.Contains(body, "E2E odstavec jedna") {
		t.Fatal("expected updated about to appear on home page")
	}

	// Plain (no-JS) update redirects back to the edit page.
	resp = putAbout("Plain odstavec jedna", "Plain odstavec dva", "Plain citát", false)
	resp.Body.Close()
	assertRedirect(t, resp, "/admin/about")

	resp, err = client.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get home after plain update: %v", err)
	}

	if body := readBody(t, resp); !strings.Contains(body, "Plain odstavec jedna") {
		t.Fatal("expected plain update to persist on home page")
	}
}

// TestAdminContactFlow covers contact editing end to end:
// login -> admin form shows seed -> invalid update rejected ->
// valid htmx update visible on web.
func TestAdminContactFlow(t *testing.T) {
	server, _, _ := testServer(t)
	client := testClient(t)

	resp := postLogin(t, client, server.URL, testAdminEmail, testAdminPassword, false)
	assertRedirect(t, resp, "/admin/dashboard")

	resp, err := client.Get(server.URL + "/admin/contact")
	if err != nil {
		t.Fatalf("get admin contact: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected contact 200, got %d", resp.StatusCode)
	}

	if body := readBody(t, resp); !strings.Contains(body, `hx-put="/admin/contact"`) {
		t.Fatal("expected contact form to use htmx")
	}

	putContact := func(name, phone, email string) *http.Response {
		t.Helper()

		csrf := jarCookie(t, client, server.URL, auth.CsrfCookieName)
		form := url.Values{"name": {name}, "role": {"Manažérka"}, "phone": {phone}, "email": {email}}

		req, err := http.NewRequest(http.MethodPut, server.URL+"/admin/contact", strings.NewReader(form.Encode()))
		if err != nil {
			t.Fatalf("create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		req.Header.Set(auth.CsrfHeaderName, csrf)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("put contact: %v", err)
		}

		return resp
	}

	resp = putContact("E2E Manažérka", "0907", "kontakt@example.com")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected htmx validation 200, got %d", resp.StatusCode)
	}

	if body := readBody(t, resp); !strings.Contains(body, "Telefónne číslo nie je platné") {
		t.Fatalf("expected inline phone error, got %q", body)
	}

	resp = putContact("E2E Manažérka", "+421 907 111 222", "e2e@example.com")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected update 200, got %d", resp.StatusCode)
	}

	if trigger := resp.Header.Get("HX-Trigger"); !strings.Contains(trigger, "showToast") {
		t.Fatalf("expected toast trigger, got %q", trigger)
	}
	resp.Body.Close()

	resp, err = client.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get home: %v", err)
	}

	if body := readBody(t, resp); !strings.Contains(body, "E2E Manažérka") {
		t.Fatal("expected updated contact to appear on home page")
	}
}

// TestUploadsServeImmutable verifies uploaded files are served with a long
// immutable cache lifetime (file names carry a content hash) and that
// missing files 404.
func TestUploadsServeImmutable(t *testing.T) {
	server, _, uploadDir := testServer(t)
	client := testClient(t)

	name := "manager-0123456789abcdef.webp"
	content := []byte("fake-webp-content")

	if err := os.WriteFile(filepath.Join(uploadDir, name), content, 0o644); err != nil {
		t.Fatalf("seed upload: %v", err)
	}

	resp, err := client.Get(server.URL + "/uploads/" + name)
	if err != nil {
		t.Fatalf("get upload: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected upload 200, got %d", resp.StatusCode)
	}

	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("expected immutable cache header, got %q", cc)
	}

	if body := readBody(t, resp); body != string(content) {
		t.Fatalf("expected upload body, got %q", body)
	}

	resp, err = client.Get(server.URL + "/uploads/manager-0000000000000000.webp")
	if err != nil {
		t.Fatalf("get missing upload: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected missing upload 404, got %d", resp.StatusCode)
	}
}

// TestHomeShowsSeededContact verifies the public contact section is
// DB-driven: the seeded manager from the migration renders on the home page.
func TestHomeShowsSeededContact(t *testing.T) {
	server, _, _ := testServer(t)
	client := testClient(t)

	resp, err := client.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get home: %v", err)
	}

	body := readBody(t, resp)
	for _, want := range []string{"Jana Testová", "kontakt@example.com", "+421 900 123 456"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected home to contain %q", want)
		}
	}
}

// TestHomeShowsSeededSocial verifies the public social section is DB-driven:
// the seeded links from the migration render on the home page.
func TestHomeShowsSeededSocial(t *testing.T) {
	server, _, _ := testServer(t)
	client := testClient(t)

	resp, err := client.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get home: %v", err)
	}

	body := readBody(t, resp)
	for _, want := range []string{
		"kapelaPOH",
		"https://www.facebook.com/kapelaPOH/",
		"https://open.spotify.com/artist/79t3KrQngTRDooegKsR4cw",
		"Odoberať",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected home to contain %q", want)
		}
	}
}

// TestAdminSocialFlow covers social editing end to end:
// login -> admin list shows seed -> invalid update rejected ->
// valid htmx update visible on web and in admin -> plain update redirects.
func TestAdminSocialFlow(t *testing.T) {
	server, _, _ := testServer(t)
	client := testClient(t)

	resp := postLogin(t, client, server.URL, testAdminEmail, testAdminPassword, false)
	assertRedirect(t, resp, "/admin/dashboard")

	resp, err := client.Get(server.URL + "/admin/social")
	if err != nil {
		t.Fatalf("get admin social: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected social 200, got %d", resp.StatusCode)
	}

	if body := readBody(t, resp); !strings.Contains(body, `hx-get="/admin/social/facebook/edit"`) {
		t.Fatal("expected social rows to use htmx inline editing")
	}

	putSocial := func(platform, handle, linkURL string, useHtmx bool) *http.Response {
		t.Helper()

		csrf := jarCookie(t, client, server.URL, auth.CsrfCookieName)
		form := url.Values{"handle": {handle}, "url": {linkURL}}

		var req *http.Request
		var err error
		if useHtmx {
			req, err = http.NewRequest(http.MethodPut, server.URL+"/admin/social/"+platform, strings.NewReader(form.Encode()))
			if err != nil {
				t.Fatalf("create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("HX-Request", "true")
			req.Header.Set(auth.CsrfHeaderName, csrf)
		} else {
			form.Set("csrf_token", csrf)
			req, err = http.NewRequest(http.MethodPut, server.URL+"/admin/social/"+platform, strings.NewReader(form.Encode()))
			if err != nil {
				t.Fatalf("create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("put social: %v", err)
		}

		return resp
	}

	// Invalid URL must be rejected with an inline error for htmx.
	resp = putSocial("facebook", "kapelaPOH", "http://example.com/x", true)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected htmx validation 200, got %d", resp.StatusCode)
	}

	if body := readBody(t, resp); !strings.Contains(body, "URL musí začínať https://") {
		t.Fatalf("expected inline URL error, got %q", body)
	}

	// Valid htmx update returns the row plus a toast.
	resp = putSocial("facebook", "E2E handle", "https://www.facebook.com/e2e/", true)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected update 200, got %d", resp.StatusCode)
	}

	if trigger := resp.Header.Get("HX-Trigger"); !strings.Contains(trigger, "showToast") {
		t.Fatalf("expected toast trigger, got %q", trigger)
	}
	resp.Body.Close()

	resp, err = client.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get home: %v", err)
	}

	if body := readBody(t, resp); !strings.Contains(body, "E2E handle") {
		t.Fatal("expected updated social to appear on home page")
	}

	// Plain (no-JS) update redirects back to the list.
	resp = putSocial("facebook", "Plain handle", "https://www.facebook.com/plain/", false)
	resp.Body.Close()
	assertRedirect(t, resp, "/admin/social")

	resp, err = client.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get home after plain update: %v", err)
	}

	if body := readBody(t, resp); !strings.Contains(body, "Plain handle") {
		t.Fatal("expected plain update to persist on home page")
	}
}

// TestOrganizersPageShowsSeed verifies the organizers page is DB-driven:
// seeded documents, photos and the shared contact card render.
func TestOrganizersPageShowsSeed(t *testing.T) {
	server, _, _ := testServer(t)
	client := testClient(t)

	resp, err := client.Get(server.URL + "/pre-organizatorov")
	if err != nil {
		t.Fatalf("get organizers: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected organizers 200, got %d", resp.StatusCode)
	}

	body := readBody(t, resp)
	for _, want := range []string{
		"Pre organizátorov",
		"Jana Testová",
		"kontakt@example.com",
		"+421 900 123 456",
		"Technický rider",
		"/static/files/poh-rider.pdf",
		"poh-promo-1.png",
		"promo-1-preview",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected organizers page to contain %q", want)
		}
	}
}

// TestOrganizersContactSync verifies the organizers contact card follows
// the shared contact section: an update in /admin/contact appears on
// /pre-organizatorov without a second edit.
func TestOrganizersContactSync(t *testing.T) {
	server, _, _ := testServer(t)
	client := testClient(t)

	resp := postLogin(t, client, server.URL, testAdminEmail, testAdminPassword, false)
	assertRedirect(t, resp, "/admin/dashboard")

	csrf := jarCookie(t, client, server.URL, auth.CsrfCookieName)
	form := url.Values{
		"name":  {"E2E Organizátori"},
		"role":  {"Manažérka"},
		"phone": {"+421 907 111 222"},
		"email": {"e2e@example.com"},
	}

	req, err := http.NewRequest(http.MethodPut, server.URL+"/admin/contact", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.Header.Set(auth.CsrfHeaderName, csrf)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("put contact: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected contact update 200, got %d", resp.StatusCode)
	}

	resp, err = client.Get(server.URL + "/pre-organizatorov")
	if err != nil {
		t.Fatalf("get organizers: %v", err)
	}

	if body := readBody(t, resp); !strings.Contains(body, "E2E Organizátori") {
		t.Fatal("expected organizers page to follow shared contact")
	}
}

// organizerTestFile is one multipart file field for organizer uploads.
type organizerTestFile struct {
	filename string
	content  []byte
}

// organizersMultipart builds a multipart htmx file-upload request.
func organizersMultipart(
	t *testing.T,
	method, target string,
	files map[string]organizerTestFile,
	values url.Values,
	csrf string,
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

	req, err := http.NewRequest(method, target, &buf)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("HX-Request", "true")
	req.Header.Set(auth.CsrfHeaderName, csrf)

	return req
}

// TestAdminOrganizersDocumentsFlow covers document replacement end to end:
// login -> admin list shows seed -> non-PDF rejected ->
// valid htmx replacement visible on the public page.
func TestAdminOrganizersDocumentsFlow(t *testing.T) {
	server, _, _ := testServer(t)
	client := testClient(t)

	resp := postLogin(t, client, server.URL, testAdminEmail, testAdminPassword, false)
	assertRedirect(t, resp, "/admin/dashboard")

	resp, err := client.Get(server.URL + "/admin/organizers")
	if err != nil {
		t.Fatalf("get admin organizers: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected organizers 200, got %d", resp.StatusCode)
	}

	if body := readBody(t, resp); !strings.Contains(body, `hx-put="/admin/organizers/documents/rider"`) {
		t.Fatal("expected document replace forms to use htmx")
	}

	csrf := jarCookie(t, client, server.URL, auth.CsrfCookieName)

	jpeg, err := os.ReadFile(filepath.Join("..", "..", "web", "static", "files", "poh-promo-2.jpg"))
	if err != nil {
		t.Fatalf("read seed photo: %v", err)
	}

	// Non-PDF must be rejected with an inline error.
	req := organizersMultipart(t, http.MethodPut, server.URL+"/admin/organizers/documents/rider",
		map[string]organizerTestFile{"file": {"foto.jpg", jpeg}}, nil, csrf)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("put document: %v", err)
	}

	if body := readBody(t, resp); !strings.Contains(body, "Dokument musí byť PDF.") {
		t.Fatalf("expected inline PDF error, got %q", body)
	}

	// Valid replacement returns the row plus a toast.
	req = organizersMultipart(t, http.MethodPut, server.URL+"/admin/organizers/documents/rider",
		map[string]organizerTestFile{"file": {"rider.pdf", []byte("%PDF-1.4 E2E rider\n")}}, nil, csrf)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("put document: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected replace 200, got %d", resp.StatusCode)
	}

	if trigger := resp.Header.Get("HX-Trigger"); !strings.Contains(trigger, "showToast") {
		t.Fatalf("expected toast trigger, got %q", trigger)
	}
	resp.Body.Close()

	resp, err = client.Get(server.URL + "/pre-organizatorov")
	if err != nil {
		t.Fatalf("get organizers: %v", err)
	}

	if body := readBody(t, resp); !strings.Contains(body, "/uploads/organizatori-rider-") {
		t.Fatal("expected replaced document to appear on organizers page")
	}
}

// TestAdminOrganizersPhotosFlow covers promo photo management end to end:
// login -> create photo -> visible on web -> delete -> gone everywhere.
func TestAdminOrganizersPhotosFlow(t *testing.T) {
	server, _, _ := testServer(t)
	client := testClient(t)

	resp := postLogin(t, client, server.URL, testAdminEmail, testAdminPassword, false)
	assertRedirect(t, resp, "/admin/dashboard")

	csrf := jarCookie(t, client, server.URL, auth.CsrfCookieName)

	jpeg, err := os.ReadFile(filepath.Join("..", "..", "web", "static", "files", "poh-promo-2.jpg"))
	if err != nil {
		t.Fatalf("read seed photo: %v", err)
	}

	req := organizersMultipart(t, http.MethodPost, server.URL+"/admin/organizers/photos",
		map[string]organizerTestFile{"photo": {"e2e.jpg", jpeg}}, url.Values{"alt": {"E2E promo"}}, csrf)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("post photo: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected create 200, got %d", resp.StatusCode)
	}

	resp, err = client.Get(server.URL + "/pre-organizatorov")
	if err != nil {
		t.Fatalf("get organizers: %v", err)
	}

	if body := readBody(t, resp); !strings.Contains(body, "E2E promo") {
		t.Fatal("expected new photo to appear on organizers page")
	}

	resp, err = client.Get(server.URL + "/admin/organizers")
	if err != nil {
		t.Fatalf("get admin organizers: %v", err)
	}

	id := scrapePhotoID(t, readBody(t, resp), "E2E promo")

	deleteReq, err := http.NewRequest(http.MethodDelete, server.URL+"/admin/organizers/photos/"+id, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	deleteReq.Header.Set("HX-Request", "true")
	deleteReq.Header.Set(auth.CsrfHeaderName, jarCookie(t, client, server.URL, auth.CsrfCookieName))

	resp, err = client.Do(deleteReq)
	if err != nil {
		t.Fatalf("delete photo: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected delete 200, got %d", resp.StatusCode)
	}

	resp, err = client.Get(server.URL + "/pre-organizatorov")
	if err != nil {
		t.Fatalf("get organizers after delete: %v", err)
	}

	if body := readBody(t, resp); strings.Contains(body, "E2E promo") {
		t.Fatal("expected deleted photo to disappear from organizers page")
	}
}

func scrapePhotoID(t *testing.T, body, alt string) string {
	t.Helper()

	anchor := strings.Index(body, alt)
	if anchor < 0 {
		t.Fatalf("expected photo %q in admin list", alt)
	}

	const prefix = `hx-delete="/admin/organizers/photos/`
	idx := strings.Index(body[anchor:], prefix)
	if idx < 0 {
		t.Fatalf("expected delete button for %q", alt)
	}

	rest := body[anchor+idx+len(prefix):]
	end := strings.IndexFunc(rest, func(r rune) bool {
		return r < '0' || r > '9'
	})
	if end <= 0 {
		t.Fatalf("expected numeric photo id, got %q", body)
	}

	return rest[:end]
}

// TestHomeShowsSeededMusic verifies the public music section is DB-driven:
// seeded albums and the Spotify player render on the home page.
func TestHomeShowsSeededMusic(t *testing.T) {
	server, _, _ := testServer(t)
	client := testClient(t)

	resp, err := client.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get home: %v", err)
	}

	body := readBody(t, resp)
	for _, want := range []string{
		"Život",
		"Na konci sveta",
		"Obal albumu Život (2026)",
		"open.spotify.com/embed/artist/79t3KrQngTRDooegKsR4cw",
		"Prehrať na Spotify",
		`id="album-carousel"`,
		`aria-roledescription="carousel"`,
		`onclick="albumScroll(-1)"`,
		`onclick="albumScroll(1)"`,
		`album-fade-r`,
		`scroll-pt-3 scroll-px-1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected home to contain %q", want)
		}
	}
}

// TestAdminMusicFlow covers album management end to end:
// login -> create album with cover -> visible on web ->
// Social Spotify update rewrites player and links -> delete -> gone.
func TestAdminMusicFlow(t *testing.T) {
	server, _, _ := testServer(t)
	client := testClient(t)

	resp := postLogin(t, client, server.URL, testAdminEmail, testAdminPassword, false)
	assertRedirect(t, resp, "/admin/dashboard")

	resp, err := client.Get(server.URL + "/admin/music")
	if err != nil {
		t.Fatalf("get admin music: %v", err)
	}

	if body := readBody(t, resp); !strings.Contains(body, "album-create-form") {
		t.Fatal("expected album create form in admin")
	}

	csrf := jarCookie(t, client, server.URL, auth.CsrfCookieName)

	cover, err := os.ReadFile(filepath.Join("..", "..", "web", "static", "images", "album-na-konci-sveta.jpg"))
	if err != nil {
		t.Fatalf("read seed cover: %v", err)
	}

	req := organizersMultipart(t, http.MethodPost, server.URL+"/admin/music",
		map[string]organizerTestFile{"cover": {"obal.jpg", cover}},
		url.Values{"title": {"E2E album"}, "year": {"2025"}, "badge": {"E2E"}, "is_new": {"1"}},
		csrf)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("post album: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected create 200, got %d", resp.StatusCode)
	}

	resp, err = client.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get home: %v", err)
	}

	if body := readBody(t, resp); !strings.Contains(body, "E2E album") {
		t.Fatal("expected new album to appear on home page")
	}

	// Spotify URL in Social rewrites the music embed and all links.
	form := url.Values{
		"handle": {"P.O.H"},
		"url":    {"https://open.spotify.com/artist/dQw4w9WgXcQdQw4w9WgXcQ1"},
	}
	putReq, err := http.NewRequest(http.MethodPut, server.URL+"/admin/social/spotify", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	putReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	putReq.Header.Set("HX-Request", "true")
	putReq.Header.Set(auth.CsrfHeaderName, csrf)

	resp, err = client.Do(putReq)
	if err != nil {
		t.Fatalf("put social spotify: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected social update 200, got %d", resp.StatusCode)
	}

	resp, err = client.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get home: %v", err)
	}

	if body := readBody(t, resp); !strings.Contains(body, "open.spotify.com/embed/artist/dQw4w9WgXcQdQw4w9WgXcQ1") {
		t.Fatal("expected updated Spotify embed on home page")
	}

	// Scrape the album id from the admin list and delete it.
	resp, err = client.Get(server.URL + "/admin/music")
	if err != nil {
		t.Fatalf("get admin music: %v", err)
	}

	id := scrapeAlbumID(t, readBody(t, resp), "E2E album")

	deleteReq, err := http.NewRequest(http.MethodDelete, server.URL+"/admin/music/"+id, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	deleteReq.Header.Set("HX-Request", "true")
	deleteReq.Header.Set(auth.CsrfHeaderName, jarCookie(t, client, server.URL, auth.CsrfCookieName))

	resp, err = client.Do(deleteReq)
	if err != nil {
		t.Fatalf("delete album: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected delete 200, got %d", resp.StatusCode)
	}

	resp, err = client.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get home after delete: %v", err)
	}

	if body := readBody(t, resp); strings.Contains(body, "E2E album") {
		t.Fatal("expected deleted album to disappear from home page")
	}
}

func scrapeAlbumID(t *testing.T, body, title string) string {
	t.Helper()

	anchor := strings.Index(body, title)
	if anchor < 0 {
		t.Fatalf("expected album %q in admin list", title)
	}

	const prefix = `hx-delete="/admin/music/`
	idx := strings.Index(body[anchor:], prefix)
	if idx < 0 {
		t.Fatalf("expected delete button for %q", title)
	}

	rest := body[anchor+idx+len(prefix):]
	end := strings.IndexFunc(rest, func(r rune) bool {
		return r < '0' || r > '9'
	})
	if end <= 0 {
		t.Fatalf("expected numeric album id, got %q", body)
	}

	return rest[:end]
}
