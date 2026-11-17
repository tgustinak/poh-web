-- name: ListGalleryItemsOrdered :many
SELECT id, kind, file_path, preview_path, youtube_id, title, alt, position, created_at
FROM gallery_items
ORDER BY position ASC, id ASC;

-- name: GetGalleryItemByID :one
SELECT id, kind, file_path, preview_path, youtube_id, title, alt, position, created_at
FROM gallery_items
WHERE id = ?;

-- name: LastGalleryItemPosition :one
SELECT position
FROM gallery_items
ORDER BY position DESC
LIMIT 1;

-- name: CreateGalleryItem :one
INSERT INTO gallery_items (kind, file_path, preview_path, youtube_id, title, alt, position)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING id, kind, file_path, preview_path, youtube_id, title, alt, position, created_at;

-- name: UpdateGalleryPhotoAlt :one
UPDATE gallery_items
SET alt = ?
WHERE id = ? AND kind = 'photo'
RETURNING id, kind, file_path, preview_path, youtube_id, title, alt, position, created_at;

-- name: UpdateGalleryVideo :one
UPDATE gallery_items
SET youtube_id = ?, title = ?
WHERE id = ? AND kind = 'video'
RETURNING id, kind, file_path, preview_path, youtube_id, title, alt, position, created_at;

-- name: DeleteGalleryItem :exec
DELETE FROM gallery_items
WHERE id = ?;

-- name: ShiftGalleryItemPositions :exec
UPDATE gallery_items
SET position = position + 1;

-- name: UpdateGalleryItemPosition :exec
UPDATE gallery_items
SET position = ?
WHERE id = ?;

-- name: CountGalleryItems :one
SELECT COUNT(*)
FROM gallery_items;
