// Package videos provides domain logic for videoclips:
// YouTube ID parsing, form validation and display mapping.
// All metadata (title, date, duration) are entered manually,
// no external YouTube calls are made.
package videos

import (
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/tgustinak/pohkapela.sk/internal/db"
)

// DateLayout is the ISO-8601 layout used for published dates
// (storage, filtering and HTML date inputs).
const DateLayout = "2006-01-02"

var (
	youtubeIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	durationPattern  = regexp.MustCompile(`^\d{1,3}:[0-5]\d$`)
)

// Video is the display form of a videoclip for the public page.
type Video struct {
	ID        int64
	YoutubeID string
	Title     string
	// PublishedDate is the ISO-8601 date for machine use.
	PublishedDate string
	// Date is the human-readable date ("19. 6. 2026").
	Date     string
	Duration string
}

// ParseYouTubeID extracts the 11-character video ID from a YouTube URL
// or returns the input itself when it already is a bare ID.
// Supported forms: watch?v=, youtu.be/, /shorts/, /embed/, /live/, bare ID.
func ParseYouTubeID(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}

	if youtubeIDPattern.MatchString(s) {
		return s, true
	}

	// Allow URLs without scheme ("www.youtube.com/watch?v=...").
	withScheme := s
	if !strings.Contains(s, "://") {
		withScheme = "https://" + s
	}

	u, err := url.Parse(withScheme)
	if err != nil {
		return "", false
	}

	host := strings.ToLower(u.Hostname())
	isYouTube := strings.Contains(host, "youtube.com") || strings.Contains(host, "youtube-nocookie.com")

	if isYouTube {
		if id := u.Query().Get("v"); id != "" && youtubeIDPattern.MatchString(id) {
			return id, true
		}
	}

	path := strings.Trim(u.Path, "/")
	segments := strings.Split(path, "/")

	switch {
	case host == "youtu.be" || host == "www.youtu.be":
		if len(segments) >= 1 && youtubeIDPattern.MatchString(segments[0]) {
			return segments[0], true
		}
	case isYouTube:
		// /embed/<id>, /shorts/<id>, /live/<id>, /v/<id>
		if len(segments) >= 2 && youtubeIDPattern.MatchString(segments[1]) {
			switch segments[0] {
			case "embed", "shorts", "live", "v":
				return segments[1], true
			}
		}
	}

	return "", false
}

// ParseDate parses an ISO-8601 published date.
func ParseDate(s string) (time.Time, error) {
	return time.Parse(DateLayout, s)
}

// ValidateTitle checks the clip title (1..200 chars after trim).
func ValidateTitle(title string) (string, bool) {
	t := strings.TrimSpace(title)
	if t == "" || len([]rune(t)) > 200 {
		return "", false
	}

	return t, true
}

// ValidateDuration checks the duration text ("4:24", "12:05").
func ValidateDuration(duration string) (string, bool) {
	d := strings.TrimSpace(duration)
	if !durationPattern.MatchString(d) {
		return "", false
	}

	return d, true
}

// ShortDate formats an ISO-8601 date as "19. 6. 2026" for display.
func ShortDate(iso string) string {
	date, err := ParseDate(iso)
	if err != nil {
		return iso
	}

	return date.Format("2. 1. 2006")
}

// FromDB maps a database video to its display form.
func FromDB(v db.Video) Video {
	return Video{
		ID:            v.ID,
		YoutubeID:     v.YoutubeID,
		Title:         v.Title,
		PublishedDate: v.PublishedDate,
		Date:          ShortDate(v.PublishedDate),
		Duration:      v.Duration,
	}
}

// FromDBMany maps database videos to their display form, preserving order.
func FromDBMany(rows []db.Video) []Video {
	out := make([]Video, 0, len(rows))
	for _, v := range rows {
		out = append(out, FromDB(v))
	}

	return out
}
