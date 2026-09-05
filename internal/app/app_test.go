// Package app_test contains integration tests that boot the full
// application mux (routing, middleware, handlers, templates) against
// a real SQLite database and exercise it over HTTP, mirroring the
// manual end-to-end checks from development.
package app_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
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
// and returns the server plus its log buffer.
func testServer(t *testing.T) (*httptest.Server, *logbuffer.Buffer) {
	t.Helper()

	ctx := context.Background()

	sqlDB, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	authSvc := auth.NewService(db.New(sqlDB))

	created, err := authSvc.EnsureAdminUser(ctx, testAdminEmail, testAdminPassword)
	if err != nil {
		t.Fatalf("seed admin user: %v", err)
	}

	if !created {
		t.Fatal("expected admin user to be created")
	}

	logs := logbuffer.New(10)

	mux := app.NewMux(app.Deps{
		Auth:        authSvc,
		AuthHandler: admin.NewAuthHandler(authSvc),
		LogsHandler: admin.NewLogsHandler(logs),
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server, logs
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
	server, _ := testServer(t)
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
	server, _ := testServer(t)
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
	server, _ := testServer(t)
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
	server, logs := testServer(t)
	client := testClient(t)

	resp, err := client.Get(server.URL + "/admin/logy")
	if err != nil {
		t.Fatalf("get logs: %v", err)
	}
	resp.Body.Close()
	assertRedirect(t, resp, "/admin")

	logs.Append(logbuffer.Entry{Level: "WARN", Message: "test warning"})

	resp = postLogin(t, client, server.URL, testAdminEmail, testAdminPassword, false)
	assertRedirect(t, resp, "/admin/dashboard")

	resp, err = client.Get(server.URL + "/admin/logy")
	if err != nil {
		t.Fatalf("get logs after login: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected logs 200, got %d", resp.StatusCode)
	}

	body := readBody(t, resp)
	// A single entry fits on one page, so no infinite-scroll sentinel
	// is rendered (pagination is covered by TestAdminLogsEntriesPagination).
	for _, want := range []string{"Logy", "test warning", "/admin/logy/entries"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected logs page to contain %q", want)
		}
	}
}

func TestAdminLogsEntriesPagination(t *testing.T) {
	server, logs := testServer(t)
	client := testClient(t)

	resp := postLogin(t, client, server.URL, testAdminEmail, testAdminPassword, false)
	assertRedirect(t, resp, "/admin/dashboard")

	for i := 0; i < 5; i++ {
		logs.Append(logbuffer.Entry{Level: "ERROR", Message: "boom"})
	}

	getEntries := func(query string) string {
		t.Helper()

		resp, err := client.Get(server.URL + "/admin/logy/entries" + query)
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

func TestCookieConsentBanner(t *testing.T) {
	server, _ := testServer(t)
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
	server, _ := testServer(t)
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
