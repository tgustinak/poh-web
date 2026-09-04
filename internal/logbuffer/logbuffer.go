// Package logbuffer keeps the most recent slog records in memory
// so the admin log viewer (/admin/logs) can display them.
// Records are also forwarded to a wrapped handler (stderr),
// so nothing is lost when the buffer overflows.
package logbuffer

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
)

// DefaultMaxEntries caps how many records are kept.
const DefaultMaxEntries = 500

// DefaultPageSize is the default number of entries per log viewer page.
const DefaultPageSize = 50

// MaxPageSize caps the limit query parameter of the log viewer.
const MaxPageSize = 100

// Entry is a single log record prepared for display.
type Entry struct {
	// Seq is a monotonic id used as pagination cursor.
	Seq uint64
	// TimeLabel is the record time formatted for display (15:04:05).
	TimeLabel string
	// Level is INFO, WARN, ERROR or DEBUG.
	Level string
	// Message is the log message.
	Message string
	// Attrs are key=value pairs joined with spaces.
	Attrs string
}

// Buffer is a thread-safe ring of recent entries (stored oldest first,
// capped at max; reads return newest first).
type Buffer struct {
	mu      sync.Mutex
	entries []Entry
	max     int
	seq     uint64
}

// New creates a buffer holding up to max entries.
func New(max int) *Buffer {
	if max <= 0 {
		max = DefaultMaxEntries
	}

	return &Buffer{max: max}
}

// Append adds an entry, dropping the oldest one when full.
func (b *Buffer) Append(e Entry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.seq++
	e.Seq = b.seq
	b.entries = append(b.entries, e)
	if len(b.entries) > b.max {
		b.entries = append([]Entry(nil), b.entries[len(b.entries)-b.max:]...)
	}
}

// Entries returns a copy of buffered entries, newest first.
func (b *Buffer) Entries() []Entry {
	b.mu.Lock()
	defer b.mu.Unlock()

	out := make([]Entry, len(b.entries))
	for i, e := range b.entries {
		out[len(b.entries)-1-i] = e
	}

	return out
}

// Recent returns up to limit newest entries (newest first) plus
// a cursor for the next (older) page, or 0 when there is nothing more.
func (b *Buffer) Recent(limit int) ([]Entry, uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.pageLocked(len(b.entries), limit)
}

// Before returns up to limit entries older than seq (newest first) plus
// a cursor for the next page, or 0 when there is nothing more.
func (b *Buffer) Before(seq uint64, limit int) ([]Entry, uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	end := len(b.entries)
	for end > 0 && b.entries[end-1].Seq >= seq {
		end--
	}

	return b.pageLocked(end, limit)
}

// pageLocked pages entries[:end] (oldest first storage), newest first out.
func (b *Buffer) pageLocked(end, limit int) ([]Entry, uint64) {
	if limit <= 0 {
		limit = DefaultPageSize
	}

	if limit > MaxPageSize {
		limit = MaxPageSize
	}

	start := end - limit
	if start < 0 {
		start = 0
	}

	out := make([]Entry, 0, end-start)
	for i := end - 1; i >= start; i-- {
		out = append(out, b.entries[i])
	}

	var next uint64
	if start > 0 {
		next = b.entries[start].Seq
	}

	return out, next
}

// Len returns the number of buffered entries.
func (b *Buffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return len(b.entries)
}

// TeeHandler returns a handler that writes records to w (text format)
// and stores them in buf.
func TeeHandler(w io.Writer, opts *slog.HandlerOptions, buf *Buffer) slog.Handler {
	return &teeHandler{
		text:   slog.NewTextHandler(w, opts),
		memory: &memoryHandler{buf: buf},
	}
}

type teeHandler struct {
	text   slog.Handler
	memory *memoryHandler
}

func (h *teeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.text.Enabled(ctx, level)
}

func (h *teeHandler) Handle(ctx context.Context, record slog.Record) error {
	_ = h.memory.Handle(ctx, record)

	return h.text.Handle(ctx, record)
}

func (h *teeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &teeHandler{
		text:   h.text.WithAttrs(attrs),
		memory: h.memory.WithAttrs(attrs),
	}
}

func (h *teeHandler) WithGroup(name string) slog.Handler {
	return &teeHandler{
		text:   h.text.WithGroup(name),
		memory: h.memory.WithGroup(name),
	}
}

type memoryHandler struct {
	buf    *Buffer
	attrs  []slog.Attr
	groups []string
}

func (h *memoryHandler) Enabled(_ context.Context, level slog.Level) bool {
	// The viewer keeps only warnings and errors. Info records
	// (e.g. per-request logs) still go to stderr via the tee,
	// so the buffer stays small and relevant.
	return level >= slog.LevelWarn
}

func (h *memoryHandler) Handle(ctx context.Context, record slog.Record) error {
	if !h.Enabled(ctx, record.Level) {
		return nil
	}

	var parts []string

	var appendAttr func(prefix string, a slog.Attr)
	appendAttr = func(prefix string, a slog.Attr) {
		a.Value = a.Value.Resolve()
		if a.Value.Kind() == slog.KindGroup {
			for _, sub := range a.Value.Group() {
				appendAttr(prefix+a.Key+".", sub)
			}

			return
		}

		parts = append(parts, prefix+a.Key+"="+fmt.Sprintf("%v", a.Value.Any()))
	}

	prefix := ""
	if len(h.groups) > 0 {
		prefix = strings.Join(h.groups, ".") + "."
	}

	for _, a := range h.attrs {
		appendAttr(prefix, a)
	}

	record.Attrs(func(a slog.Attr) bool {
		appendAttr(prefix, a)

		return true
	})

	h.buf.Append(Entry{
		TimeLabel: record.Time.Format("15:04:05"),
		Level:     record.Level.String(),
		Message:   record.Message,
		Attrs:     strings.Join(parts, " "),
	})

	return nil
}

func (h *memoryHandler) WithAttrs(attrs []slog.Attr) *memoryHandler {
	return &memoryHandler{
		buf:    h.buf,
		attrs:  append(append([]slog.Attr(nil), h.attrs...), attrs...),
		groups: h.groups,
	}
}

func (h *memoryHandler) WithGroup(name string) *memoryHandler {
	return &memoryHandler{
		buf:    h.buf,
		attrs:  h.attrs,
		groups: append(append([]string(nil), h.groups...), name),
	}
}
