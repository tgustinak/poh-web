-- +goose Up
-- published_date is TEXT in ISO-8601 (YYYY-MM-DD): it sorts chronologically
-- as plain text, with no timezone or driver formatting concerns
-- (SQLite has no dedicated DATE type anyway).
-- position defines the playlist order shown on the web (1 = first).
CREATE TABLE videos (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  youtube_id TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  published_date TEXT NOT NULL CHECK (published_date GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
  duration TEXT NOT NULL,
  position INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_videos_position ON videos(position, id);

INSERT INTO videos (youtube_id, title, published_date, duration, position) VALUES
  ('7Msu0l_5Pi0', 'Nechcem zabudnúť', '2026-06-19', '4:24', 1),
  ('ylJqEmM-Sjo', 'Kubín', '2025-04-16', '3:20', 2),
  ('BB56qBn2wtA', 'Mám otázku', '2024-02-13', '3:42', 3),
  ('AxT7ni2COK4', 'Nenechaj ma ísť', '2024-01-28', '3:38', 4),
  ('7tgNr5sWXds', 'Vrátiť sa', '2023-07-12', '4:42', 5),
  ('AWmVqHcHn5c', 'Na konci sveta', '2023-03-23', '4:34', 6),
  ('KKd_lv_zC5E', 'Spievaj mi', '2022-02-10', '4:24', 7),
  ('Qx7rHKYHRAE', 'Voz veľkých hviezd', '2019-08-25', '5:11', 8),
  ('nxsVT3s9ibM', 'Prečo', '2018-11-17', '4:42', 9);

-- +goose Down
DELETE FROM videos;
DROP INDEX IF EXISTS idx_videos_position;
DROP TABLE IF EXISTS videos;
