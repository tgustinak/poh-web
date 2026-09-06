package concerts

import (
	"testing"

	"github.com/tgustinak/pohkapela.sk/internal/db"
)

func dbConcert(date string) db.Concert {
	return db.Concert{Name: "Test", EventDate: date}
}

func TestFromDB(t *testing.T) {
	c := FromDB(dbConcert("2026-09-18"))

	if c.Name != "Test" || c.Day != 18 || c.Month != "sep" {
		t.Fatalf("unexpected mapping: %+v", c)
	}

	may := FromDB(dbConcert("2026-05-01"))
	if may.Month != "máj" {
		t.Fatalf("expected máj, got %q", may.Month)
	}

	broken := FromDB(dbConcert("not-a-date"))
	if broken.Name != "Test" || broken.Day != 0 || broken.Month != "" {
		t.Fatalf("expected name-only fallback, got %+v", broken)
	}
}

func TestGroupPastByYear(t *testing.T) {
	in := []db.Concert{
		dbConcert("2026-03-01"),
		dbConcert("2026-01-18"),
		dbConcert("2025-12-20"),
		dbConcert("not-a-date"),
	}

	groups := GroupPastByYear(in)
	if len(groups) != 2 {
		t.Fatalf("expected 2 year groups, got %d", len(groups))
	}

	if groups[0].Year != 2026 || len(groups[0].Concerts) != 2 {
		t.Fatalf("unexpected first group: %+v", groups[0])
	}

	if groups[1].Year != 2025 || len(groups[1].Concerts) != 1 {
		t.Fatalf("unexpected second group: %+v", groups[1])
	}

	if len(GroupPastByYear(nil)) != 0 {
		t.Fatal("expected no groups for empty input")
	}
}

func TestShortDate(t *testing.T) {
	if got := ShortDate("2026-09-18"); got != "18. 9. 2026" {
		t.Fatalf("expected 18. 9. 2026, got %q", got)
	}

	if got := ShortDate("bogus"); got != "bogus" {
		t.Fatalf("expected fallback to input, got %q", got)
	}
}
