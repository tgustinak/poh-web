// Package organizers provides domain logic for the "Pre organizátorov"
// page: validation, file naming and display mapping for organizer
// documents (3 fixed slots, admin only replaces the file) and promo
// photos (free CRUD list).
package organizers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tgustinak/pohkapela.sk/internal/db"
)

// Document slots in display order (matches the position column seed).
const (
	SlotRider     = "rider"
	SlotInputList = "input-list"
	SlotLogo      = "logo"
)

// Size limits for uploads.
const (
	MaxDocumentBytes = 15 << 20
	MaxPhotoBytes    = 10 << 20
)

// Max lengths for editable fields (in runes, after trim).
const MaxAltLen = 200

// Slots returns the fixed document slots in display order.
func Slots() []string {
	return []string{SlotRider, SlotInputList, SlotLogo}
}

// IsSlot reports whether s is one of the fixed document slots.
func IsSlot(s string) bool {
	for _, known := range Slots() {
		if s == known {
			return true
		}
	}

	return false
}

// ValidateAlt checks the photo description (0..MaxAltLen chars after trim).
// Empty alt is allowed: templates fall back to a generic label.
func ValidateAlt(s string) (string, bool) {
	t := strings.TrimSpace(s)
	if len([]rune(t)) > MaxAltLen {
		return "", false
	}

	return t, true
}

// DetectDocument checks uploaded bytes are a PDF for a document slot.
// It returns the file extension for storage.
func DetectDocument(data []byte) (string, bool) {
	if len(data) == 0 || len(data) > MaxDocumentBytes {
		return "", false
	}

	if http.DetectContentType(data) != "application/pdf" {
		return "", false
	}

	return ".pdf", true
}

// DetectPhoto checks uploaded bytes are a supported image.
// It returns the file extension for storage.
func DetectPhoto(data []byte) (string, bool) {
	if len(data) == 0 || len(data) > MaxPhotoBytes {
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

// KindLabel returns the short file-kind label for a meta line
// ("PDF", "PNG", "JPG", "WEBP") from a file extension.
func KindLabel(ext string) string {
	switch strings.ToLower(ext) {
	case ".pdf":
		return "PDF"
	case ".jpg", ".jpeg":
		return "JPG"
	case ".png":
		return "PNG"
	case ".webp":
		return "WEBP"
	default:
		return strings.ToUpper(strings.TrimPrefix(ext, "."))
	}
}

// MetaLabel builds the meta line for a freshly uploaded file,
// e.g. "PDF • 2,8 MB".
func MetaLabel(ext string, sizeBytes int64) string {
	return KindLabel(ext) + " • " + HumanSize(sizeBytes)
}

// HumanSize formats bytes with Slovak decimal comma,
// e.g. 123158 -> "120 kB", 2967087 -> "2,8 MB".
func HumanSize(bytes int64) string {
	if bytes < 0 {
		bytes = 0
	}

	const (
		kb = 1024
		mb = 1024 * 1024
	)

	switch {
	case bytes >= mb:
		return comma1(float64(bytes)/mb) + " MB"
	case bytes >= kb:
		return fmt.Sprintf("%d kB", (bytes+kb/2)/kb)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// comma1 formats a float with one decimal and a Slovak decimal comma.
func comma1(f float64) string {
	s := fmt.Sprintf("%.1f", f)

	return strings.Replace(s, ".", ",", 1)
}

// FormatUpdated formats an updated_at timestamp for admin display,
// e.g. "28. 9. 2026 21:30".
func FormatUpdated(t time.Time) string {
	return t.Format("2. 1. 2006 15:04")
}

// FileName returns the base file name of a file path
// (upload URL or /static/ path) for admin display.
func FileName(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}

	return path
}

// PhotoAlt returns the alt text for a photo,
// falling back to a generic label when empty.
func PhotoAlt(p Photo) string {
	if p.Alt != "" {
		return p.Alt
	}

	return "Promo foto P.O.H"
}

// Document is the display form of one organizer document.
type Document struct {
	Slot        string
	DisplayName string
	FilePath    string
	// Meta is the "PDF • 120 kB" line.
	Meta string
}

// Photo is the display form of one promo photo.
type Photo struct {
	ID          int64
	FilePath    string
	PreviewPath string
	Alt         string
	MetaLabel   string
}

// DocumentFromDB maps a database document row to its display form.
func DocumentFromDB(row db.OrganizerDocument) Document {
	ext := ""
	if dot := strings.LastIndex(row.FilePath, "."); dot >= 0 {
		ext = row.FilePath[dot:]
	}

	return Document{
		Slot:        row.Slot,
		DisplayName: row.DisplayName,
		FilePath:    row.FilePath,
		Meta:        KindLabel(ext) + " • " + HumanSize(row.SizeBytes),
	}
}

// DocumentsFromDBMany maps document rows to display form, preserving order.
func DocumentsFromDBMany(rows []db.OrganizerDocument) []Document {
	out := make([]Document, 0, len(rows))
	for _, row := range rows {
		out = append(out, DocumentFromDB(row))
	}

	return out
}

// PhotoFromDB maps a database photo row to its display form.
func PhotoFromDB(row db.OrganizerPhoto) Photo {
	return Photo{
		ID:          row.ID,
		FilePath:    row.FilePath,
		PreviewPath: row.PreviewPath,
		Alt:         row.Alt,
		MetaLabel:   row.MetaLabel,
	}
}

// PhotosFromDBMany maps photo rows to display form, preserving order.
func PhotosFromDBMany(rows []db.OrganizerPhoto) []Photo {
	out := make([]Photo, 0, len(rows))
	for _, row := range rows {
		out = append(out, PhotoFromDB(row))
	}

	return out
}
