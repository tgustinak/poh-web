-- name: ListUpcomingConcerts :many
SELECT id, name, event_date, created_at
FROM concerts
WHERE event_date >= ?
ORDER BY event_date ASC;

-- name: ListPastConcerts :many
SELECT id, name, event_date, created_at
FROM concerts
WHERE event_date < ?
ORDER BY event_date DESC;

-- name: ListAllConcerts :many
SELECT id, name, event_date, created_at
FROM concerts
ORDER BY event_date ASC;

-- name: GetConcertByID :one
SELECT id, name, event_date, created_at
FROM concerts
WHERE id = ?;

-- name: CreateConcert :one
INSERT INTO concerts (name, event_date)
VALUES (?, ?)
RETURNING id, name, event_date, created_at;

-- name: UpdateConcert :one
UPDATE concerts
SET name = ?, event_date = ?
WHERE id = ?
RETURNING id, name, event_date, created_at;

-- name: DeleteConcert :exec
DELETE FROM concerts
WHERE id = ?;

-- name: CountUpcomingConcerts :one
SELECT COUNT(*)
FROM concerts
WHERE event_date >= ?;
