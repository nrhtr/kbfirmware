-- +goose Up
ALTER TABLE firmware_file ADD COLUMN compressed INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite doesn't support DROP COLUMN in older versions; recreate without it
CREATE TABLE firmware_file_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    firmware_entry_id INTEGER NOT NULL REFERENCES firmware_entry(id) ON DELETE CASCADE,
    file_tag TEXT NOT NULL,
    filename TEXT NOT NULL,
    mime_type TEXT NOT NULL DEFAULT 'application/octet-stream',
    sha256 TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    data BLOB NOT NULL,
    uploaded_at INTEGER NOT NULL DEFAULT (unixepoch())
);
INSERT INTO firmware_file_new SELECT id, firmware_entry_id, file_tag, filename, mime_type, sha256, size_bytes, data, uploaded_at FROM firmware_file;
DROP TABLE firmware_file;
ALTER TABLE firmware_file_new RENAME TO firmware_file;
