package models

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"time"
)

// BookmarkModelInterface - the bookmark methods used by the handlers.
// Every method takes the owner's user ID: a bookmark belonging to another user
// behaves as if it does not exist (ErrNoRecord).
type BookmarkModelInterface interface {
	Insert(userID int, b *Bookmark) (int, error)
	Get(userID, id int) (*Bookmark, error)
	Update(userID int, b *Bookmark) error
	Latest(userID, limit int) ([]*Bookmark, error)
	ToggleFavourite(userID, id int) error
	Delete(userID, id int) error
}

// Maximum lengths, in characters. Shared so that the forms, and later the
// importer and API, all enforce the same numbers.
const (
	MaxTitleChars       = 200
	MaxURLChars         = 2000
	MaxDescriptionChars = 2000
	MaxNotesChars       = 10000
	MaxTagNameChars     = 50
)

// Bookmark holds one saved link and its tags.
type Bookmark struct {
	ID          int
	UserID      int
	URL         string
	Title       string
	Description string
	Notes       string
	Favicon     string
	Favourite   bool
	ReadLater   bool
	Created     time.Time
	Tags        []string
}

// BookmarkModel wraps a database connection pool. The limits are per user, and
// 0 means unlimited.
type BookmarkModel struct {
	DB           *sql.DB
	MaxBookmarks int
	MaxTags      int
}

// NormaliseTags trims, lower-cases, de-duplicates and sorts tag names,
// dropping empty ones.
func NormaliseTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	slices.Sort(out)
	return out
}

// Insert saves a new bookmark and its tags in one transaction.
// Returns ErrDuplicateURL if it would be a Duplicate of one of the user's
// bookmarks, or ErrBookmarkLimit or ErrTagLimit. A failed save changes nothing.
func (m *BookmarkModel) Insert(userID int, b *Bookmark) (int, error) {
	ctx := context.Background()
	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() // no-op after Commit

	normalisedURL, err := NormaliseURL(b.URL)
	if err != nil {
		return 0, err
	}
	created := b.Created
	if created.IsZero() {
		created = time.Now()
	}
	stmt := `INSERT INTO bookmarks
	         (user_id, url, normalised_url, title, description, notes, favicon, favourite, read_later, created_at)
	         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	result, err := tx.ExecContext(ctx, stmt, userID, b.URL, normalisedURL, b.Title, b.Description, b.Notes,
		b.Favicon, b.Favourite, b.ReadLater, created.UTC())
	if err != nil {
		if isUniqueViolation(err, "bookmarks.user_id, bookmarks.normalised_url") {
			return 0, ErrDuplicateURL
		}
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	over, err := overLimit(ctx, tx, m.MaxBookmarks, `SELECT count(*) FROM bookmarks WHERE user_id = ?`, userID)
	if err != nil {
		return 0, err
	}
	if over {
		return 0, ErrBookmarkLimit
	}

	tagCreated, err := addTags(ctx, tx, userID, int(id), NormaliseTags(b.Tags))
	if err != nil {
		return 0, err
	}
	if err = m.checkTagLimit(ctx, tx, userID, tagCreated); err != nil {
		return 0, err
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return int(id), nil
}

// checkTagLimit returns ErrTagLimit if this save created a tag and the user is
// now over the limit. Saves that create no tag always pass, so someone already
// over a lowered limit can still edit their bookmarks.
func (m *BookmarkModel) checkTagLimit(ctx context.Context, tx *sql.Tx, userID int, created bool) error {
	if !created {
		return nil
	}
	over, err := overLimit(ctx, tx, m.MaxTags, `SELECT count(*) FROM tags WHERE user_id = ?`, userID)
	if err != nil {
		return err
	}
	if over {
		return ErrTagLimit
	}
	return nil
}

// addTags links a bookmark to the named tags, creating any the user doesn't
// have yet. Links that already exist are left alone. Returns whether any tag
// was created.
func addTags(ctx context.Context, tx *sql.Tx, userID, bookmarkID int, names []string) (bool, error) {
	created := false
	for _, name := range names {
		result, err := tx.ExecContext(ctx,
			`INSERT INTO tags (user_id, name) VALUES (?, ?) ON CONFLICT (user_id, name) DO NOTHING`,
			userID, name)
		if err != nil {
			return false, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return false, err
		}
		created = created || n > 0
		_, err = tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO bookmark_tags (bookmark_id, tag_id)
			 SELECT ?, id FROM tags WHERE user_id = ? AND name = ?`,
			bookmarkID, userID, name)
		if err != nil {
			return false, err
		}
	}
	return created, nil
}

