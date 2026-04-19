-- +goose Up
CREATE TABLE IF NOT EXISTS pcb (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    revision TEXT NOT NULL DEFAULT '',
    designer TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE TABLE IF NOT EXISTS firmware_entry (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pcb_id INTEGER NOT NULL REFERENCES pcb(id) ON DELETE CASCADE,
    firmware_name TEXT NOT NULL,
    source_url TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE TABLE IF NOT EXISTS firmware_file (
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

CREATE TABLE IF NOT EXISTS tag (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS firmware_entry_tag (
    firmware_entry_id INTEGER NOT NULL REFERENCES firmware_entry(id) ON DELETE CASCADE,
    tag_id INTEGER NOT NULL REFERENCES tag(id) ON DELETE CASCADE,
    PRIMARY KEY (firmware_entry_id, tag_id)
);

CREATE TABLE IF NOT EXISTS flag (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    firmware_entry_id INTEGER NOT NULL REFERENCES firmware_entry(id) ON DELETE CASCADE,
    reason TEXT NOT NULL DEFAULT '',
    reporter_ip TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    resolved INTEGER NOT NULL DEFAULT 0,
    resolution_notes TEXT NOT NULL DEFAULT ''
);

-- +goose Down
DROP TABLE IF EXISTS flag;
DROP TABLE IF EXISTS firmware_entry_tag;
DROP TABLE IF EXISTS tag;
DROP TABLE IF EXISTS firmware_file;
DROP TABLE IF EXISTS firmware_entry;
DROP TABLE IF EXISTS pcb;
