// Package social provides domain logic for the "Social" section:
// validation, display mapping and CTA labels for the fixed set
// of 6 social / streaming links. The set is fixed (no add/delete
// from admin): admin edits only handle and url, everything else
// (display name, CTA, button style, icon, order) is derived
// from the platform.
package social

import (
	"net/url"
	"strings"

	"github.com/tgustinak/pohkapela.sk/internal/db"
)

// Platforms in display order (matches the position column seed).
const (
	PlatformFacebook  = "facebook"
	PlatformInstagram = "instagram"
	PlatformYouTube   = "youtube"
	PlatformSpotify   = "spotify"
	PlatformYTMusic   = "ytmusic"
	PlatformApple     = "apple"
)

// Max lengths for the editable fields (in runes, after trim).
const (
	MaxHandleLen = 100
	MaxURLLen    = 500
)

// Link is the display form of one social link.
type Link struct {
	Platform    string
	DisplayName string
	Handle      string
	URL         string
	// CTA is derived from the platform (Sledovať / Odoberať / Počúvať).
	CTA string
}

// Platforms returns the fixed platform set in display order.
func Platforms() []string {
	return []string{
		PlatformFacebook,
		PlatformInstagram,
		PlatformYouTube,
		PlatformSpotify,
		PlatformYTMusic,
		PlatformApple,
	}
}

// IsPlatform reports whether p is one of the fixed platforms.
func IsPlatform(p string) bool {
	for _, known := range Platforms() {
		if p == known {
			return true
		}
	}

	return false
}

// Cta returns the button label for a platform.
// Unknown platforms fall back to "Sledovať".
func Cta(platform string) string {
	switch platform {
	case PlatformYouTube:
		return "Odoberať"
	case PlatformSpotify, PlatformYTMusic, PlatformApple:
		return "Počúvať"
	default:
		return "Sledovať"
	}
}

// ValidateHandle checks the displayed handle (1..MaxHandleLen chars after trim).
func ValidateHandle(s string) (string, bool) {
	t := strings.TrimSpace(s)
	if t == "" || len([]rune(t)) > MaxHandleLen {
		return "", false
	}

	return t, true
}

// ValidateURL checks the link URL. Only https:// URLs are accepted
// (1..MaxURLLen chars after trim, valid absolute URL with a host).
func ValidateURL(s string) (string, bool) {
	t := strings.TrimSpace(s)
	if t == "" || len([]rune(t)) > MaxURLLen {
		return "", false
	}

	if !strings.HasPrefix(t, "https://") {
		return "", false
	}

	u, err := url.Parse(t)
	if err != nil {
		return "", false
	}

	if !u.IsAbs() || u.Hostname() == "" {
		return "", false
	}

	if strings.Contains(t, " ") {
		return "", false
	}

	return t, true
}

// FromDB maps a database social link to its display form.
func FromDB(row db.SocialLink) Link {
	return Link{
		Platform:    row.Platform,
		DisplayName: row.DisplayName,
		Handle:      row.Handle,
		URL:         row.Url,
		CTA:         Cta(row.Platform),
	}
}

// FromDBMany maps database social links to their display form,
// preserving order.
func FromDBMany(rows []db.SocialLink) []Link {
	out := make([]Link, 0, len(rows))
	for _, row := range rows {
		out = append(out, FromDB(row))
	}

	return out
}
