package templates

import (
	"github.com/tgustinak/pohkapela.sk/internal/concerts"
	"github.com/tgustinak/pohkapela.sk/internal/db"
)

// HomeData is the view model for the public home page,
// grouped per section so growing sections never change
// the Home template signature.
type HomeData struct {
	Concerts ConcertsData
}

// ConcertsData is the view model for the concerts section.
type ConcertsData struct {
	Upcoming []concerts.Concert
	Past     []concerts.PastYear
	// PastCount is the total number of past concerts.
	PastCount int
}

// NewHomeData assembles the home page view model from section view models.
func NewHomeData(concerts ConcertsData) HomeData {
	return HomeData{
		Concerts: concerts,
	}
}

// NewConcertsData assembles the concerts section view model
// from database rows.
func NewConcertsData(upcomingRows, pastRows []db.Concert) ConcertsData {
	return ConcertsData{
		Upcoming:  concerts.FromDBMany(upcomingRows),
		Past:      concerts.GroupPastByYear(pastRows),
		PastCount: len(pastRows),
	}
}
