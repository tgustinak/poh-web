// Package database opens the SQLite database and applies schema migrations.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	dbmigrations "github.com/tgustinak/pohkapela.sk/db/migrations"
	"github.com/tgustinak/pohkapela.sk/internal/db"
)

// Open connects to the SQLite database at path, creates the parent
// directory if needed and applies all pending goose migrations.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}

	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// SQLite handles a single writer at a time; serialize access.
	conn.SetMaxOpenConns(1)

	if err := conn.PingContext(ctx); err != nil {
		_ = conn.Close()

		return nil, fmt.Errorf("ping database: %w", err)
	}

	if err := Migrate(ctx, conn); err != nil {
		_ = conn.Close()

		return nil, err
	}

	// Drop expired sessions left over from previous runs.
	if err := db.New(conn).DeleteExpiredSessions(ctx); err != nil {
		_ = conn.Close()

		return nil, fmt.Errorf("delete expired sessions: %w", err)
	}

	return conn, nil
}

// Migrate applies all pending embedded migrations to conn.
func Migrate(ctx context.Context, conn *sql.DB) error {
	goose.SetBaseFS(dbmigrations.FS)

	if err := goose.SetDialect("sqlite"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	if err := goose.UpContext(ctx, conn, "."); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}
