package social

import (
	"strings"
	"testing"
)

func TestCta(t *testing.T) {
	cases := map[string]string{
		PlatformFacebook:  "Sledovať",
		PlatformInstagram: "Sledovať",
		PlatformYouTube:   "Odoberať",
		PlatformSpotify:   "Počúvať",
		PlatformYTMusic:   "Počúvať",
		PlatformApple:     "Počúvať",
		"unknown":         "Sledovať",
	}

	for platform, want := range cases {
		if got := Cta(platform); got != want {
			t.Fatalf("expected CTA %q for %q, got %q", want, platform, got)
		}
	}
}

func TestIsPlatform(t *testing.T) {
	for _, p := range Platforms() {
		if !IsPlatform(p) {
			t.Fatalf("expected %q to be a known platform", p)
		}
	}

	if len(Platforms()) != 6 {
		t.Fatalf("expected 6 fixed platforms, got %d", len(Platforms()))
	}

	if IsPlatform("tiktok") {
		t.Fatal("expected tiktok to be rejected")
	}

	if IsPlatform("") {
		t.Fatal("expected empty platform to be rejected")
	}
}

func TestValidateHandle(t *testing.T) {
	cleaned, ok := ValidateHandle("  @kapela  ")
	if !ok || cleaned != "@kapela" {
		t.Fatalf("expected trimmed value, got %q, %v", cleaned, ok)
	}

	if _, ok := ValidateHandle("   "); ok {
		t.Fatal("expected empty handle to be invalid")
	}

	if _, ok := ValidateHandle(strings.Repeat("a", MaxHandleLen+1)); ok {
		t.Fatal("expected overlong handle to be invalid")
	}

	if _, ok := ValidateHandle(strings.Repeat("a", MaxHandleLen)); !ok {
		t.Fatal("expected max-length handle to be valid")
	}
}

func TestValidateURL(t *testing.T) {
	cleaned, ok := ValidateURL("  https://www.facebook.com/kapelaPOH/  ")
	if !ok || cleaned != "https://www.facebook.com/kapelaPOH/" {
		t.Fatalf("expected trimmed value, got %q, %v", cleaned, ok)
	}

	for _, bad := range []string{
		"",
		"   ",
		"http://example.com/x",
		"javascript:alert(1)",
		"https://",
		"https://exa mple.com",
		"www.facebook.com/kapelaPOH",
	} {
		if _, ok := ValidateURL(bad); ok {
			t.Fatalf("expected %q to be invalid", bad)
		}
	}

	if _, ok := ValidateURL("https://" + strings.Repeat("a", MaxURLLen)); ok {
		t.Fatal("expected overlong URL to be invalid")
	}
}
