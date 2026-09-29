package templates

import (
	"github.com/tgustinak/pohkapela.sk/internal/about"
	"github.com/tgustinak/pohkapela.sk/internal/concerts"
	"github.com/tgustinak/pohkapela.sk/internal/contact"
	"github.com/tgustinak/pohkapela.sk/internal/db"
	"github.com/tgustinak/pohkapela.sk/internal/music"
	"github.com/tgustinak/pohkapela.sk/internal/social"
	"github.com/tgustinak/pohkapela.sk/internal/videos"
)

// HomeData is the view model for the public home page,
// grouped per section so growing sections never change
// the Home template signature.
type HomeData struct {
	Concerts ConcertsData
	Music    MusicData
	Videos   []videos.Video
	About    about.About
	Social   []social.Link
	Contact  contact.Contact
}

// MusicData is the view model for the music section.
type MusicData struct {
	Albums  []music.Album
	Spotify music.Settings
}

// ConcertsData is the view model for the concerts section.
type ConcertsData struct {
	Upcoming []concerts.Concert
	Past     []concerts.PastYear
	// PastCount is the total number of past concerts.
	PastCount int
}

// HomeParams carries the inputs for NewHomeData as named fields,
// so callers cannot swap sections and new sections do not change
// the function signature.
type HomeParams struct {
	Concerts ConcertsData
	Videos   []db.Video
	About    db.About
	Social   []db.SocialLink
	// Contact is already resolved by admin.ContactHandler.GetContact,
	// which falls back to no photo when the file is missing on disk.
	Contact contact.Contact
	// Albums are the raw album rows in display order.
	Albums []db.Album
}

// spotifyPlatform is the Social link feeding the music player and links.
const spotifyPlatform = "spotify"

// NewHomeData assembles the home page view model from section view models.
func NewHomeData(p HomeParams) HomeData {
	spotifyURL := ""
	for _, l := range p.Social {
		if l.Platform == spotifyPlatform {
			spotifyURL = l.Url
		}
	}

	return HomeData{
		Concerts: p.Concerts,
		Music: MusicData{
			Albums:  music.AlbumsFromDBMany(p.Albums),
			Spotify: music.SettingsFromSpotifyURL(spotifyURL),
		},
		Videos:  videos.FromDBMany(p.Videos),
		About:   about.FromDB(p.About),
		Social:  social.FromDBMany(p.Social),
		Contact: p.Contact,
	}
}

// ConcertsParams carries the inputs for NewConcertsData as named fields,
// so upcoming and past rows cannot be swapped by mistake.
type ConcertsParams struct {
	Upcoming []db.Concert
	Past     []db.Concert
}

// NewConcertsData assembles the concerts section view model
// from database rows.
func NewConcertsData(p ConcertsParams) ConcertsData {
	return ConcertsData{
		Upcoming:  concerts.FromDBMany(p.Upcoming),
		Past:      concerts.GroupPastByYear(p.Past),
		PastCount: len(p.Past),
	}
}
