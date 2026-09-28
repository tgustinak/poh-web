package contact

import "testing"

func TestNormalizePhone(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
		ok    bool
	}{
		{"spaced Slovak", "+421 900 123 456", "+421900123456", true},
		{"local Slovak", "0900 123 456", "+421900123456", true},
		{"local Slovak dashes", "0900-123-456", "+421900123456", true},
		{"double zero prefix", "00421900123456", "+421900123456", true},
		{"already E164", "+421900123456", "+421900123456", true},
		{"international", "+1 415 555 2671", "+14155552671", true},
		{"too short", "0907", "", false},
		{"slovak wrong length", "+421 900 123 45", "", false},
		{"letters", "0907abc893", "", false},
		{"empty", "", "", false},
		{"no country code", "123456789", "", false},
		{"too long", "+1234567890123456", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := NormalizePhone(tc.input)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("NormalizePhone(%q) = (%q, %v), want (%q, %v)", tc.input, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestDisplayPhone(t *testing.T) {
	if got := DisplayPhone("+421900123456"); got != "+421 900 123 456" {
		t.Fatalf("DisplayPhone Slovak = %q", got)
	}

	if got := DisplayPhone("+14155552671"); got != "+14155552671" {
		t.Fatalf("DisplayPhone international = %q", got)
	}

	if got := DisplayPhone(""); got != "" {
		t.Fatalf("DisplayPhone empty = %q", got)
	}
}

func TestNormalizeEmail(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
		ok    bool
	}{
		{"plain", "kontakt@example.com", "kontakt@example.com", true},
		{"trim and lowercase", "  Kontakt@Example.COM ", "kontakt@example.com", true},
		{"missing at", "manager.pohkapela.sk", "", false},
		{"missing domain dot", "kontakt@localhost", "", false},
		{"empty", "", "", false},
		{"with space", "mana ger@pohkapela.sk", "", false},
		{"no tld", "kontakt@example", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := NormalizeEmail(tc.input)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("NormalizeEmail(%q) = (%q, %v), want (%q, %v)", tc.input, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestValidateNameAndRole(t *testing.T) {
	if _, ok := ValidateName("  "); ok {
		t.Fatal("blank name should be invalid")
	}

	if _, ok := ValidateRole(""); ok {
		t.Fatal("empty role should be invalid")
	}

	long := make([]rune, MaxNameLen+1)
	for i := range long {
		long[i] = 'a'
	}

	if _, ok := ValidateName(string(long)); ok {
		t.Fatal("overlong name should be invalid")
	}

	if got, ok := ValidateName("  Jana Testová "); !ok || got != "Jana Testová" {
		t.Fatalf("ValidateName trim = (%q, %v)", got, ok)
	}
}
