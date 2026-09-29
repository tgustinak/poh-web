package templates

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tgustinak/pohkapela.sk/internal/music"
	"github.com/tgustinak/pohkapela.sk/internal/scripts"
)

func renderMusic(t *testing.T, count int) string {
	t.Helper()

	albums := make([]music.Album, 0, count)
	for i := 0; i < count; i++ {
		albums = append(albums, music.Album{Title: "Album", YearLabel: "2026"})
	}

	req := httptest.NewRequest("GET", "/", nil)
	ctx, _ := scripts.WithCollector(req.Context())

	rec := httptest.NewRecorder()
	Music(albums, music.Settings{SpotifyURL: "https://open.spotify.com/artist/x"}).Render(ctx, rec)

	return rec.Body.String()
}

func TestMusicCardWidthByCount(t *testing.T) {
	two := renderMusic(t, 2)
	if !strings.Contains(two, `sm:basis-[calc(50%-12px)]`) {
		t.Fatal("expected two albums to fill the row")
	}

	if strings.Contains(two, "44%-12px") {
		t.Fatal("expected no peek width for two albums")
	}

	three := renderMusic(t, 3)
	if !strings.Contains(three, `sm:basis-[calc(44%-12px)]`) {
		t.Fatal("expected third album to peek with more than two albums")
	}
}
