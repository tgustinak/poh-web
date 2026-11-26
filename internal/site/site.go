// Package site holds the public base URL of the website (scheme + host).
// It is used for absolute URLs that must not depend on the incoming
// request: canonical links, Open Graph tags, sitemap and robots.txt.
// The value is set once at startup from the required SITE_URL, so each
// environment (production, staging) advertises its own host.
package site

import "strings"

var baseURL string

// SetBaseURL sets the public base URL. Call once at startup, before
// serving requests. A trailing slash is ignored.
func SetBaseURL(u string) {
	baseURL = strings.TrimRight(u, "/")
}

// URL returns the absolute URL for a site path. The path must start with
// "/" ("/" is the home page).
func URL(path string) string {
	return baseURL + path
}
