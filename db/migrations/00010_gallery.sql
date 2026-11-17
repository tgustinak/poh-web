-- +goose Up
-- Gallery items are either an uploaded photo or a YouTube video.
-- Both kinds share one ordered list (position 1 = first shown), so photos
-- and videos are displayed on the same level in the public gallery.
-- Photo items use file_path + preview_path + alt; video items use
-- youtube_id + title. The CHECK constraint enforces exactly one source.
CREATE TABLE gallery_items (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  kind TEXT NOT NULL CHECK (kind IN ('photo', 'video')),
  file_path TEXT NOT NULL DEFAULT '',
  preview_path TEXT NOT NULL DEFAULT '',
  youtube_id TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '',
  alt TEXT NOT NULL DEFAULT '',
  position INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK (
    (kind = 'photo' AND file_path <> '' AND youtube_id = '')
    OR
    (kind = 'video' AND youtube_id <> '' AND file_path = '')
  )
);

CREATE INDEX idx_gallery_items_position ON gallery_items(position, id);

-- +goose Down
DROP INDEX IF EXISTS idx_gallery_items_position;
DROP TABLE IF EXISTS gallery_items;
