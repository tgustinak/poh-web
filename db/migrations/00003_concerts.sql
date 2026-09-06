-- +goose Up
-- event_date is TEXT in ISO-8601 (YYYY-MM-DD): it sorts chronologically
-- as plain text, with no timezone or driver formatting concerns
-- (SQLite has no dedicated DATE type anyway).
CREATE TABLE concerts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  event_date TEXT NOT NULL CHECK (event_date GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_concerts_event_date ON concerts(event_date);

-- +goose Down
DROP INDEX IF EXISTS idx_concerts_event_date;
DROP TABLE IF EXISTS concerts;
