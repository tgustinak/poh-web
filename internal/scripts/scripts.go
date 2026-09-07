// Package scripts implements a scripts stack for server-rendered pages.
//
// Page partials register their inline <script> blocks with Defer instead
// of rendering them mid-page. Base flushes everything right before
// </body>, so the final HTML groups all scripts in one place:
//
//	@scripts.Defer("concerts", ConcertsScripts()) // in the partial
//	@scripts.Flush()                              // in Base, before </body>
//
// Registration travels in the request context (see WithCollector),
// because templ renders in a single pass: children render before
// Base reaches the flush point. Without a collector in the context
// (tests, htmx fragments), Defer renders inline as before, so nothing
// silently disappears.
package scripts

import (
	"context"
	"io"

	"github.com/a-h/templ"
)

type collectorKey struct{}

// Collector gathers deferred script components in registration order.
// The zero value is not usable; create one with WithCollector.
type Collector struct {
	keys  []string
	seen  map[string]struct{}
	comps []templ.Component
}

// WithCollector stores a new Collector in ctx and returns both.
func WithCollector(ctx context.Context) (context.Context, *Collector) {
	col := &Collector{seen: map[string]struct{}{}}

	return context.WithValue(ctx, collectorKey{}, col), col
}

func fromContext(ctx context.Context) (*Collector, bool) {
	col, ok := ctx.Value(collectorKey{}).(*Collector)

	return col, ok && col != nil
}

// Add stashes component under key (first registration wins).
func (c *Collector) Add(key string, comp templ.Component) {
	if _, ok := c.seen[key]; ok {
		return
	}

	c.seen[key] = struct{}{}
	c.keys = append(c.keys, key)
	c.comps = append(c.comps, comp)
}

// All returns stashed components in registration order.
func (c *Collector) All() []templ.Component {
	return c.comps
}

// Defer stashes component for later rendering by Flush and renders
// nothing in place. Without a collector in ctx it renders inline,
// matching the old behavior (safe for tests and htmx fragments).
func Defer(key string, comp templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if col, ok := fromContext(ctx); ok {
			col.Add(key, comp)

			return nil
		}

		return comp.Render(ctx, w)
	})
}

// Flush renders all stashed components in registration order.
// Without a collector in ctx it renders nothing.
func Flush() templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		col, ok := fromContext(ctx)
		if !ok {
			return nil
		}

		for _, comp := range col.All() {
			if err := comp.Render(ctx, w); err != nil {
				return err
			}
		}

		return nil
	})
}
