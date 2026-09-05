package logbuffer

import (
	"log/slog"
	"strings"
	"sync"
	"testing"
)

func TestBufferKeepsNewestFirst(t *testing.T) {
	buf := New(3)
	buf.Append(Entry{Message: "first"})
	buf.Append(Entry{Message: "second"})

	entries := buf.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].Message != "second" || entries[1].Message != "first" {
		t.Fatalf("expected newest first, got %+v", entries)
	}
}

func TestBufferDropsOldestWhenFull(t *testing.T) {
	buf := New(2)
	buf.Append(Entry{Message: "first"})
	buf.Append(Entry{Message: "second"})
	buf.Append(Entry{Message: "third"})

	entries := buf.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].Message != "third" || entries[1].Message != "second" {
		t.Fatalf("expected oldest dropped, got %+v", entries)
	}
}

func TestBufferConcurrentUse(t *testing.T) {
	buf := New(100)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				buf.Append(Entry{Message: "x"})
				_ = buf.Entries()
				_ = buf.Len()
			}
		}()
	}
	wg.Wait()

	if buf.Len() != 100 {
		t.Fatalf("expected buffer capped at 100, got %d", buf.Len())
	}
}

func TestTeeHandlerStoresAndForwards(t *testing.T) {
	var text strings.Builder
	buf := New(10)

	logger := slog.New(TeeHandler(&text, nil, buf))
	logger.Info("noise")
	logger.Warn("trouble", "method", "GET", "status", 200)

	if buf.Len() != 1 {
		t.Fatalf("expected only the warning buffered, got %d", buf.Len())
	}

	entry := buf.Entries()[0]
	if entry.Level != "WARN" || entry.Message != "trouble" {
		t.Fatalf("unexpected entry: %+v", entry)
	}

	if !strings.Contains(entry.Attrs, "method=GET") || !strings.Contains(entry.Attrs, "status=200") {
		t.Fatalf("expected attrs in entry, got %q", entry.Attrs)
	}

	if !strings.Contains(text.String(), "noise") || !strings.Contains(text.String(), "trouble") {
		t.Fatalf("expected all records on stderr, got %q", text.String())
	}
}

func seedMessages(t *testing.T, buf *Buffer, n int) {
	t.Helper()

	for i := 0; i < n; i++ {
		buf.Append(Entry{Level: "WARN", Message: "msg"})
	}
}

func TestRecentPaginates(t *testing.T) {
	buf := New(10)
	seedMessages(t, buf, 5)

	page, next := buf.Recent(2)
	if len(page) != 2 || next == 0 {
		t.Fatalf("expected 2 entries and a cursor, got %d entries next=%d", len(page), next)
	}

	if page[0].Seq <= page[1].Seq {
		t.Fatal("expected newest first")
	}

	page2, next2 := buf.Before(next, 2)
	if len(page2) != 2 || next2 == 0 {
		t.Fatalf("expected 2 entries and a cursor, got %d entries next=%d", len(page2), next2)
	}

	if page2[0].Seq >= page[1].Seq {
		t.Fatal("expected strictly older entries on next page")
	}

	page3, next3 := buf.Before(next2, 2)
	if len(page3) != 1 || next3 != 0 {
		t.Fatalf("expected last entry and no cursor, got %d entries next=%d", len(page3), next3)
	}
}

func TestRecentClampsLimit(t *testing.T) {
	buf := New(10)
	seedMessages(t, buf, 3)

	page, _ := buf.Recent(0)
	if len(page) != 3 {
		t.Fatalf("expected default limit to return all, got %d", len(page))
	}

	page, _ = buf.Recent(1000)
	if len(page) != 3 {
		t.Fatalf("expected clamped limit to return all, got %d", len(page))
	}
}
