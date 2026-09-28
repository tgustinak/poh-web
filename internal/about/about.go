// Package about provides domain logic for the "O nás" section:
// validation and display mapping for the singleton about row.
package about

import (
	"strings"

	"github.com/tgustinak/pohkapela.sk/internal/db"
)

// Max lengths for the editable fields (in runes, after trim).
const (
	MaxParagraphLen = 2000
	MaxQuoteLen     = 500
)

// About is the display form of the "O nás" section.
type About struct {
	P1    string
	P2    string
	Quote string
}

// ValidateParagraph checks a body paragraph (1..MaxParagraphLen chars after trim).
func ValidateParagraph(s string) (string, bool) {
	t := strings.TrimSpace(s)
	if t == "" || len([]rune(t)) > MaxParagraphLen {
		return "", false
	}

	return t, true
}

// ValidateQuote checks the quote (1..MaxQuoteLen chars after trim).
func ValidateQuote(s string) (string, bool) {
	t := strings.TrimSpace(s)
	if t == "" || len([]rune(t)) > MaxQuoteLen {
		return "", false
	}

	return t, true
}

// FromDB maps a database about row to its display form.
func FromDB(row db.About) About {
	return About{
		P1:    row.P1,
		P2:    row.P2,
		Quote: row.Quote,
	}
}
