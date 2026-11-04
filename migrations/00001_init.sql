-- +goose Up
CREATE TABLE users (
    id              INTEGER PRIMARY KEY,
    name            TEXT NOT NULL,
    email           TEXT NOT NULL UNIQUE COLLATE NOCASE,
    hashed_password TEXT NOT NULL,
    created         DATETIME NOT NULL
);

-- Session table in the layout expected by scs/sqlite3store.
CREATE TABLE sessions (
    token  TEXT PRIMARY KEY,
    data   BLOB NOT NULL,
    expiry REAL NOT NULL
);
CREATE INDEX sessions_expiry_idx ON sessions (expiry);

CREATE TABLE bookmarks (
    id          INTEGER PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    url         TEXT NOT NULL,
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    notes       TEXT NOT NULL DEFAULT '',
    favicon     TEXT NOT NULL DEFAULT '',
    favourite   BOOLEAN NOT NULL DEFAULT FALSE,
    read_later  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  DATETIME NOT NULL,
    UNIQUE (user_id, url)
);
CREATE INDEX bookmarks_user_created_idx ON bookmarks (user_id, created_at);

CREATE TABLE tags (
    id      INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name    TEXT NOT NULL,
    UNIQUE (user_id, name)
);

CREATE TABLE bookmark_tags (
    bookmark_id INTEGER NOT NULL REFERENCES bookmarks(id) ON DELETE CASCADE,
    tag_id      INTEGER NOT NULL REFERENCES tags(id)      ON DELETE CASCADE,
    PRIMARY KEY (bookmark_id, tag_id)
);
CREATE INDEX bookmark_tags_tag_idx ON bookmark_tags (tag_id);

-- +goose Down
DROP TABLE bookmark_tags;
DROP TABLE tags;
DROP TABLE bookmarks;
DROP TABLE sessions;
DROP TABLE users;
