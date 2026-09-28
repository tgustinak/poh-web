-- name: ListOrganizerDocumentsOrdered :many
SELECT slot, display_name, file_path, size_bytes, position, updated_at
FROM organizer_documents
ORDER BY position ASC;

-- name: GetOrganizerDocumentBySlot :one
SELECT slot, display_name, file_path, size_bytes, position, updated_at
FROM organizer_documents
WHERE slot = ?;

-- name: UpdateOrganizerDocumentFile :one
UPDATE organizer_documents
SET file_path = ?, size_bytes = ?, updated_at = CURRENT_TIMESTAMP
WHERE slot = ?
RETURNING slot, display_name, file_path, size_bytes, position, updated_at;
