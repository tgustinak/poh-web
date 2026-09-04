package admin

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"unicode"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	dbmigrations "github.com/tgustinak/pohkapela.sk/db/migrations"
	"github.com/tgustinak/pohkapela.sk/internal/auth"
	"github.com/tgustinak/pohkapela.sk/internal/db"
	"github.com/tgustinak/pohkapela.sk/internal/logbuffer"
)

func newTestHandler(t *testing.T) (*Handler, *auth.Service) {
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

	authSvc := auth.NewService(db.New(conn))

	if _, err := authSvc.EnsureAdminUser(t.Context(), "admin@example.com", "secret123"); err != nil {
		t.Fatalf("seed admin user: %v", err)
	}

	return NewHandler(authSvc, logbuffer.New(10)), authSvc
}

// testAnonCsrf is the anonymous CSRF token used by loginRequest.
const testAnonCsrf = "test-anon-csrf-token"

func loginRequest(t *testing.T, email, password string) *http.Request {
	t.Helper()

	form := url.Values{"email": {email}, "password": {password}, "csrf_token": {testAnonCsrf}}

	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.CsrfCookieName, Value: testAnonCsrf})

	return req
}

// htmxRequest marks a request as coming from htmx.
func htmxRequest(req *http.Request) *http.Request {
	req.Header.Set("HX-Request", "true")

	return req
}

func sessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == auth.CookieName {
			return cookie
		}
	}

	return nil
}

func TestLoginPageRendersForm(t *testing.T) {
	handler, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()

	handler.LoginPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `action="/admin/login"`) {
		t.Fatal("expected login form to post to /admin/login")
	}
}

func TestLoginPageRedirectsWhenAuthenticated(t *testing.T) {
	handler, authSvc := newTestHandler(t)

	token, _, err := authSvc.Login(t.Context(), "admin@example.com", "secret123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()

	handler.LoginPage(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rec.Code)
	}

	if location := rec.Header().Get("Location"); location != "/admin/dashboard" {
		t.Fatalf("expected redirect to /admin/dashboard, got %s", location)
	}
}

func TestLoginSuccessSetsCookie(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := httptest.NewRecorder()

	handler.Login(rec, loginRequest(t, "admin@example.com", "secret123"))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rec.Code)
	}

	cookie := sessionCookie(rec)
	if cookie == nil || cookie.Value == "" {
		t.Fatal("expected session cookie to be set")
	}

	if !cookie.HttpOnly {
		t.Fatal("expected session cookie to be HttpOnly")
	}
}

func TestLoginFailureRendersError(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := httptest.NewRecorder()

	handler.Login(rec, loginRequest(t, "admin@example.com", "wrong"))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	if cookie := sessionCookie(rec); cookie != nil && cookie.Value != "" {
		t.Fatal("expected no session cookie on failed login")
	}

	if !strings.Contains(rec.Body.String(), "Nesprávny e-mail alebo heslo.") {
		t.Fatal("expected error message in rendered form")
	}
}

func TestLogoutClearsSession(t *testing.T) {
	handler, authSvc := newTestHandler(t)

	token, _, err := authSvc.Login(t.Context(), "admin@example.com", "secret123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/logout", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()

	handler.Logout(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rec.Code)
	}

	if _, _, err := authSvc.Authenticate(t.Context(), token); err == nil {
		t.Fatal("expected session to be deleted after logout")
	}
}

func TestRequireAuthMiddleware(t *testing.T) {
	_, authSvc := newTestHandler(t)

	next := authSvc.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.UserFromRequest(r)
		if !ok || user.Email != "admin@example.com" {
			t.Error("expected user in request context")
		}

		w.WriteHeader(http.StatusOK)
	}))

	// No cookie -> redirect to login.
	rec := httptest.NewRecorder()
	next.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/dashboard", nil))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 without cookie, got %d", rec.Code)
	}

	if location := rec.Header().Get("Location"); location != "/admin" {
		t.Fatalf("expected redirect to /admin, got %s", location)
	}

	// Valid cookie -> reaches the handler.
	token, _, err := authSvc.Login(t.Context(), "admin@example.com", "secret123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec = httptest.NewRecorder()

	next.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid session, got %d", rec.Code)
	}
}

