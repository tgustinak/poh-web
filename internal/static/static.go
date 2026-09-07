// Package static resolves versioned URLs for static assets.
// The ?v= query carries the file modification time, so browsers always
// fetch a fresh copy after a rebuild instead of serving a stale one
// from cache. The timestamp is read on every call (no caching),
// which keeps `make dev` rebuilds working.
package static

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// URL returns the public URL for a file under web/static,
// with a modification-time query for cache busting.
// Unknown files fall back to the plain path.
func URL(path string) string {
	public := "/static/" + path

	info, err := os.Stat(filepath.Join(staticDir(), path))
	if err != nil {
		return public
	}

	return fmt.Sprintf("%s?v=%d", public, info.ModTime().Unix())
}

func staticDir() string {
	_, file, _, _ := runtime.Caller(0)

	return filepath.Join(filepath.Dir(file), "..", "..", "web", "static")
}
