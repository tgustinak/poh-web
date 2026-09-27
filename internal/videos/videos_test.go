package videos

import "testing"

func TestParseYouTubeID(t *testing.T) {
	cases := map[string]string{
		"7Msu0l_5Pi0": "7Msu0l_5Pi0",
		"https://www.youtube.com/watch?v=7Msu0l_5Pi0":       "7Msu0l_5Pi0",
		"https://www.youtube.com/watch?v=7Msu0l_5Pi0&t=10s": "7Msu0l_5Pi0",
		"https://youtu.be/ylJqEmM-Sjo":                      "ylJqEmM-Sjo",
		"https://www.youtube.com/shorts/AxT7ni2COK4":        "AxT7ni2COK4",
		"https://www.youtube.com/embed/BB56qBn2wtA":         "BB56qBn2wtA",
		"https://www.youtube.com/live/7tgNr5sWXds":          "7tgNr5sWXds",
		"www.youtube.com/watch?v=KKd_lv_zC5E":               "KKd_lv_zC5E",
		"  Qx7rHKYHRAE  ":                                   "Qx7rHKYHRAE",
	}

	for in, want := range cases {
		if got, ok := ParseYouTubeID(in); !ok || got != want {
			t.Errorf("ParseYouTubeID(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
}

func TestParseYouTubeIDInvalid(t *testing.T) {
	for _, in := range []string{
		"",
		"not a video!!",
		"https://www.youtube.com/watch?v=short",
		"https://example.com/watch?v=7Msu0l_5Pi0",
		"https://www.youtube.com/",
	} {
		if got, ok := ParseYouTubeID(in); ok {
			t.Errorf("ParseYouTubeID(%q) = %q, expected failure", in, got)
		}
	}
}

func TestValidateDuration(t *testing.T) {
	for _, ok := range []string{"4:24", "3:20", "12:05", "5:11"} {
		if _, valid := ValidateDuration(ok); !valid {
			t.Errorf("expected duration %q to be valid", ok)
		}
	}

	for _, bad := range []string{"", "4", "4:5", "4:75", "abc", "4:240"} {
		if _, valid := ValidateDuration(bad); valid {
			t.Errorf("expected duration %q to be invalid", bad)
		}
	}
}

func TestShortDate(t *testing.T) {
	if got := ShortDate("2026-06-19"); got != "19. 6. 2026" {
		t.Fatalf("ShortDate = %q", got)
	}
}
