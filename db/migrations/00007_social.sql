-- +goose Up
-- Social section is a fixed set of 6 rows (no add/delete from admin):
-- each platform has a fixed display name and position, admin edits
-- only handle and url. Seeded with the current hardcoded homepage values.
-- CTA text is derived from the platform in code, not stored.
CREATE TABLE social_links (
  platform TEXT PRIMARY KEY CHECK (platform IN ('facebook', 'instagram', 'youtube', 'spotify', 'ytmusic', 'apple')),
  display_name TEXT NOT NULL,
  handle TEXT NOT NULL,
  url TEXT NOT NULL CHECK (url LIKE 'https://%'),
  position INTEGER NOT NULL UNIQUE,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO social_links (platform, display_name, handle, url, position) VALUES
  ('facebook', 'Facebook', 'kapelaPOH', 'https://www.facebook.com/kapelaPOH/', 1),
  ('instagram', 'Instagram', '@kapela_p.o.h', 'https://www.instagram.com/kapela_p.o.h/', 2),
  ('youtube', 'YouTube', '@p.o.h6377', 'https://www.youtube.com/@p.o.h6377', 3),
  ('spotify', 'Spotify', 'P.O.H', 'https://open.spotify.com/artist/79t3KrQngTRDooegKsR4cw', 4),
  ('ytmusic', 'YouTube Music', 'P.O.H', 'https://music.youtube.com/channel/UCWK7N2Ir9r8YuyMXScgs0dw', 5),
  ('apple', 'Apple Music', 'P.O.H', 'https://music.apple.com/us/artist/p-o-h/6788148500', 6);

-- +goose Down
DROP TABLE IF EXISTS social_links;
