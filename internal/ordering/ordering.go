// Package ordering owns the position bookkeeping of admin-sorted lists
// (albums, videoclips, gallery items): drag & drop reordering and
// inserting a new item first. Every write runs in one transaction, so a
// failure never leaves a half-applied order or a gap in the positions.
package ordering

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/tgustinak/pohkapela.sk/internal/db"
)

var (
	// ErrInvalid means the submitted ids are empty, malformed or duplicated.
	ErrInvalid = errors.New("ordering: invalid id list")
	// ErrStale means the submitted ids no longer match the stored list
	// (an item was added or removed meanwhile).
	ErrStale = errors.New("ordering: list changed")
)

// List is the seam to one position-ordered table: the queries that
// differ between albums, videos and gallery items. Positions start at 1.
type List struct {
	// IDs returns the ids in current display order.
	IDs func(ctx context.Context, q *db.Queries) ([]int64, error)
	// SetPosition moves one item to the given position.
	SetPosition func(ctx context.Context, q *db.Queries, id, position int64) error
	// Shift moves every item down by one, freeing position 1.
	Shift func(ctx context.Context, q *db.Queries) error
}

// Albums is the album list.
var Albums = List{
	IDs: func(ctx context.Context, q *db.Queries) ([]int64, error) {
		rows, err := q.ListAlbumsOrdered(ctx)
		if err != nil {
			return nil, err
		}

		ids := make([]int64, len(rows))
		for i, r := range rows {
			ids[i] = r.ID
		}

		return ids, nil
	},
	SetPosition: func(ctx context.Context, q *db.Queries, id, position int64) error {
		return q.UpdateAlbumPosition(ctx, db.UpdateAlbumPositionParams{Position: position, ID: id})
	},
	Shift: func(ctx context.Context, q *db.Queries) error { return q.ShiftAlbumPositions(ctx) },
}

// Videos is the videoclip playlist.
var Videos = List{
	IDs: func(ctx context.Context, q *db.Queries) ([]int64, error) {
		rows, err := q.ListVideosOrdered(ctx)
		if err != nil {
			return nil, err
		}

		ids := make([]int64, len(rows))
		for i, r := range rows {
			ids[i] = r.ID
		}

		return ids, nil
	},
	SetPosition: func(ctx context.Context, q *db.Queries, id, position int64) error {
		return q.UpdateVideoPosition(ctx, db.UpdateVideoPositionParams{Position: position, ID: id})
	},
	Shift: func(ctx context.Context, q *db.Queries) error { return q.ShiftVideoPositions(ctx) },
}

// GalleryItems is the gallery, one list mixing photos and videos.
var GalleryItems = List{
	IDs: func(ctx context.Context, q *db.Queries) ([]int64, error) {
		rows, err := q.ListGalleryItemsOrdered(ctx)
		if err != nil {
			return nil, err
		}

		ids := make([]int64, len(rows))
		for i, r := range rows {
			ids[i] = r.ID
		}

		return ids, nil
	},
	SetPosition: func(ctx context.Context, q *db.Queries, id, position int64) error {
		return q.UpdateGalleryItemPosition(ctx, db.UpdateGalleryItemPositionParams{Position: position, ID: id})
	},
	Shift: func(ctx context.Context, q *db.Queries) error { return q.ShiftGalleryItemPositions(ctx) },
}

// Reorder stores the order given by rawIDs (position 1 = first). The ids
// must be exactly the ids currently in the list: duplicates, malformed or
// non-positive values give ErrInvalid; missing or unknown ids give
// ErrStale. Either the whole new order is applied or none of it.
func Reorder(ctx context.Context, q *db.Queries, list List, rawIDs []string) error {
	ids, err := parseIDs(rawIDs)
	if err != nil {
		return err
	}

	return q.InTx(ctx, func(q *db.Queries) error {
		current, err := list.IDs(ctx, q)
		if err != nil {
			return err
		}

		if !sameSet(current, ids) {
			return ErrStale
		}

		for i, id := range ids {
			if err := list.SetPosition(ctx, q, id, int64(i+1)); err != nil {
				return err
			}
		}

		return nil
	})
}

// InsertFirst frees position 1 and runs insert, which must create the new
// item at position 1. If insert fails the shift is rolled back, so the
// positions never end up with a gap. insert's error is returned unchanged.
func InsertFirst(ctx context.Context, q *db.Queries, list List, insert func(q *db.Queries) error) error {
	return q.InTx(ctx, func(q *db.Queries) error {
		if err := list.Shift(ctx, q); err != nil {
			return err
		}

		return insert(q)
	})
}

func parseIDs(raw []string) ([]int64, error) {
	if len(raw) == 0 {
		return nil, ErrInvalid
	}

	ids := make([]int64, 0, len(raw))
	seen := make(map[int64]struct{}, len(raw))

	for _, r := range raw {
		id, err := strconv.ParseInt(strings.TrimSpace(r), 10, 64)
		if err != nil || id <= 0 {
			return nil, ErrInvalid
		}

		if _, dup := seen[id]; dup {
			return nil, ErrInvalid
		}

		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	return ids, nil
}

// sameSet reports whether a and b contain the same ids (b has no duplicates).
func sameSet(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}

	known := make(map[int64]struct{}, len(a))
	for _, id := range a {
		known[id] = struct{}{}
	}

	for _, id := range b {
		if _, ok := known[id]; !ok {
			return false
		}
	}

	return true
}
