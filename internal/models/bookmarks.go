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

// BookmarkModel wraps a database connection pool.
type BookmarkModel struct {
	DB *sql.DB
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
// Returns ErrDuplicateURL if it would be a Duplicate of one of the user's bookmarks.
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

	for _, name := range NormaliseTags(b.Tags) {
		// Create the tag if this user doesn't have it yet, then link it.
		_, err = tx.ExecContext(ctx,
			`INSERT INTO tags (user_id, name) VALUES (?, ?) ON CONFLICT (user_id, name) DO NOTHING`,
			userID, name)
		if err != nil {
			return 0, err
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO bookmark_tags (bookmark_id, tag_id)
			 SELECT ?, id FROM tags WHERE user_id = ? AND name = ?`,
			id, userID, name)
		if err != nil {
			return 0, err
		}
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return int(id), nil
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
