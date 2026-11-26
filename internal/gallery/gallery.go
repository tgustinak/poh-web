// Package gallery provides domain logic for the public media gallery:
// validation, YouTube parsing, image detection and display mapping.
// A gallery item is either an uploaded photo or a YouTube video; both
// kinds live in one ordered list and are displayed on the same level.
package gallery

import (
	"strings"

	"github.com/tgustinak/poh-web/internal/db"
	"github.com/tgustinak/poh-web/internal/uploads"
	"github.com/tgustinak/poh-web/internal/videos"
)

// Item kinds stored in gallery_items.kind.
const (
	KindPhoto = "photo"
	KindVideo = "video"
)

// Upload and editable-field limits.
const (
	MaxPhotoBytes = 10 << 20
	MaxAltLen     = 200
	MaxTitleLen   = 200
)

// IsKind reports whether s is a supported gallery item kind.
func IsKind(s string) bool {
	return s == KindPhoto || s == KindVideo
}

// ValidateAlt checks a photo description (0..MaxAltLen chars after trim).
// Empty alt is allowed: templates fall back to a generic label.
func ValidateAlt(s string) (string, bool) {
	t := strings.TrimSpace(s)
	if len([]rune(t)) > MaxAltLen {
		return "", false
	}

	return t, true
}

// ValidateTitle checks a video title (1..MaxTitleLen chars after trim).
func ValidateTitle(s string) (string, bool) {
	t := strings.TrimSpace(s)
	if t == "" || len([]rune(t)) > MaxTitleLen {
		return "", false
	}

	return t, true
}

// DetectPhoto checks uploaded bytes are a supported image.
// It returns the file extension for storage.
func DetectPhoto(data []byte) (string, bool) {
	return uploads.DetectImage(data, MaxPhotoBytes)
}

// YouTubeID extracts the 11-character video ID from a YouTube URL or
// returns the input itself when it already is a bare ID.
func YouTubeID(raw string) (string, bool) {
	return videos.ParseYouTubeID(raw)
}

// ThumbnailURL returns the YouTube thumbnail URL for a video ID.
func ThumbnailURL(id string) string {
	return "https://i.ytimg.com/vi/" + id + "/hqdefault.jpg"
}

// FileName returns the base file name of a file path for admin display.
func FileName(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}

	return path
}

// Item is the display form of one gallery entry.
type Item struct {
	ID          int64
	Kind        string
	FilePath    string
	PreviewPath string
	YoutubeID   string
	Title       string
	Alt         string
}

// IsPhoto reports whether the item is an uploaded photo.
func (i Item) IsPhoto() bool {
	return i.Kind == KindPhoto
}

// IsVideo reports whether the item is a YouTube video.
func (i Item) IsVideo() bool {
	return i.Kind == KindVideo
}

// DisplayTitle is the caption/title used in the grid and lightbox,
// with a generic fallback when the admin left it empty.
func (i Item) DisplayTitle() string {
	if i.IsVideo() {
		if i.Title != "" {
			return i.Title
		}

		return "Video P.O.H"
	}

	if i.Alt != "" {
		return i.Alt
	}

	return "Fotografia P.O.H"
}

// Thumbnail is the image shown in the grid: the lightweight photo
// preview, the full photo when no preview exists, or the YouTube thumb.
func (i Item) Thumbnail() string {
	if i.IsVideo() {
		return ThumbnailURL(i.YoutubeID)
	}

	if i.PreviewPath != "" {
		return i.PreviewPath
	}

	return i.FilePath
}

// FromDB maps a database gallery item to its display form.
func FromDB(row db.GalleryItem) Item {
	return Item{
		ID:          row.ID,
		Kind:        row.Kind,
		FilePath:    row.FilePath,
		PreviewPath: row.PreviewPath,
		YoutubeID:   row.YoutubeID,
		Title:       row.Title,
		Alt:         row.Alt,
	}
}

// FromDBMany maps database gallery items to display form, preserving order.
func FromDBMany(rows []db.GalleryItem) []Item {
	out := make([]Item, 0, len(rows))
	for _, row := range rows {
		out = append(out, FromDB(row))
	}

	return out
}
