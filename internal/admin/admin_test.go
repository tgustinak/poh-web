package admin

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	dbmigrations "github.com/tgustinak/pohkapela.sk/db/migrations"
	"github.com/tgustinak/pohkapela.sk/internal/auth"
	"github.com/tgustinak/pohkapela.sk/internal/db"
	"github.com/tgustinak/pohkapela.sk/internal/logbuffer"
)

func newTestHandlers(t *testing.T) (*AuthHandler, *LogsHandler, *auth.Service) {
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

	return NewAuthHandler(authSvc), NewLogsHandler(logbuffer.New(10)), authSvc
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
