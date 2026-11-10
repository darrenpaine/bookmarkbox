-- Imports: a bookmark file a User has uploaded and is previewing, waiting for
-- them to confirm it. Each User has at most one. The file is kept as uploaded
-- and read again on confirm, so the preview and the import always agree.

-- +goose Up
CREATE TABLE imports (
    user_id    INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    filename   TEXT NOT NULL,
    data       BLOB NOT NULL,
    created_at INTEGER NOT NULL -- seconds since 1970, to compare with an expiry time
);

-- +goose Down
DROP TABLE imports;
