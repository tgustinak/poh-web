-- name: ListVideosOrdered :many
SELECT id, youtube_id, title, published_date, duration, position, created_at
FROM videos
ORDER BY position ASC, id ASC;

-- name: GetVideoByID :one
SELECT id, youtube_id, title, published_date, duration, position, created_at
FROM videos
WHERE id = ?;

-- name: CreateVideo :one
INSERT INTO videos (youtube_id, title, published_date, duration, position)
VALUES (?, ?, ?, ?, ?)
RETURNING id, youtube_id, title, published_date, duration, position, created_at;

-- name: UpdateVideo :one
UPDATE videos
SET youtube_id = ?, title = ?, published_date = ?, duration = ?
WHERE id = ?
RETURNING id, youtube_id, title, published_date, duration, position, created_at;

-- name: DeleteVideo :exec
DELETE FROM videos
WHERE id = ?;

-- name: ShiftVideoPositions :exec
UPDATE videos
SET position = position + 1;

-- name: UpdateVideoPosition :exec
UPDATE videos
SET position = ?
WHERE id = ?;

-- name: CountVideos :one
SELECT COUNT(*)
FROM videos;
