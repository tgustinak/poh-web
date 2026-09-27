package templates

import (
	"testing"

	"github.com/tgustinak/pohkapela.sk/internal/db"
)

func TestNewConcertsData(t *testing.T) {
	upcoming := []db.Concert{{Name: "Budúci", EventDate: "2026-12-01"}}
	past := []db.Concert{
		{Name: "Minulý 1", EventDate: "2025-05-01"},
		{Name: "Minulý 2", EventDate: "2024-06-01"},
	}

	data := NewConcertsData(upcoming, past)

	if len(data.Upcoming) != 1 || data.Upcoming[0].Name != "Budúci" {
		t.Fatalf("unexpected upcoming: %+v", data.Upcoming)
	}

	if data.PastCount != 2 {
		t.Fatalf("expected past count 2, got %d", data.PastCount)
	}

	if len(data.Past) != 2 || data.Past[0].Year != 2025 || data.Past[1].Year != 2024 {
		t.Fatalf("unexpected past groups: %+v", data.Past)
	}
}

func TestNewHomeData(t *testing.T) {
	section := NewConcertsData(
		[]db.Concert{{Name: "Budúci", EventDate: "2026-12-01"}},
		[]db.Concert{{Name: "Minulý", EventDate: "2025-05-01"}},
	)

	home := NewHomeData(section, nil)

	if len(home.Concerts.Upcoming) != 1 || home.Concerts.PastCount != 1 {
		t.Fatalf("expected section passthrough, got %+v", home.Concerts)
	}

	empty := NewHomeData(NewConcertsData(nil, nil), nil)
	if len(empty.Concerts.Upcoming) != 0 || empty.Concerts.PastCount != 0 {
		t.Fatalf("expected empty home data, got %+v", empty.Concerts)
	}
}
