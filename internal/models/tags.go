package models

import (
	"context"
	"database/sql"
	"errors"
)

// TagModelInterface - the tag methods used by the handlers. Like bookmarks,
// another user's tag behaves as if it does not exist (ErrNoRecord).
type TagModelInterface interface {
	Get(userID, id int) (*Tag, error)
	List(userID int) ([]*Tag, error)
	Rename(userID, id int, name string) error
	Merge(userID, id int, name string) error
}

// Tag is a user's label, with the number of bookmarks using it.
type Tag struct {
	ID            int
	Name          string
	BookmarkCount int
}

// TagModel wraps a database connection pool.
type TagModel struct {
	DB *sql.DB
}

const tagSelect = `SELECT t.id, t.name,
	(SELECT count(*) FROM bookmark_tags bt WHERE bt.tag_id = t.id)
	FROM tags t WHERE t.user_id = ?`

// Get returns one of the user's tags.
func (m *TagModel) Get(userID, id int) (*Tag, error) {
	t := &Tag{}
	err := m.DB.QueryRow(tagSelect+` AND t.id = ?`, userID, id).Scan(&t.ID, &t.Name, &t.BookmarkCount)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}
	return t, nil
}

// List returns all the user's tags, by name.
func (m *TagModel) List(userID int) ([]*Tag, error) {
	rows, err := m.DB.Query(tagSelect+` ORDER BY t.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tags := []*Tag{}
	for rows.Next() {
		t := &Tag{}
		if err = rows.Scan(&t.ID, &t.Name, &t.BookmarkCount); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return tags, nil
}

// Rename changes a tag's name, which every bookmark using it follows. The name
// must already be normalised. Returns ErrDuplicateTag if the user already has a
// tag with that name.
func (m *TagModel) Rename(userID, id int, name string) error {
	result, err := m.DB.Exec(`UPDATE tags SET name = ? WHERE user_id = ? AND id = ?`, name, userID, id)
	if isUniqueViolation(err, "tags.user_id, tags.name") {
		return ErrDuplicateTag
	}
	return checkAffected(result, err)
}

// Merge moves the tag's bookmarks to the user's existing tag with the given
// name. Removing the last link deletes the old tag, through the
// tags_remove_unused trigger. A bookmark with both tags keeps just the target.
func (m *TagModel) Merge(userID, id int, name string) error {
	ctx := context.Background()
	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op after Commit

	_, err = tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO bookmark_tags (bookmark_id, tag_id)
		 SELECT bt.bookmark_id, target.id
		 FROM bookmark_tags bt
		 JOIN tags source ON source.id = bt.tag_id
		 JOIN tags target ON target.user_id = source.user_id
		 WHERE source.user_id = ? AND source.id = ? AND target.name = ? AND target.id <> source.id`,
		userID, id, name)
	if err != nil {
		return err
	}

	// Check both tags exist, and belong to this user, before removing anything.
	var found int
	err = tx.QueryRowContext(ctx,
		`SELECT count(*) FROM tags
		 WHERE user_id = ? AND ((id = ? AND name <> ?) OR name = ?)`,
		userID, id, name, name).Scan(&found)
	if err != nil {
		return err
	}
	if found != 2 {
		return ErrNoRecord
	}

	_, err = tx.ExecContext(ctx,
		`DELETE FROM bookmark_tags
		 WHERE tag_id = (SELECT id FROM tags WHERE user_id = ? AND id = ?)`, userID, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}