func TestLoginHtmxSuccessRedirects(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := httptest.NewRecorder()

	handler.Login(rec, htmxRequest(loginRequest(t, "admin@example.com", "secret123")))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if location := rec.Header().Get("HX-Redirect"); location != "/admin/dashboard" {
		t.Fatalf("expected HX-Redirect to /admin/dashboard, got %q", location)
	}

	if cookie := sessionCookie(rec); cookie == nil || cookie.Value == "" {
		t.Fatal("expected session cookie to be set")
	}
}

func TestLoginHtmxFailureTriggersToast(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := httptest.NewRecorder()

	handler.Login(rec, htmxRequest(loginRequest(t, "admin@example.com", "wrong")))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	trigger := rec.Header().Get("HX-Trigger")
	if !strings.Contains(trigger, "showToast") {
		t.Fatalf("expected HX-Trigger with showToast, got %q", trigger)
	}

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
		t.Fatalf("expected error message in HX-Trigger, got %q", payload["showToast"])
	}

	if rec.Body.String() != "" {
		t.Fatalf("expected empty body with hx-swap=none, got %q", rec.Body.String())
	}

	if cookie := sessionCookie(rec); cookie != nil && cookie.Value != "" {
		t.Fatal("expected no session cookie on failed login")
	}
}

func TestLogoutHtmxRedirects(t *testing.T) {
	handler, authSvc := newTestHandler(t)

	token, _, err := authSvc.Login(t.Context(), "admin@example.com", "secret123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/logout", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()

	handler.Logout(rec, htmxRequest(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if location := rec.Header().Get("HX-Redirect"); location != "/admin" {
		t.Fatalf("expected HX-Redirect to /admin, got %q", location)
	}

	if _, _, err := authSvc.Authenticate(t.Context(), token); err == nil {
		t.Fatal("expected session to be deleted after logout")
	}
}

func TestLoginCsrfMismatchRejected(t *testing.T) {
	handler, _ := newTestHandler(t)

	form := url.Values{"email": {"admin@example.com"}, "password": {"secret123"}, "csrf_token": {"wrong"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.CsrfCookieName, Value: testAnonCsrf})
	rec := httptest.NewRecorder()

	handler.Login(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}

	if cookie := sessionCookie(rec); cookie != nil && cookie.Value != "" {
		t.Fatal("expected no session cookie on CSRF failure")
	}
}

func TestLoginHtmxCsrfMismatchTriggersToast(t *testing.T) {
	handler, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(url.Values{}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.Login(rec, htmxRequest(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload map[string]string
	if err := json.Unmarshal([]byte(rec.Header().Get("HX-Trigger")), &payload); err != nil {
		t.Fatalf("decode HX-Trigger: %v", err)
	}

	if payload["showToast"] != csrfFailure {
		t.Fatalf("expected CSRF toast, got %q", payload["showToast"])
	}
}

func TestRequireCsrfMiddleware(t *testing.T) {
	_, authSvc := newTestHandler(t)

	token, _, err := authSvc.Login(t.Context(), "admin@example.com", "secret123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	session, _, err := authSvc.Authenticate(t.Context(), token)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	protected := authSvc.RequireAuth(auth.RequireCsrf(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	// POST with valid session but no CSRF token -> 403.
	req := httptest.NewRequest(http.MethodPost, "/admin/logout", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()

	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without CSRF token, got %d", rec.Code)
	}

	// POST with valid session and CSRF header -> passes through.
	req = httptest.NewRequest(http.MethodPost, "/admin/logout", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	req.Header.Set("X-CSRF-Token", session.CsrfToken)
	rec = httptest.NewRecorder()

	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with CSRF token, got %d", rec.Code)
	}

	// Safe methods pass without a token.
	req = httptest.NewRequest(http.MethodGet, "/admin/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec = httptest.NewRecorder()

	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET without CSRF token, got %d", rec.Code)
	}
}
