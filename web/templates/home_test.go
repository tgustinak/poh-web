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

	data := NewConcertsData(ConcertsParams{Upcoming: upcoming, Past: past})

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
	section := NewConcertsData(ConcertsParams{
		Upcoming: []db.Concert{{Name: "Budúci", EventDate: "2026-12-01"}},
		Past:     []db.Concert{{Name: "Minulý", EventDate: "2025-05-01"}},
	})
	aboutRow := db.About{P1: "p1", P2: "p2", Quote: "q"}

	home := NewHomeData(HomeParams{Concerts: section, Videos: nil, About: aboutRow})

	if len(home.Concerts.Upcoming) != 1 || home.Concerts.PastCount != 1 {
		t.Fatalf("expected section passthrough, got %+v", home.Concerts)
	}

	if home.About.P1 != "p1" || home.About.P2 != "p2" || home.About.Quote != "q" {
		t.Fatalf("expected about passthrough, got %+v", home.About)
	}

	empty := NewHomeData(HomeParams{Concerts: NewConcertsData(ConcertsParams{}), About: db.About{}})
	if len(empty.Concerts.Upcoming) != 0 || empty.Concerts.PastCount != 0 {
		t.Fatalf("expected empty home data, got %+v", empty.Concerts)
	}
}
