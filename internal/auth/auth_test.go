package auth

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	dbmigrations "github.com/tgustinak/pohkapela.sk/db/migrations"
	"github.com/tgustinak/pohkapela.sk/internal/db"
)

func openTestDB(t *testing.T) *sql.DB {
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

	return conn
}

func newTestService(t *testing.T) *Service {
	t.Helper()

	return NewService(db.New(openTestDB(t)))
}

func seedTestUser(t *testing.T, svc *Service) {
	t.Helper()

	created, err := svc.EnsureAdminUser(t.Context(), "admin@example.com", "secret123")
	if err != nil {
		t.Fatalf("seed admin user: %v", err)
	}

	if !created {
		t.Fatal("expected admin user to be created")
	}
}

func TestEnsureAdminUserOnlySeedsOnce(t *testing.T) {
	svc := newTestService(t)
	ctx := t.Context()

	created, err := svc.EnsureAdminUser(ctx, "admin@example.com", "secret123")
	if err != nil {
		t.Fatalf("first seed: %v", err)
	}

	if !created {
		t.Fatal("expected first seed to create a user")
	}

	created, err = svc.EnsureAdminUser(ctx, "other@example.com", "other-secret")
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}

	if created {
		t.Fatal("expected second seed to do nothing")
	}
}

func TestLoginSuccess(t *testing.T) {
	svc := newTestService(t)
	seedTestUser(t, svc)

	token, csrf, err := svc.Login(t.Context(), "admin@example.com", "secret123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	if token == "" {
		t.Fatal("expected non-empty session token")
	}

	if csrf == "" {
		t.Fatal("expected non-empty CSRF token")
	}

	if csrf == token {
		t.Fatal("expected CSRF token to differ from session token")
	}

	session, user, err := svc.Authenticate(t.Context(), token)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	if user.Email != "admin@example.com" {
		t.Fatalf("expected admin@example.com, got %s", user.Email)
	}

	if session.CsrfToken != csrf {
		t.Fatal("expected session to carry the login CSRF token")
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	svc := newTestService(t)
	seedTestUser(t, svc)

	cases := map[string][2]string{
		"wrong password": {"admin@example.com", "nope"},
		"unknown email":  {"nobody@example.com", "secret123"},
		"empty":          {"", ""},
	}

	for name, creds := range cases {
		if _, _, err := svc.Login(t.Context(), creds[0], creds[1]); err != ErrInvalidCredentials {
			t.Fatalf("%s: expected ErrInvalidCredentials, got %v", name, err)
		}
	}

	if _, _, err := svc.Authenticate(t.Context(), "bogus-token"); err == nil {
		t.Fatal("expected error for unknown token")
	}

	if _, _, err := svc.Authenticate(t.Context(), ""); err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestAuthenticateExpiredSession(t *testing.T) {
	svc := newTestService(t)
	seedTestUser(t, svc)

	user, err := svc.queries.GetUserByEmail(t.Context(), "admin@example.com")
	if err != nil {
		t.Fatalf("find user: %v", err)
	}

	token, err := newToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	if _, err := svc.queries.CreateSession(t.Context(), db.CreateSessionParams{
		TokenHash: hashToken(token),
		UserID:    user.ID,
		CsrfToken: "test-csrf-token",
		ExpiresAt: time.Now().UTC().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("create expired session: %v", err)
	}

	if _, _, err := svc.Authenticate(t.Context(), token); err == nil {
		t.Fatal("expected error for expired session")
	}

	if _, err := svc.queries.GetSessionByTokenHash(t.Context(), hashToken(token)); err != sql.ErrNoRows {
		t.Fatalf("expected expired session to be deleted, got %v", err)
	}
}

func TestLogout(t *testing.T) {
	svc := newTestService(t)
	seedTestUser(t, svc)

	token, _, err := svc.Login(t.Context(), "admin@example.com", "secret123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	if err := svc.Logout(t.Context(), token); err != nil {
		t.Fatalf("logout: %v", err)
	}

	if _, _, err := svc.Authenticate(t.Context(), token); err == nil {
		t.Fatal("expected logged-out token to be invalid")
	}

	// Logging out twice (or with an empty token) must not fail.
	if err := svc.Logout(t.Context(), token); err != nil {
		t.Fatalf("second logout: %v", err)
	}

	if err := svc.Logout(t.Context(), ""); err != nil {
		t.Fatalf("empty logout: %v", err)
	}
}

func TestHashPassword(t *testing.T) {
	hash, err := HashPassword("secret123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	if hash == "secret123" {
		t.Fatal("password must not be stored in plain text")
	}
}

func csrfRequest(t *testing.T, header, field string) *http.Request {
	t.Helper()

	form := url.Values{}
	if field != "" {
		form.Set("csrf_token", field)
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/logout", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if header != "" {
		req.Header.Set("X-CSRF-Token", header)
	}

	return req
}

func TestValidCsrf(t *testing.T) {
	const expected = "session-csrf-token"

	if !ValidCsrf(csrfRequest(t, expected, ""), expected) {
		t.Fatal("expected header token to validate")
	}

	if !ValidCsrf(csrfRequest(t, "", expected), expected) {
		t.Fatal("expected form field token to validate")
	}

	if ValidCsrf(csrfRequest(t, "wrong", ""), expected) {
		t.Fatal("expected wrong header token to fail")
	}

	if ValidCsrf(csrfRequest(t, "", "wrong"), expected) {
		t.Fatal("expected wrong form field token to fail")
	}

	if ValidCsrf(csrfRequest(t, "", ""), expected) {
		t.Fatal("expected missing token to fail")
	}

	if ValidCsrf(csrfRequest(t, "", ""), "") {
		t.Fatal("expected empty expected token to never validate")
	}
}

func TestBackfillSessionCsrf(t *testing.T) {
	svc := newTestService(t)
	seedTestUser(t, svc)
	ctx := t.Context()

	user, err := svc.queries.GetUserByEmail(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("find user: %v", err)
	}

	// Simulate a pre-CSRF session with an empty token.
	legacy, err := newToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	if _, err := svc.queries.CreateSession(ctx, db.CreateSessionParams{
		TokenHash: hashToken(legacy),
		UserID:    user.ID,
		CsrfToken: "",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatalf("create legacy session: %v", err)
	}

	healed, err := svc.BackfillSessionCsrf(ctx)
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}

	if healed != 1 {
		t.Fatalf("expected 1 healed session, got %d", healed)
	}

	session, err := svc.queries.GetSessionByTokenHash(ctx, hashToken(legacy))
	if err != nil {
		t.Fatalf("find session: %v", err)
	}

	if session.CsrfToken == "" {
		t.Fatal("expected session to have a CSRF token after backfill")
	}

	healed, err = svc.BackfillSessionCsrf(ctx)
	if err != nil {
		t.Fatalf("second backfill: %v", err)
	}

	if healed != 0 {
		t.Fatalf("expected 0 healed sessions on second run, got %d", healed)
	}
}
