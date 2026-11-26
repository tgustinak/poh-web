// Package site holds the public base URL of the website (scheme + host).
// It is used for absolute URLs that must not depend on the incoming
// request: canonical links, Open Graph tags, sitemap and robots.txt.
// The value is set once at startup from SITE_URL, so staging can point
// at its own host while production uses the default.
package site

import "strings"

// DefaultBaseURL is the production site.
const DefaultBaseURL = "https://www.pohkapela.sk"

var baseURL = DefaultBaseURL

// SetBaseURL sets the public base URL. Call once at startup, before
// serving requests. A trailing slash is ignored.
func SetBaseURL(u string) {
	baseURL = strings.TrimRight(u, "/")
}

// BaseURL returns the public base URL without a trailing slash.
func BaseURL() string {
	return baseURL
}

// URL returns the absolute URL for a site path. The path must start with
// "/" ("/" is the home page).
func URL(path string) string {
	return baseURL + path
}
