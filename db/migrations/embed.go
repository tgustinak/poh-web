// Package migrations exposes the embedded goose SQL migration files
// so the application binary can migrate the database without needing
// the migration files on disk.
package migrations

import "embed"

// FS holds all *.sql migration files in this directory.
//
//go:embed *.sql
var FS embed.FS
