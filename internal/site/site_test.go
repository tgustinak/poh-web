package site

import "testing"

func TestURL(t *testing.T) {
	t.Cleanup(func() { SetBaseURL("") })

	SetBaseURL("https://staging.example.com/")

	if got := URL("/"); got != "https://staging.example.com/" {
		t.Fatalf("unexpected URL %q", got)
	}

	if got := URL("/static/x.jpg"); got != "https://staging.example.com/static/x.jpg" {
		t.Fatalf("unexpected URL %q", got)
	}
}
