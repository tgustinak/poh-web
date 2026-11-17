package gallery

import (
	"strings"
	"testing"

	"github.com/tgustinak/pohkapela.sk/internal/db"
)

func TestIsKind(t *testing.T) {
	for _, kind := range []string{KindPhoto, KindVideo} {
		if !IsKind(kind) {
			t.Fatalf("expected %q to be a valid kind", kind)
		}
	}

	for _, kind := range []string{"", "image", "youtube", "PHOTO"} {
		if IsKind(kind) {
			t.Fatalf("expected %q to be invalid", kind)
		}
	}
}

func TestValidateAlt(t *testing.T) {
	got, ok := ValidateAlt("  Kapela na pódiu  ")
	if !ok || got != "Kapela na pódiu" {
		t.Fatalf("expected trimmed alt, got %q (ok=%v)", got, ok)
	}

	if _, ok := ValidateAlt(strings.Repeat("a", MaxAltLen+1)); ok {
		t.Fatal("expected overlong alt to be rejected")
	}

	if _, ok := ValidateAlt(strings.Repeat("a", MaxAltLen)); !ok {
		t.Fatal("expected max-length alt to be accepted")
	}
}

func TestValidateTitle(t *testing.T) {
	got, ok := ValidateTitle("  Živý záznam  ")
	if !ok || got != "Živý záznam" {
		t.Fatalf("expected trimmed title, got %q (ok=%v)", got, ok)
	}

	if _, ok := ValidateTitle("   "); ok {
		t.Fatal("expected empty title to be rejected")
	}

	if _, ok := ValidateTitle(strings.Repeat("a", MaxTitleLen+1)); ok {
		t.Fatal("expected overlong title to be rejected")
	}
}

func TestDetectPhoto(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		ext  string
		ok   bool
	}{
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, ".jpg", true},
		{"png", []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n', 0x00, 0x00}, ".png", true},
		{"webp", []byte{'R', 'I', 'F', 'F', 0x24, 0x00, 0x00, 0x00, 'W', 'E', 'B', 'P', 'V', 'P', '8', ' '}, ".webp", true},
		{"pdf", []byte("%PDF-1.4\n"), "", false},
		{"empty", nil, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext, ok := DetectPhoto(tt.data)
			if ok != tt.ok || ext != tt.ext {
				t.Fatalf("DetectPhoto() = (%q, %v), want (%q, %v)", ext, ok, tt.ext, tt.ok)
			}
		})
	}
}

func TestYouTubeID(t *testing.T) {
	tests := []struct {
		raw string
		id  string
		ok  bool
	}{
		{"https://www.youtube.com/watch?v=7Msu0l_5Pi0", "7Msu0l_5Pi0", true},
		{"https://youtu.be/ylJqEmM-Sjo", "ylJqEmM-Sjo", true},
		{"7Msu0l_5Pi0", "7Msu0l_5Pi0", true},
		{"https://example.com/watch?v=7Msu0l_5Pi0", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		got, ok := YouTubeID(tt.raw)
		if ok != tt.ok || got != tt.id {
			t.Fatalf("YouTubeID(%q) = (%q, %v), want (%q, %v)", tt.raw, got, ok, tt.id, tt.ok)
		}
	}
}

func TestFromDB(t *testing.T) {
	photo := FromDB(db.GalleryItem{
		ID:          1,
		Kind:        KindPhoto,
		FilePath:    "/uploads/galeria-foto-a.jpg",
		PreviewPath: "/uploads/galeria-nahlad-a.webp",
		Alt:         "Kapela na pódiu",
		Position:    1,
	})
	if photo.IsVideo() || !photo.IsPhoto() {
		t.Fatalf("expected photo item, got kind %q", photo.Kind)
	}

	if photo.DisplayTitle() != "Kapela na pódiu" {
		t.Fatalf("unexpected photo title %q", photo.DisplayTitle())
	}

	if photo.Thumbnail() != "/uploads/galeria-nahlad-a.webp" {
		t.Fatalf("unexpected photo thumbnail %q", photo.Thumbnail())
	}

	video := FromDB(db.GalleryItem{
		ID:        2,
		Kind:      KindVideo,
		YoutubeID: "7Msu0l_5Pi0",
		Title:     "Nechcem zabudnúť",
		Position:  2,
	})
	if !video.IsVideo() || video.IsPhoto() {
		t.Fatalf("expected video item, got kind %q", video.Kind)
	}

	if video.DisplayTitle() != "Nechcem zabudnúť" {
		t.Fatalf("unexpected video title %q", video.DisplayTitle())
	}

	if video.Thumbnail() != "https://i.ytimg.com/vi/7Msu0l_5Pi0/hqdefault.jpg" {
		t.Fatalf("unexpected video thumbnail %q", video.Thumbnail())
	}
}

func TestDisplayFallbacks(t *testing.T) {
	photo := Item{Kind: KindPhoto, FilePath: "/uploads/galeria-foto-a.jpg"}
	if photo.DisplayTitle() != "Fotografia P.O.H" {
		t.Fatalf("unexpected photo fallback title %q", photo.DisplayTitle())
	}

	if photo.Thumbnail() != "/uploads/galeria-foto-a.jpg" {
		t.Fatalf("expected full photo fallback, got %q", photo.Thumbnail())
	}

	video := Item{Kind: KindVideo, YoutubeID: "7Msu0l_5Pi0"}
	if video.DisplayTitle() != "Video P.O.H" {
		t.Fatalf("unexpected video fallback title %q", video.DisplayTitle())
	}
}

func TestFileName(t *testing.T) {
	tests := map[string]string{
		"/uploads/galeria-foto-abc.jpg": "galeria-foto-abc.jpg",
		"galeria-nahlad-abc.webp":       "galeria-nahlad-abc.webp",
		"":                              "",
	}

	for in, want := range tests {
		if got := FileName(in); got != want {
			t.Fatalf("FileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFromDBManyPreservesOrder(t *testing.T) {
	items := FromDBMany([]db.GalleryItem{
		{ID: 1, Kind: KindPhoto},
		{ID: 2, Kind: KindVideo},
	})

	if len(items) != 2 || items[0].ID != 1 || items[1].ID != 2 {
		t.Fatalf("expected order preserved, got %+v", items)
	}
}
