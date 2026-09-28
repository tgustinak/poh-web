-- +goose Up
-- Organizers page ("Pre organizátorov") files.
-- Documents are a fixed set of 3 slots (no add/delete from admin):
-- admin only replaces the file behind each slot. Uploaded files live
-- in the uploads directory (see UPLOAD_DIR); file_path starting with
-- /static/ points at the versioned seed files in web/static.
-- Promo photos are a free CRUD list (position = display order).
-- preview_path may equal file_path when no lightweight preview exists.
CREATE TABLE organizer_documents (
  slot TEXT PRIMARY KEY CHECK (slot IN ('rider', 'input-list', 'logo')),
  display_name TEXT NOT NULL,
  file_path TEXT NOT NULL,
  size_bytes INTEGER NOT NULL DEFAULT 0,
  position INTEGER NOT NULL UNIQUE,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO organizer_documents (slot, display_name, file_path, size_bytes, position) VALUES
  ('rider', 'Technický rider', '/static/files/poh-rider.pdf', 123158, 1),
  ('input-list', 'Input list', '/static/files/poh-input-list.pdf', 2967087, 2),
  ('logo', 'Logo v krivkách', '/static/files/poh-logo.pdf', 418238, 3);

CREATE TABLE organizer_photos (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  file_path TEXT NOT NULL,
  preview_path TEXT NOT NULL,
  alt TEXT NOT NULL DEFAULT '',
  meta_label TEXT NOT NULL DEFAULT '',
  position INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO organizer_photos (file_path, preview_path, alt, meta_label, position) VALUES
  ('/static/files/poh-promo-1.png', '/static/images/promo-1-preview.webp', 'Kapela P.O.H – promo foto 1', 'PNG • 2105 × 2560', 1),
  ('/static/files/poh-promo-2.jpg', '/static/images/promo-2-preview.webp', 'Kapela P.O.H – promo foto 2', 'JPG • 2560 × 2560', 2);

-- +goose Down
DROP TABLE IF EXISTS organizer_photos;
DROP TABLE IF EXISTS organizer_documents;
