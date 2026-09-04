-- name: CreateUser :one
INSERT INTO users (email, password_hash)
VALUES (?, ?)
RETURNING id, email, password_hash, created_at;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, created_at
FROM users
WHERE email = ?;

-- name: GetUserByID :one
SELECT id, email, password_hash, created_at
FROM users
WHERE id = ?;

-- name: CountUsers :one
SELECT COUNT(*)
FROM users;
