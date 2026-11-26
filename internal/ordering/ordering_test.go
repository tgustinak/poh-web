package ordering_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/tgustinak/pohkapela.sk/internal/database"
	"github.com/tgustinak/pohkapela.sk/internal/db"
	"github.com/tgustinak/pohkapela.sk/internal/ordering"
)

// seedVideos creates n videos at positions 1..n and returns their ids in order.
func seedVideos(t *testing.T, n int) (*db.Queries, []int64) {
	t.Helper()

	conn, err := database.Open(t.Context(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close() })

	// Migrations seed some clips; start from an empty list.
	if _, err := conn.ExecContext(t.Context(), "DELETE FROM videos"); err != nil {
		t.Fatalf("clear videos: %v", err)
	}

	q := db.New(conn)
	ids := make([]int64, 0, n)

	for i := 1; i <= n; i++ {
		v, err := q.CreateVideo(t.Context(), db.CreateVideoParams{
			YoutubeID:     "aaaaaaaaaa" + strconv.Itoa(i),
			Title:         "clip " + strconv.Itoa(i),
			PublishedDate: "2024-01-01",
			Duration:      "3:00",
			Position:      int64(i),
		})
		if err != nil {
			t.Fatalf("seed video: %v", err)
		}

		ids = append(ids, v.ID)
	}

	return q, ids
}

func order(t *testing.T, q *db.Queries) []int64 {
	t.Helper()

	ids, err := ordering.Videos.IDs(t.Context(), q)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	return ids
}

func strs(ids ...int64) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = strconv.FormatInt(id, 10)
	}

	return out
}

func TestReorderAppliesNewOrder(t *testing.T) {
	q, ids := seedVideos(t, 3)
	want := []int64{ids[2], ids[0], ids[1]}

	if err := ordering.Reorder(t.Context(), q, ordering.Videos, strs(want...)); err != nil {
		t.Fatalf("reorder: %v", err)
	}

	if got := order(t, q); !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestReorderRejectsBadInput(t *testing.T) {
	q, ids := seedVideos(t, 3)

	invalid := [][]string{
		nil,
		{"abc", "1", "2"},
		{"0", strconv.FormatInt(ids[0], 10), strconv.FormatInt(ids[1], 10)},
		strs(ids[0], ids[0], ids[1]),
	}
	for _, raw := range invalid {
		if err := ordering.Reorder(t.Context(), q, ordering.Videos, raw); !errors.Is(err, ordering.ErrInvalid) {
			t.Errorf("%v: err = %v, want ErrInvalid", raw, err)
		}
	}

	stale := [][]string{
		strs(ids[0], ids[1]),                 // missing one
		strs(ids[0], ids[1], ids[2], 999999), // extra one
		strs(ids[0], ids[1], 999999),         // unknown one
	}
	for _, raw := range stale {
		if err := ordering.Reorder(t.Context(), q, ordering.Videos, raw); !errors.Is(err, ordering.ErrStale) {
			t.Errorf("%v: err = %v, want ErrStale", raw, err)
		}
	}

	if got := order(t, q); !slices.Equal(got, ids) {
		t.Fatalf("rejected input must not change the order, got %v want %v", got, ids)
	}
}

func TestReorderIsAtomic(t *testing.T) {
	q, ids := seedVideos(t, 3)

	calls := 0
	failing := ordering.Videos
	failing.SetPosition = func(ctx context.Context, q *db.Queries, id, position int64) error {
		calls++
		if calls == 2 {
			return errors.New("boom")
		}

		return ordering.Videos.SetPosition(ctx, q, id, position)
	}

	err := ordering.Reorder(t.Context(), q, failing, strs(ids[2], ids[1], ids[0]))
	if err == nil {
		t.Fatal("expected error")
	}

	if got := order(t, q); !slices.Equal(got, ids) {
		t.Fatalf("failed reorder must roll back, got %v want %v", got, ids)
	}
}

func TestInsertFirstShiftsExistingItems(t *testing.T) {
	q, ids := seedVideos(t, 2)

	var created int64

	err := ordering.InsertFirst(t.Context(), q, ordering.Videos, func(q *db.Queries) error {
		v, err := q.CreateVideo(t.Context(), db.CreateVideoParams{
			YoutubeID: "bbbbbbbbbbb", Title: "new", PublishedDate: "2024-02-02", Duration: "1:00", Position: 1,
		})
		created = v.ID

		return err
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	want := []int64{created, ids[0], ids[1]}
	if got := order(t, q); !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestInsertFirstRollsBackShiftOnFailure(t *testing.T) {
	q, ids := seedVideos(t, 2)
	insertErr := errors.New("insert failed")

	err := ordering.InsertFirst(t.Context(), q, ordering.Videos, func(*db.Queries) error { return insertErr })
	if !errors.Is(err, insertErr) {
		t.Fatalf("err = %v, want insert error unchanged", err)
	}

	rows, err := q.ListVideosOrdered(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	for i, v := range rows {
		if v.ID != ids[i] || v.Position != int64(i+1) {
			t.Fatalf("positions must be unchanged, got id=%d pos=%d at index %d", v.ID, v.Position, i)
		}
	}
}
