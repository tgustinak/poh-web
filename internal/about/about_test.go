package about

import (
	"strings"
	"testing"
)

func TestValidateParagraph(t *testing.T) {
	cleaned, ok := ValidateParagraph("  Ahoj  ")
	if !ok || cleaned != "Ahoj" {
		t.Fatalf("expected trimmed value, got %q, %v", cleaned, ok)
	}

	if _, ok := ValidateParagraph("   "); ok {
		t.Fatal("expected empty paragraph to be invalid")
	}

	if _, ok := ValidateParagraph(strings.Repeat("a", MaxParagraphLen+1)); ok {
		t.Fatal("expected overlong paragraph to be invalid")
	}

	if _, ok := ValidateParagraph(strings.Repeat("a", MaxParagraphLen)); !ok {
		t.Fatal("expected max-length paragraph to be valid")
	}
}

func TestValidateQuote(t *testing.T) {
	cleaned, ok := ValidateQuote("  Citát  ")
	if !ok || cleaned != "Citát" {
		t.Fatalf("expected trimmed value, got %q, %v", cleaned, ok)
	}

	if _, ok := ValidateQuote(""); ok {
		t.Fatal("expected empty quote to be invalid")
	}

	if _, ok := ValidateQuote(strings.Repeat("a", MaxQuoteLen+1)); ok {
		t.Fatal("expected overlong quote to be invalid")
	}
}
