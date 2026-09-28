-- name: ListOrganizerPhotosOrdered :many
SELECT id, file_path, preview_path, alt, meta_label, position, created_at
FROM organizer_photos
ORDER BY position ASC, id ASC;

-- name: GetOrganizerPhotoByID :one
SELECT id, file_path, preview_path, alt, meta_label, position, created_at
FROM organizer_photos
WHERE id = ?;

-- name: LastOrganizerPhotoPosition :one
SELECT position
FROM organizer_photos
ORDER BY position DESC
LIMIT 1;

-- name: CreateOrganizerPhoto :one
INSERT INTO organizer_photos (file_path, preview_path, alt, meta_label, position)
VALUES (?, ?, ?, ?, ?)
RETURNING id, file_path, preview_path, alt, meta_label, position, created_at;

-- name: UpdateOrganizerPhotoAlt :one
UPDATE organizer_photos
SET alt = ?
WHERE id = ?
RETURNING id, file_path, preview_path, alt, meta_label, position, created_at;

-- name: DeleteOrganizerPhoto :exec
DELETE FROM organizer_photos
WHERE id = ?;
