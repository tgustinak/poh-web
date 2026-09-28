-- name: GetContact :one
SELECT id, name, role, phone, email, photo_path, updated_at
FROM contact
WHERE id = 1;

-- name: UpdateContact :one
UPDATE contact
SET name = ?, role = ?, phone = ?, email = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = 1
RETURNING id, name, role, phone, email, photo_path, updated_at;

-- name: UpdateContactPhoto :one
UPDATE contact
SET photo_path = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = 1
RETURNING id, name, role, phone, email, photo_path, updated_at;
