package site

import "testing"

func TestURL(t *testing.T) {
	t.Cleanup(func() { SetBaseURL(DefaultBaseURL) })

	if got := URL("/"); got != "https://www.pohkapela.sk/" {
		t.Fatalf("unexpected default URL %q", got)
	}

	SetBaseURL("https://staging.example.com/")

	if got := URL("/static/x.jpg"); got != "https://staging.example.com/static/x.jpg" {
		t.Fatalf("unexpected URL %q", got)
	}
}
