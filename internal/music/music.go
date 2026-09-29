// Package music provides domain logic for the "Hudba" section:
// validation, Spotify artist URL parsing and display mapping for
// the album list (free CRUD in display order) and the singleton
// music settings (Spotify artist URL for the player and links).
package music

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/tgustinak/pohkapela.sk/internal/db"
)

// Year bounds for album validation.
const (
	MinYear = 1900
	MaxYear = 2100
)

// Max lengths for editable fields (in runes, after trim).
const (
	MaxTitleLen = 100
	MaxBadgeLen = 50
)

// MaxCoverBytes caps an uploaded album cover.
const MaxCoverBytes = 10 << 20

// Album is the display form of one album.
type Album struct {
	ID        int64
	Title     string
	Year      int64
	Badge     string
	CoverPath string
	IsNew     bool
	// CoverAlt is derived ("Obal albumu Život (2026)").
	CoverAlt string
	// YearLabel is the year as text ("2026").
	YearLabel string
}

// Settings is the display form of the Spotify artist presence.
// The URL is owned by the Social section (spotify link): links always
// use it raw, the embed player is derived only when it points at an
// artist page (otherwise the player is hidden and links still work).
type Settings struct {
	// SpotifyURL is the Social spotify link.
	SpotifyURL string
	// SpotifyEmbedURL is the consent-gated player source,
	// empty when the URL is not an artist page.
	SpotifyEmbedURL string
}

// ValidateTitle checks the album title (1..MaxTitleLen chars after trim).
func ValidateTitle(s string) (string, bool) {
	t := strings.TrimSpace(s)
	if t == "" || len([]rune(t)) > MaxTitleLen {
		return "", false
	}

	return t, true
}

// ValidateYear checks the album year (4 digits within MinYear..MaxYear).
func ValidateYear(s string) (int64, bool) {
	t := strings.TrimSpace(s)
	if len(t) != 4 {
		return 0, false
	}

	year, err := strconv.ParseInt(t, 10, 64)
	if err != nil {
		return 0, false
	}

	if year < MinYear || year > MaxYear {
		return 0, false
	}

	return year, true
}

// ValidateBadge checks the album badge (0..MaxBadgeLen chars after trim).
// Empty badge is allowed: no badge is rendered then.
func ValidateBadge(s string) (string, bool) {
	t := strings.TrimSpace(s)
	if len([]rune(t)) > MaxBadgeLen {
		return "", false
	}

	return t, true
}

// DetectCover checks uploaded bytes are a supported cover image.
// It returns the file extension for storage.
func DetectCover(data []byte) (string, bool) {
	if len(data) == 0 || len(data) > MaxCoverBytes {
		return "", false
	}

	switch http.DetectContentType(data) {
	case "image/jpeg":
		return ".jpg", true
	case "image/png":
		return ".png", true
	case "image/webp":
		return ".webp", true
	default:
		return "", false
	}
}

// ParseSpotifyArtistURL extracts the normalized artist URL from a Spotify
// artist link. It accepts https://open.spotify.com/artist/<id> with an
// optional /intl-<locale> prefix and query string, and returns the
// normalized form without query. The second return value is false for
// anything else (tracks, playlists, other hosts, non-https).
func ParseSpotifyArtistURL(raw string) (string, bool) {
	t := strings.TrimSpace(raw)
	if t == "" {
		return "", false
	}

	if !strings.HasPrefix(t, "https://") {
		return "", false
	}

	u, err := url.Parse(t)
	if err != nil {
		return "", false
	}

	if u.Hostname() != "open.spotify.com" {
		return "", false
	}

	path := strings.Trim(u.Path, "/")
	segments := strings.Split(path, "/")

	// Optional intl-<locale> prefix, e.g. /intl-sk/artist/<id>.
	if len(segments) == 3 && strings.HasPrefix(segments[0], "intl-") {
		segments = segments[1:]
	}

	if len(segments) != 2 || segments[0] != "artist" || segments[1] == "" {
		return "", false
	}

	id := segments[1]
	for _, r := range id {
		isAlnum := r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if !isAlnum {
			return "", false
		}
	}

	return "https://open.spotify.com/artist/" + id, true
}

// EmbedURL builds the consent-gated player source from a normalized
// artist URL, e.g. https://open.spotify.com/embed/artist/<id>?utm_source=generator&theme=0.
func EmbedURL(spotifyURL string) string {
	return "https://open.spotify.com/embed/artist/" + artistID(spotifyURL) + "?utm_source=generator&theme=0"
}

// artistID returns the last path segment of a normalized artist URL.
func artistID(spotifyURL string) string {
	trimmed := strings.TrimSuffix(strings.TrimSpace(spotifyURL), "/")
	if i := strings.LastIndex(trimmed, "/"); i >= 0 {
		return trimmed[i+1:]
	}

	return trimmed
}

// CoverAlt builds the cover alt text ("Obal albumu Život (2026)").
func CoverAlt(title string, year int64) string {
	return "Obal albumu " + title + " (" + strconv.FormatInt(year, 10) + ")"
}

// AlbumFromDB maps a database album to its display form.
func AlbumFromDB(row db.Album) Album {
	yearLabel := strconv.FormatInt(row.Year, 10)

	return Album{
		ID:        row.ID,
		Title:     row.Title,
		Year:      row.Year,
		Badge:     row.Badge,
		CoverPath: row.CoverPath,
		IsNew:     row.IsNew == 1,
		CoverAlt:  CoverAlt(row.Title, row.Year),
		YearLabel: yearLabel,
	}
}

// AlbumsFromDBMany maps database albums to display form, preserving order.
func AlbumsFromDBMany(rows []db.Album) []Album {
	out := make([]Album, 0, len(rows))
	for _, row := range rows {
		out = append(out, AlbumFromDB(row))
	}

	return out
}

// SettingsFromSpotifyURL builds the Spotify display form from the Social
// spotify link. The embed player is derived only from artist page URLs;
// any other URL keeps working links with the player hidden.
func SettingsFromSpotifyURL(rawURL string) Settings {
	spotifyURL := strings.TrimSpace(rawURL)
	embedURL := ""

	if normalized, ok := ParseSpotifyArtistURL(spotifyURL); ok {
		embedURL = EmbedURL(normalized)
	}

	return Settings{
		SpotifyURL:      spotifyURL,
		SpotifyEmbedURL: embedURL,
	}
}
