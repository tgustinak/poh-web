-- name: GetAbout :one
SELECT id, p1, p2, quote, updated_at
FROM about
WHERE id = 1;

-- name: UpdateAbout :one
UPDATE about
SET p1 = ?, p2 = ?, quote = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = 1
RETURNING id, p1, p2, quote, updated_at;
