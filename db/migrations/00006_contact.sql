-- +goose Up
-- Contact section is a singleton row (id = 1): manager name, role,
-- phone (E.164 in tel: link), e-mail and an optional photo.
-- photo_path is '' when there is no photo yet (e.g. a new manager
-- without a picture) — the site then shows a silhouette placeholder.
-- Uploaded photos live in the uploads directory (see UPLOAD_DIR),
-- never in web/static.
-- Seeded with the current hardcoded homepage values (no photo).
CREATE TABLE contact (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  name TEXT NOT NULL,
  role TEXT NOT NULL,
  phone TEXT NOT NULL,
  email TEXT NOT NULL,
  photo_path TEXT NOT NULL DEFAULT '',
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO contact (id, name, role, phone, email, photo_path) VALUES (
  1,
  'Jana Testová',
  'Manažérka',
  '+421900123456',
  'kontakt@example.com',
  ''
);

-- +goose Down
DROP TABLE IF EXISTS contact;
