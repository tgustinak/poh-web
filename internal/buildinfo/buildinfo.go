// Package buildinfo exposes the version, commit and build time injected at
// build time (via -ldflags -X) plus the applied database schema version.
package buildinfo

import "sync/atomic"

// Set at build time, e.g.
// -X github.com/tgustinak/poh-web/internal/buildinfo.Version=v1.0.0
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

var dbVersion atomic.Int64

// SetDBVersion records the applied database schema (goose) version.
func SetDBVersion(v int64) { dbVersion.Store(v) }

// DBVersion returns the applied database schema version.
func DBVersion() int64 { return dbVersion.Load() }

// ShortCommit returns the first 7 characters of the commit hash.
func ShortCommit() string {
	if len(Commit) > 7 {
		return Commit[:7]
	}

	return Commit
}
