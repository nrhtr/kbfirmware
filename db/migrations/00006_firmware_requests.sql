-- +goose Up
CREATE TABLE firmware_request (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    pcb_name     TEXT    NOT NULL,
    firmware_url TEXT    NOT NULL DEFAULT '',
    notes        TEXT    NOT NULL DEFAULT '',
    contact      TEXT    NOT NULL DEFAULT '',
    ip_hash      TEXT    NOT NULL DEFAULT '',
    created_at   INTEGER NOT NULL DEFAULT (unixepoch()),
    resolved     INTEGER NOT NULL DEFAULT 0
);

-- +goose Down
DROP TABLE firmware_request;
