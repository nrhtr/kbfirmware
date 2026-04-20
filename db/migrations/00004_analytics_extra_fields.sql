-- +goose Up
ALTER TABLE analytics_event ADD COLUMN path         TEXT NOT NULL DEFAULT '';
ALTER TABLE analytics_event ADD COLUMN referrer     TEXT NOT NULL DEFAULT '';
ALTER TABLE analytics_event ADD COLUMN search_query TEXT NOT NULL DEFAULT '';

-- +goose Down
-- SQLite doesn't support DROP COLUMN cleanly; recreate without the columns
CREATE TABLE analytics_event_new (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    type       TEXT    NOT NULL CHECK(type IN ('visit','download')),
    file_id    INTEGER REFERENCES firmware_file(id) ON DELETE SET NULL,
    ip_hash    TEXT    NOT NULL DEFAULT '',
    country    TEXT    NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL DEFAULT (unixepoch())
);
INSERT INTO analytics_event_new SELECT id, type, file_id, ip_hash, country, created_at FROM analytics_event;
DROP TABLE analytics_event;
ALTER TABLE analytics_event_new RENAME TO analytics_event;
