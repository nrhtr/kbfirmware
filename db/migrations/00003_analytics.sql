-- +goose Up
CREATE TABLE analytics_event (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    type       TEXT    NOT NULL CHECK(type IN ('visit','download')),
    file_id    INTEGER REFERENCES firmware_file(id) ON DELETE SET NULL,
    ip_hash    TEXT    NOT NULL DEFAULT '',
    country    TEXT    NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE INDEX analytics_event_type_created ON analytics_event(type, created_at);
CREATE INDEX analytics_event_file_id      ON analytics_event(file_id);

-- +goose Down
DROP TABLE analytics_event;
