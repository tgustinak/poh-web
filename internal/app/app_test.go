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
)

const (
	testAdminEmail    = "admin@example.com"
	testAdminPassword = "secret123"
)

// testServer boots the full app mux with a fresh database.
func testServer(t *testing.T) *httptest.Server {
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

	mux := app.NewMux(app.Deps{
		Auth:         authSvc,
		AdminHandler: admin.NewHandler(authSvc),
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
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

// postLogin submits the login form, optionally as an htmx request.
func postLogin(t *testing.T, client *http.Client, base string, email, password string, htmx bool) *http.Response {
	t.Helper()

	req, err := http.NewRequest(
		http.MethodPost,
		base+"/admin/login",
		strings.NewReader(url.Values{"email": {email}, "password": {password}}.Encode()),
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
	server := testServer(t)
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

	req, err := http.NewRequest(http.MethodPost, server.URL+"/admin/logout", nil)
	if err != nil {
		t.Fatalf("create logout request: %v", err)
	}

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("post logout: %v", err)
	}
	resp.Body.Close()
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
	server := testServer(t)
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

	req, err := http.NewRequest(http.MethodPost, server.URL+"/admin/logout", nil)
	if err != nil {
		t.Fatalf("create logout request: %v", err)
	}
	req.Header.Set("HX-Request", "true")

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("htmx logout: %v", err)
	}
	resp.Body.Close()

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
