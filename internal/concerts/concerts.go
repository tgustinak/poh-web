package concerts

import (
	"time"

	"github.com/tgustinak/pohkapela.sk/internal/db"
)

type Concert struct {
	Name  string
	Day   int
	Month string
	// Date is the ISO-8601 event date, for full-date rendering.
	Date string
}

// DateLayout is the ISO-8601 layout used for concert dates
// (storage, filtering and HTML date inputs).
const DateLayout = "2006-01-02"

// slovakMonths are abbreviated Slovak month names for the concert badge.
var slovakMonths = [...]string{
	"",
	"jan", "feb", "mar", "apr", "máj", "jún",
	"júl", "aug", "sep", "okt", "nov", "dec",
}

// ParseDate parses an ISO-8601 concert date.
func ParseDate(s string) (time.Time, error) {
	return time.Parse(DateLayout, s)
}

// FromDB maps a database concert to its display form.
// Concerts with unparseable dates render with an empty badge.
func FromDB(c db.Concert) Concert {
	date, err := ParseDate(c.EventDate)
	if err != nil {
		return Concert{Name: c.Name}
	}

	month := ""
	if m := int(date.Month()); m >= 1 && m <= 12 {
		month = slovakMonths[m]
	}

	return Concert{
		Name:  c.Name,
		Day:   date.Day(),
		Month: month,
		Date:  c.EventDate,
	}
}

// FromDBMany maps database concerts to their display form.
func FromDBMany(concerts []db.Concert) []Concert {
	out := make([]Concert, 0, len(concerts))
	for _, c := range concerts {
		out = append(out, FromDB(c))
	}

	return out
}

// PastYear groups past concerts of a single year, newest year first.
type PastYear struct {
	Year     int
	Concerts []Concert
}

// GroupPastByYear groups concerts (expected newest first) by calendar
// year, preserving order. Concerts with unparseable dates are skipped.
// Used for the compact archive on the web.
func GroupPastByYear(concerts []db.Concert) []PastYear {
	var out []PastYear

	for _, c := range concerts {
		date, err := ParseDate(c.EventDate)
		if err != nil {
			continue
		}

		if len(out) == 0 || out[len(out)-1].Year != date.Year() {
			out = append(out, PastYear{Year: date.Year()})
		}

		out[len(out)-1].Concerts = append(out[len(out)-1].Concerts, FromDB(c))
	}

	return out
}

// Today returns the current local date in ISO-8601 for
// upcoming/past filtering. Passing it as a query parameter
// (instead of CURRENT_DATE in SQL) keeps the comparison in local time.
func Today() string {
	return time.Now().Format(DateLayout)
}

// ShortDate formats an ISO-8601 date as "18. 9. 2026"
// for compact archive rows.
func ShortDate(iso string) string {
	date, err := ParseDate(iso)
	if err != nil {
		return iso
	}

	return date.Format("2. 1. 2006")
}