// Update saves changes to one of the user's bookmarks, in one transaction.
// The ID comes from b; the saved date never changes. The favicon is cleared if
// the normalised URL changes, because it belonged to the old site. Returns
// ErrNoRecord if the user has no such bookmark, and ErrDuplicateURL if the new
// URL is a Duplicate of another of their bookmarks, or ErrTagLimit.
func (m *BookmarkModel) Update(userID int, b *Bookmark) error {
	normalisedURL, err := NormaliseURL(b.URL)
	if err != nil {
		return err
	}
	ctx := context.Background()
	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op after Commit

	stmt := `UPDATE bookmarks
	         SET url = ?, normalised_url = ?, title = ?, description = ?, notes = ?,
	             favourite = ?, read_later = ?,
	             favicon = CASE WHEN normalised_url = ? THEN favicon ELSE '' END
	         WHERE user_id = ? AND id = ?`
	result, err := tx.ExecContext(ctx, stmt, b.URL, normalisedURL, b.Title, b.Description, b.Notes,
		b.Favourite, b.ReadLater, normalisedURL, userID, b.ID)
	if isUniqueViolation(err, "bookmarks.user_id, bookmarks.normalised_url") {
		return ErrDuplicateURL
	}
	if err = checkAffected(result, err); err != nil {
		return err
	}

	// Add the new links before removing old ones, and keep links that stay, so
	// that a tag used only here keeps its ID.
	names := NormaliseTags(b.Tags)
	created, err := addTags(ctx, tx, userID, b.ID, names)
	if err != nil {
		return err
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(names)), ",")
	args := []any{b.ID, userID}
	for _, name := range names {
		args = append(args, name)
	}
	stmt = `DELETE FROM bookmark_tags
	        WHERE bookmark_id = ?
	          AND tag_id NOT IN (SELECT id FROM tags WHERE user_id = ? AND name IN (` + placeholders + `))`
	if _, err = tx.ExecContext(ctx, stmt, args...); err != nil {
		return err
	}

	// Checked after removing old links, so swapping a tag at the limit works.
	if err = m.checkTagLimit(ctx, tx, userID, created); err != nil {
		return err
	}
	return tx.Commit()
}

// Columns selected for a Bookmark, with the tags joined into one
// comma-separated string (tag names never contain commas).
const bookmarkColumns = `b.id, b.user_id, b.url, b.title, b.description, b.notes, b.favicon,
	b.favourite, b.read_later, b.created_at,
	(SELECT group_concat(t.name, ',' ORDER BY t.name)
	   FROM bookmark_tags bt JOIN tags t ON t.id = bt.tag_id
	  WHERE bt.bookmark_id = b.id)`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanBookmark(row rowScanner) (*Bookmark, error) {
	b := &Bookmark{}
	var tags sql.NullString
	err := row.Scan(&b.ID, &b.UserID, &b.URL, &b.Title, &b.Description, &b.Notes, &b.Favicon,
		&b.Favourite, &b.ReadLater, &b.Created, &tags)
	if err != nil {
		return nil, err
	}
	if tags.Valid && tags.String != "" {
		b.Tags = strings.Split(tags.String, ",")
	}
	return b, nil
}

// Get returns one of the user's bookmarks.
func (m *BookmarkModel) Get(userID, id int) (*Bookmark, error) {
	stmt := `SELECT ` + bookmarkColumns + ` FROM bookmarks b WHERE b.user_id = ? AND b.id = ?`
	b, err := scanBookmark(m.DB.QueryRow(stmt, userID, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}
	return b, nil
}

// Latest returns the user's most recently saved bookmarks, newest first.
func (m *BookmarkModel) Latest(userID, limit int) ([]*Bookmark, error) {
	stmt := `SELECT ` + bookmarkColumns + ` FROM bookmarks b
	         WHERE b.user_id = ? ORDER BY b.created_at DESC, b.id DESC LIMIT ?`
	rows, err := m.DB.Query(stmt, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bookmarks := []*Bookmark{}
	for rows.Next() {
		b, err := scanBookmark(rows)
		if err != nil {
			return nil, err
		}
		bookmarks = append(bookmarks, b)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return bookmarks, nil
}

// ToggleFavourite flips the favourite star on one of the user's bookmarks.
func (m *BookmarkModel) ToggleFavourite(userID, id int) error {
	result, err := m.DB.Exec(
		`UPDATE bookmarks SET favourite = NOT favourite WHERE user_id = ? AND id = ?`, userID, id)
	return checkAffected(result, err)
}

// Delete removes one of the user's bookmarks (its tag links cascade).
func (m *BookmarkModel) Delete(userID, id int) error {
	result, err := m.DB.Exec(`DELETE FROM bookmarks WHERE user_id = ? AND id = ?`, userID, id)
	return checkAffected(result, err)
}

// checkAffected turns "no rows changed" into ErrNoRecord.
func checkAffected(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNoRecord
	}
	return nil
}
