package organizers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIsSlot(t *testing.T) {
	if len(Slots()) != 3 {
		t.Fatalf("expected 3 fixed slots, got %d", len(Slots()))
	}

	for _, s := range Slots() {
		if !IsSlot(s) {
			t.Fatalf("expected %q to be a known slot", s)
		}
	}

	if IsSlot("rider-x") {
		t.Fatal("expected unknown slot to be rejected")
	}

	if IsSlot("") {
		t.Fatal("expected empty slot to be rejected")
	}
}

func TestValidateAlt(t *testing.T) {
	cleaned, ok := ValidateAlt("  Promo foto  ")
	if !ok || cleaned != "Promo foto" {
		t.Fatalf("expected trimmed value, got %q, %v", cleaned, ok)
	}

	if cleaned, ok := ValidateAlt(""); !ok || cleaned != "" {
		t.Fatal("expected empty alt to be valid")
	}

	if _, ok := ValidateAlt(strings.Repeat("a", MaxAltLen+1)); ok {
		t.Fatal("expected overlong alt to be invalid")
	}
}

func TestDetectDocument(t *testing.T) {
	data := []byte("%PDF-1.4\n%test document\n")

	ext, ok := DetectDocument(data)
	if !ok || ext != ".pdf" {
		t.Fatalf("expected pdf bytes to detect as pdf, got %q, %v", ext, ok)
	}

	if _, ok := DetectDocument([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F'}); ok {
		t.Fatal("expected jpeg bytes to be rejected as document")
	}

	if _, ok := DetectDocument(nil); ok {
		t.Fatal("expected empty data to be rejected")
	}
}

func TestDetectPhoto(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "web", "static", "files", "poh-promo-2.jpg"))
	if err != nil {
		t.Fatalf("read seed photo: %v", err)
	}

	ext, ok := DetectPhoto(data)
	if !ok || ext != ".jpg" {
		t.Fatalf("expected seed promo to detect as jpg, got %q, %v", ext, ok)
	}

	if _, ok := DetectPhoto([]byte("%PDF-1.4 fake")); ok {
		t.Fatal("expected pdf bytes to be rejected as photo")
	}
}

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{
		0:       "0 B",
		512:     "512 B",
		123158:  "120 kB",
		418238:  "408 kB",
		2967087: "2,8 MB",
		3339634: "3,2 MB",
	}

	for bytes, want := range cases {
		if got := HumanSize(bytes); got != want {
			t.Fatalf("expected HumanSize(%d) = %q, got %q", bytes, want, got)
		}
	}
}

func TestMetaLabel(t *testing.T) {
	if got := MetaLabel(".pdf", 123158); got != "PDF • 120 kB" {
		t.Fatalf("expected document meta, got %q", got)
	}
}

func TestFormatUpdated(t *testing.T) {
	tm := time.Date(2026, 9, 28, 21, 30, 0, 0, time.UTC)
	if got := FormatUpdated(tm); got != "28. 9. 2026 21:30" {
		t.Fatalf("expected formatted datetime, got %q", got)
	}
}
