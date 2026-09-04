-- name: CreateSession :one
INSERT INTO sessions (token_hash, user_id, expires_at)
VALUES (?, ?, ?)
RETURNING token_hash, user_id, expires_at, created_at;

-- name: GetSessionByTokenHash :one
SELECT token_hash, user_id, expires_at, created_at
FROM sessions
WHERE token_hash = ?;

-- name: DeleteSession :exec
DELETE FROM sessions
WHERE token_hash = ?;

-- name: DeleteSessionsByUserID :exec
DELETE FROM sessions
WHERE user_id = ?;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions
WHERE expires_at <= CURRENT_TIMESTAMP;
