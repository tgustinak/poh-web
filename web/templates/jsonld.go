package templates

import (
	"encoding/json"

	"github.com/a-h/templ"
	"github.com/tgustinak/poh-web/internal/site"
)

// jsonLD renders the schema.org MusicGroup description. json.Marshal
// escapes <, > and & so the content is safe inside a script element.
func jsonLD() templ.Component {
	data, err := json.Marshal(map[string]string{
		"@context":   "https://schema.org",
		"@type":      "MusicGroup",
		"name":       "P.O.H",
		"genre":      "Folk-Rock",
		"areaServed": "SK",
		"url":        site.URL("/"),
	})
	if err != nil {
		return templ.NopComponent
	}

	return templ.Raw(`<script type="application/ld+json">` + string(data) + `</script>`)
}
