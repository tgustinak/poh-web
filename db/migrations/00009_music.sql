-- +goose Up
-- Music section (diskografia): free CRUD album list in display order
-- (position, drag & drop in admin like videoclips) plus a singleton
-- with the Spotify artist URL used by the player and links.
-- Cover paths starting with /static/ point at the versioned seed files
-- in web/static; uploads live in the uploads directory (see UPLOAD_DIR).
CREATE TABLE albums (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  year INTEGER NOT NULL CHECK (year BETWEEN 1900 AND 2100),
  badge TEXT NOT NULL,
  cover_path TEXT NOT NULL,
  is_new INTEGER NOT NULL DEFAULT 0 CHECK (is_new IN (0, 1)),
  position INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_albums_position ON albums(position, id);

INSERT INTO albums (title, year, badge, cover_path, is_new, position) VALUES
  ('Život', 2026, 'Novinka', '/static/images/album-zivot.webp', 1, 1),
  ('Na konci sveta', 2021, 'Debut', '/static/images/album-na-konci-sveta.jpg', 0, 2);

-- +goose Down
DROP TABLE IF EXISTS albums;
