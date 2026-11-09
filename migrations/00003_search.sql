-- Search: a full-text index with one row per Bookmark, whose rowid is the
-- Bookmark's ID. It holds its own copy of the searched text, because the Tags
-- live in other tables. The triggers below keep it current, so the app never
-- writes to it.

-- +goose Up
CREATE VIRTUAL TABLE bookmarks_fts USING fts5(
    title, description, url, notes, tags,
    tokenize = 'unicode61 remove_diacritics 2'
);

INSERT INTO bookmarks_fts (rowid, title, description, url, notes, tags)
SELECT b.id, b.title, b.description, b.url, b.notes,
       coalesce((SELECT group_concat(t.name, ' ') FROM bookmark_tags bt JOIN tags t ON t.id = bt.tag_id
                  WHERE bt.bookmark_id = b.id), '')
FROM bookmarks b;

-- Each trigger rebuilds the rows of the Bookmarks it touched. A deleted
-- Bookmark's row is removed and, with the Bookmark gone, not rebuilt.

-- +goose StatementBegin
CREATE TRIGGER bookmarks_fts_insert AFTER INSERT ON bookmarks
BEGIN
    INSERT INTO bookmarks_fts (rowid, title, description, url, notes, tags)
    VALUES (NEW.id, NEW.title, NEW.description, NEW.url, NEW.notes, '');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER bookmarks_fts_update AFTER UPDATE OF title, description, url, notes ON bookmarks
BEGIN
    DELETE FROM bookmarks_fts WHERE rowid = OLD.id;
    INSERT INTO bookmarks_fts (rowid, title, description, url, notes, tags)
    SELECT b.id, b.title, b.description, b.url, b.notes,
           coalesce((SELECT group_concat(t.name, ' ') FROM bookmark_tags bt JOIN tags t ON t.id = bt.tag_id
                      WHERE bt.bookmark_id = b.id), '')
    FROM bookmarks b WHERE b.id = NEW.id;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER bookmarks_fts_delete AFTER DELETE ON bookmarks
BEGIN
    DELETE FROM bookmarks_fts WHERE rowid = OLD.id;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER bookmarks_fts_tag_added AFTER INSERT ON bookmark_tags
BEGIN
    DELETE FROM bookmarks_fts WHERE rowid = NEW.bookmark_id;
    INSERT INTO bookmarks_fts (rowid, title, description, url, notes, tags)
    SELECT b.id, b.title, b.description, b.url, b.notes,
           coalesce((SELECT group_concat(t.name, ' ') FROM bookmark_tags bt JOIN tags t ON t.id = bt.tag_id
                      WHERE bt.bookmark_id = b.id), '')
    FROM bookmarks b WHERE b.id = NEW.bookmark_id;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER bookmarks_fts_tag_removed AFTER DELETE ON bookmark_tags
BEGIN
    DELETE FROM bookmarks_fts WHERE rowid = OLD.bookmark_id;
    INSERT INTO bookmarks_fts (rowid, title, description, url, notes, tags)
    SELECT b.id, b.title, b.description, b.url, b.notes,
           coalesce((SELECT group_concat(t.name, ' ') FROM bookmark_tags bt JOIN tags t ON t.id = bt.tag_id
                      WHERE bt.bookmark_id = b.id), '')
    FROM bookmarks b WHERE b.id = OLD.bookmark_id;
END;
-- +goose StatementEnd

-- A renamed Tag changes every Bookmark that has it. (Merging Tags moves rows
-- in bookmark_tags, which the two triggers above already cover.)
-- +goose StatementBegin
CREATE TRIGGER bookmarks_fts_tag_renamed AFTER UPDATE OF name ON tags
BEGIN
    DELETE FROM bookmarks_fts
    WHERE rowid IN (SELECT bookmark_id FROM bookmark_tags WHERE tag_id = NEW.id);
    INSERT INTO bookmarks_fts (rowid, title, description, url, notes, tags)
    SELECT b.id, b.title, b.description, b.url, b.notes,
           coalesce((SELECT group_concat(t.name, ' ') FROM bookmark_tags bt JOIN tags t ON t.id = bt.tag_id
                      WHERE bt.bookmark_id = b.id), '')
    FROM bookmarks b
    WHERE b.id IN (SELECT bookmark_id FROM bookmark_tags WHERE tag_id = NEW.id);
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER bookmarks_fts_tag_renamed;
DROP TRIGGER bookmarks_fts_tag_removed;
DROP TRIGGER bookmarks_fts_tag_added;
DROP TRIGGER bookmarks_fts_delete;
DROP TRIGGER bookmarks_fts_update;
DROP TRIGGER bookmarks_fts_insert;
DROP TABLE bookmarks_fts;
