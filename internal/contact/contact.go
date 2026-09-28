// Package contact provides domain logic for the "Kontakt" section:
// validation, normalization and display mapping for the singleton
// contact row (manager name, role, phone, e-mail and photo).
package contact

import (
	"net/mail"
	"strings"
	"unicode"

	"github.com/tgustinak/pohkapela.sk/internal/db"
)

// Max lengths for the editable fields (in runes, after trim).
const (
	MaxNameLen  = 100
	MaxRoleLen  = 100
	MaxEmailLen = 254
)

// Contact is the display form of the "Kontakt" section.
// Phone is stored in normalized E.164 form (e.g. +421900123456)
// so the tel: link is always correct; use DisplayPhone for the
// human-readable form with spaces.
type Contact struct {
	Name      string
	Role      string
	Phone     string
	Email     string
	PhotoPath string
}

// ValidateName checks the manager name (1..MaxNameLen chars after trim).
func ValidateName(s string) (string, bool) {
	t := strings.TrimSpace(s)
	if t == "" || len([]rune(t)) > MaxNameLen {
		return "", false
	}

	return t, true
}

// ValidateRole checks the position label (1..MaxRoleLen chars after trim).
func ValidateRole(s string) (string, bool) {
	t := strings.TrimSpace(s)
	if t == "" || len([]rune(t)) > MaxRoleLen {
		return "", false
	}

	return t, true
}

// NormalizePhone cleans a phone input into E.164 form.
// It accepts Slovak numbers (0907..., +421..., 00421...) as well as
// generic international numbers (+..., 00...). Separators like spaces,
// dashes, dots, slashes and parentheses are ignored.
// The second return value is false when the input is not a valid number.
func NormalizePhone(s string) (string, bool) {
	t := strings.TrimSpace(s)
	if t == "" {
		return "", false
	}

	// Keep a leading + or 00 prefix, drop common visual separators.
	var b strings.Builder
	for _, r := range t {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' && b.Len() == 0:
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '.' || r == '/' || r == '(' || r == ')':
			continue
		default:
			return "", false
		}
	}

	digits := b.String()

	hasPlus := strings.HasPrefix(digits, "+")
	if strings.HasPrefix(digits, "00") {
		digits = "+" + digits[2:]
		hasPlus = true
	}

	numeric := strings.TrimPrefix(digits, "+")
	if numeric == "" {
		return "", false
	}

	for _, r := range numeric {
		if !unicode.IsDigit(r) {
			return "", false
		}
	}

	// Slovak 09xx numbers without a country code default to +421.
	if !hasPlus {
		if len(numeric) == 10 && strings.HasPrefix(numeric, "09") {
			numeric = "421" + numeric[1:]
		} else {
			return "", false
		}
	}

	// E.164 allows at most 15 digits; require at least 9 so that
	// short typos (e.g. "0907") are rejected.
	if len(numeric) < 9 || len(numeric) > 15 {
		return "", false
	}

	// Slovak numbers must be +421 + 9 digits.
	if strings.HasPrefix(numeric, "421") && len(numeric) != 12 {
		return "", false
	}

	return "+" + numeric, true
}

// ValidatePhone checks a phone input and returns the normalized E.164 form.
func ValidatePhone(s string) (string, bool) {
	return NormalizePhone(s)
}

// DisplayPhone formats a normalized E.164 number for display.
// Slovak numbers become "+421 900 123 456", other numbers are grouped
// in triples from the left after the country code fallback. Unknown
// input is returned unchanged.
func DisplayPhone(normalized string) string {
	if normalized == "" {
		return ""
	}

	if strings.HasPrefix(normalized, "+421") && len(normalized) == 13 {
		rest := normalized[4:]

		return "+421 " + rest[0:3] + " " + rest[3:6] + " " + rest[6:9]
	}

	return normalized
}

// NormalizeEmail trims and lowercases an e-mail address.
// The second return value is false when the address is invalid.
func NormalizeEmail(s string) (string, bool) {
	t := strings.ToLower(strings.TrimSpace(s))
	if t == "" || len([]rune(t)) > MaxEmailLen {
		return "", false
	}

	addr, err := mail.ParseAddress(t)
	if err != nil {
		return "", false
	}

	if addr.Address != t {
		return "", false
	}

	at := strings.LastIndex(t, "@")
	if at <= 0 || at >= len(t)-1 {
		return "", false
	}

	domain := t[at+1:]
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return "", false
	}

	if strings.Contains(t, " ") {
		return "", false
	}

	return t, true
}

// ValidateEmail checks an e-mail input and returns the normalized form.
func ValidateEmail(s string) (string, bool) {
	return NormalizeEmail(s)
}

// FromDB maps a database contact row to its display form.
func FromDB(row db.Contact) Contact {
	return Contact{
		Name:      row.Name,
		Role:      row.Role,
		Phone:     row.Phone,
		Email:     row.Email,
		PhotoPath: row.PhotoPath,
	}
}
