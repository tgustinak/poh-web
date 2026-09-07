package scripts

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func renderToString(t *testing.T, ctx context.Context, comp templ.Component) string {
	t.Helper()

	var out strings.Builder
	if err := comp.Render(ctx, &out); err != nil {
		t.Fatalf("render: %v", err)
	}

	return out.String()
}

func scriptComp(js string) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := w.Write([]byte("<script>" + js + "</script>"))

		return err
	})
}

func TestDeferAndFlushOrder(t *testing.T) {
	ctx, _ := WithCollector(context.Background())

	if got := renderToString(t, ctx, Defer("a", scriptComp("var a=1;"))); got != "" {
		t.Fatalf("expected Defer to render nothing, got %q", got)
	}

	_ = renderToString(t, ctx, Defer("b", scriptComp("var b=2;")))

	got := renderToString(t, ctx, Flush())
	want := "<script>var a=1;</script><script>var b=2;</script>"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestDeferDedupesByKey(t *testing.T) {
	ctx, _ := WithCollector(context.Background())

	_ = renderToString(t, ctx, Defer("x", scriptComp("first")))
	_ = renderToString(t, ctx, Defer("x", scriptComp("second")))

	if got := renderToString(t, ctx, Flush()); got != "<script>first</script>" {
		t.Fatalf("expected first registration to win, got %q", got)
	}
}

func TestDeferWithoutCollectorRendersInline(t *testing.T) {
	got := renderToString(t, context.Background(), Defer("x", scriptComp("inline")))

	if got != "<script>inline</script>" {
		t.Fatalf("expected inline fallback, got %q", got)
	}
}

func TestFlushWithoutCollectorRendersNothing(t *testing.T) {
	if got := renderToString(t, context.Background(), Flush()); got != "" {
		t.Fatalf("expected empty output, got %q", got)
	}
}
