-- +goose Up
ALTER TABLE sessions ADD COLUMN csrf_token TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE sessions DROP COLUMN csrf_token;
