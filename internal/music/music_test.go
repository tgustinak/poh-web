package music

import (
	"strings"
	"testing"
)

func TestValidateTitle(t *testing.T) {
	cleaned, ok := ValidateTitle("  Život  ")
	if !ok || cleaned != "Život" {
		t.Fatalf("expected trimmed value, got %q, %v", cleaned, ok)
	}

	if _, ok := ValidateTitle("   "); ok {
		t.Fatal("expected empty title to be invalid")
	}

	if _, ok := ValidateTitle(strings.Repeat("a", MaxTitleLen+1)); ok {
		t.Fatal("expected overlong title to be invalid")
	}
}

func TestValidateYear(t *testing.T) {
	year, ok := ValidateYear("2026")
	if !ok || year != 2026 {
		t.Fatalf("expected 2026, got %d, %v", year, ok)
	}

	for _, bad := range []string{"", "26", "20266", "abcd", "1899", "2101", "  "} {
		if _, ok := ValidateYear(bad); ok {
			t.Fatalf("expected %q to be invalid", bad)
		}
	}

	if _, ok := ValidateYear(" 2021 "); !ok {
		t.Fatal("expected padded year to be valid")
	}
}

func TestValidateBadge(t *testing.T) {
	cleaned, ok := ValidateBadge("  Novinka  ")
	if !ok || cleaned != "Novinka" {
		t.Fatalf("expected trimmed value, got %q, %v", cleaned, ok)
	}

	if cleaned, ok := ValidateBadge(""); !ok || cleaned != "" {
		t.Fatal("expected empty badge to be valid")
	}

	if _, ok := ValidateBadge(strings.Repeat("a", MaxBadgeLen+1)); ok {
		t.Fatal("expected overlong badge to be invalid")
	}
}

func TestParseSpotifyArtistURL(t *testing.T) {
	id := "79t3KrQngTRDooegKsR4cw"
	want := "https://open.spotify.com/artist/" + id

	for _, raw := range []string{
		want,
		want + "?si=e0f93c63516a49fb",
		"https://open.spotify.com/intl-sk/artist/" + id + "?si=abc",
		"  " + want + "  ",
	} {
		got, ok := ParseSpotifyArtistURL(raw)
		if !ok || got != want {
			t.Fatalf("expected %q to parse as %q, got %q, %v", raw, want, got, ok)
		}
	}

	for _, bad := range []string{
		"",
		"http://open.spotify.com/artist/" + id,
		"https://open.spotify.com/track/abc123",
		"https://open.spotify.com/album/abc123",
		"https://open.spotify.com/artist/",
		"https://open.spotify.com/artist/not an id!",
		"https://example.com/artist/" + id,
		"spotify:artist:" + id,
	} {
		if _, ok := ParseSpotifyArtistURL(bad); ok {
			t.Fatalf("expected %q to be invalid", bad)
		}
	}
}

func TestEmbedURL(t *testing.T) {
	got := EmbedURL("https://open.spotify.com/artist/79t3KrQngTRDooegKsR4cw")
	want := "https://open.spotify.com/embed/artist/79t3KrQngTRDooegKsR4cw?utm_source=generator&theme=0"

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestDetectCover(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d}
	if ext, ok := DetectCover(png); !ok || ext != ".png" {
		t.Fatalf("expected png, got %q, %v", ext, ok)
	}

	if _, ok := DetectCover([]byte("%PDF-1.4 fake")); ok {
		t.Fatal("expected pdf to be rejected as cover")
	}

	if _, ok := DetectCover(nil); ok {
		t.Fatal("expected empty data to be rejected")
	}
}

func TestCoverAlt(t *testing.T) {
	if got := CoverAlt("Život", 2026); got != "Obal albumu Život (2026)" {
		t.Fatalf("expected alt, got %q", got)
	}
}

func TestSettingsFromSpotifyURL(t *testing.T) {
	id := "79t3KrQngTRDooegKsR4cw"

	s := SettingsFromSpotifyURL("https://open.spotify.com/artist/" + id + "?si=abc")
	if s.SpotifyURL != "https://open.spotify.com/artist/"+id+"?si=abc" {
		t.Fatalf("expected raw link passthrough, got %q", s.SpotifyURL)
	}

	if s.SpotifyEmbedURL != EmbedURL("https://open.spotify.com/artist/"+id) {
		t.Fatalf("expected derived embed, got %q", s.SpotifyEmbedURL)
	}

	// Non-artist URLs keep working links with the player hidden.
	s = SettingsFromSpotifyURL("https://open.spotify.com/playlist/abc123")
	if s.SpotifyURL != "https://open.spotify.com/playlist/abc123" {
		t.Fatalf("expected raw link passthrough, got %q", s.SpotifyURL)
	}

	if s.SpotifyEmbedURL != "" {
		t.Fatalf("expected hidden player, got %q", s.SpotifyEmbedURL)
	}

	s = SettingsFromSpotifyURL("")
	if s.SpotifyURL != "" || s.SpotifyEmbedURL != "" {
		t.Fatalf("expected empty settings, got %+v", s)
	}
}
