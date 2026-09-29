-- name: ListAlbumsOrdered :many
SELECT id, title, year, badge, cover_path, is_new, position, created_at, updated_at
FROM albums
ORDER BY position ASC, id ASC;

-- name: GetAlbumByID :one
SELECT id, title, year, badge, cover_path, is_new, position, created_at, updated_at
FROM albums
WHERE id = ?;

-- name: CreateAlbum :one
INSERT INTO albums (title, year, badge, cover_path, is_new, position)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING id, title, year, badge, cover_path, is_new, position, created_at, updated_at;

-- name: UpdateAlbum :one
UPDATE albums
SET title = ?, year = ?, badge = ?, is_new = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING id, title, year, badge, cover_path, is_new, position, created_at, updated_at;

-- name: UpdateAlbumCover :one
UPDATE albums
SET cover_path = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING id, title, year, badge, cover_path, is_new, position, created_at, updated_at;

-- name: DeleteAlbum :exec
DELETE FROM albums
WHERE id = ?;

-- name: ShiftAlbumPositions :exec
UPDATE albums
SET position = position + 1;

-- name: UpdateAlbumPosition :exec
UPDATE albums
SET position = ?
WHERE id = ?;

-- name: CountAlbums :one
SELECT COUNT(*)
FROM albums;
