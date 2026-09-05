-- name: CreateSession :one
INSERT INTO sessions (token_hash, user_id, csrf_token, expires_at)
VALUES (?, ?, ?, ?)
RETURNING token_hash, user_id, csrf_token, expires_at, created_at;

-- name: GetSessionByTokenHash :one
SELECT token_hash, user_id, csrf_token, expires_at, created_at
FROM sessions
WHERE token_hash = ?;

-- name: DeleteSession :exec
DELETE FROM sessions
WHERE token_hash = ?;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions
WHERE expires_at <= CURRENT_TIMESTAMP;

-- name: ListSessionsMissingCsrf :many
SELECT token_hash
FROM sessions
WHERE csrf_token = '';

-- name: UpdateSessionCsrf :exec
UPDATE sessions
SET csrf_token = ?
WHERE token_hash = ?;
