package models

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/darrenpaine/bookmarkbox/internal/bookmarkfile"
	"github.com/darrenpaine/bookmarkbox/internal/urls"
	"github.com/darrenpaine/bookmarkbox/internal/validator"
)

// UnfiledTag is the Tag given to a new Bookmark that sat directly in a Root
// Folder, with no Folder of its own to become a Tag.
const UnfiledTag = "unfiled"

// Import limits: the size of an uploaded file, and the links in it.
const (
	MaxImportBytes = 20 << 20 // 20 MB
	MaxImportLinks = 20000
)

// ImportExpiry is how long an uploaded file waits for its import to be confirmed.
const ImportExpiry = time.Hour

// ImportItem is one Bookmark an import would save. Unfiled items get the
// Unfiled Tag, but only if they are new: a Duplicate keeps its own Tags.
type ImportItem struct {
	Bookmark
	Unfiled bool
}

// ImportFile is a bookmark file made ready to import.
type ImportFile struct {
	Items   []ImportItem
	Skipped int // links that aren't web addresses, or are too long
	Merged  int // links repeated in the file, merged into one Bookmark
}

// PrepareImport turns the links read from a file into the Bookmarks they
// become: Folders become Tags, a link with neither Folders nor a TAGS
// attribute is unfiled, and over-long text is cut. Links repeated in the file
// are merged into one, with every Tag and the earliest date. A link with no
// date is given now.
func PrepareImport(links []bookmarkfile.Link, now time.Time) *ImportFile {
	f := &ImportFile{}
	seen := map[string]int{} // normalised URL -> index in f.Items
	for _, l := range links {
		if !validator.WebURL(l.URL) || !validator.MaxChars(l.URL, MaxURLChars) {
			f.Skipped++
			continue
		}
		normalised, err := NormaliseURL(l.URL)
		if err != nil {
			f.Skipped++
			continue
		}

		var tags []string
		for _, name := range l.Folders {
			tags = append(tags, importTagName(name))
		}
		for _, name := range l.Tags {
			tags = append(tags, importTagName(name))
		}
		tags = NormaliseTags(tags)
		added := l.Added
		if added.IsZero() {
			added = now
		}
		unfiled := len(l.Folders) == 0 && !l.HasTags

		if i, ok := seen[normalised]; ok {
			item := &f.Items[i]
			item.Tags = NormaliseTags(append(item.Tags, tags...))
			if added.Before(item.Created) {
				item.Created = added
			}
			item.Unfiled = item.Unfiled && unfiled
			item.Favourite = item.Favourite || l.Favourite
			item.ReadLater = item.ReadLater || l.ReadLater
			if item.Description == "" {
				item.Description = cut(l.Description, MaxDescriptionChars)
			}
			if item.Notes == "" {
				item.Notes = cut(l.Notes, MaxNotesChars)
			}
			f.Merged++
			continue
		}

		// Trimmed after cutting too, so a cut title never ends in a space
		title := strings.TrimSpace(cut(strings.TrimSpace(l.Title), MaxTitleChars))
		if title == "" {
			title = cut(l.URL, MaxTitleChars)
		}
		seen[normalised] = len(f.Items)
		f.Items = append(f.Items, ImportItem{
			Bookmark: Bookmark{
				URL:         l.URL,
				Title:       title,
				Description: cut(l.Description, MaxDescriptionChars),
				Notes:       cut(l.Notes, MaxNotesChars),
				Favourite:   l.Favourite,
				ReadLater:   l.ReadLater,
				Created:     added,
				Tags:        tags,
			},
			Unfiled: unfiled,
		})
	}
	return f
}

// importTagName turns a Folder name, or a name from a TAGS attribute, into a
// Tag name: commas become spaces, runs of spaces become one, and the name is
// cut to the longest a Tag can have. It may still need normalising.
func importTagName(name string) string {
	name = strings.Join(strings.Fields(strings.ReplaceAll(name, ",", " ")), " ")
	return strings.TrimSpace(cut(strings.ToLower(name), MaxTagNameChars))
}

// cut shortens s to at most n characters.
func cut(s string, n int) string {
	i := 0
	for pos := range s {
		if i == n {
			return s[:pos]
		}
		i++
	}
	return s
}

// ExportLinks turns Bookmarks into links to write to a bookmark file.
func ExportLinks(bookmarks []*Bookmark) []bookmarkfile.Link {
	links := make([]bookmarkfile.Link, len(bookmarks))
	for i, b := range bookmarks {
		links[i] = bookmarkfile.Link{
			URL:         b.URL,
			Title:       b.Title,
			Description: b.Description,
			Notes:       b.Notes,
			Added:       b.Created,
			Tags:        b.Tags,
			HasTags:     true,
			Favourite:   b.Favourite,
			ReadLater:   b.ReadLater,
		}
	}
	return links
}

// ImportSummary describes what an import does, or would do.
type ImportSummary struct {
	New        int // Bookmarks added
	Duplicates int // already saved; they only gain any new Tags
	Updated    int // Duplicates that gain a Tag
	Unfiled    int // new Bookmarks given the Unfiled Tag
	Tags       int // different Tags the import uses, from Folders and TAGS attributes
	NewTags    int // Tags the User doesn't have yet, including Unfiled

	BookmarksAfter, MaxBookmarks int // the User's Bookmarks afterwards, and their limit (0: none)
	TagsAfter, MaxTags           int
}

// OverBookmarkLimit reports whether the import would pass the Bookmark limit.
func (s *ImportSummary) OverBookmarkLimit() bool {
	return s.MaxBookmarks > 0 && s.New > 0 && s.BookmarksAfter > s.MaxBookmarks
}

// OverTagLimit reports whether the import would create a Tag and pass the Tag
// limit. As with a single save, an import creating no Tag always passes.
func (s *ImportSummary) OverTagLimit() bool {
	return s.MaxTags > 0 && s.NewTags > 0 && s.TagsAfter > s.MaxTags
}

// ChangesNothing reports whether every Bookmark is already saved, with its Tags.
func (s *ImportSummary) ChangesNothing() bool {
	return s.New == 0 && s.Updated == 0
}

// importPlan is what an import will do with each item.
type importPlan struct {
	summary  ImportSummary
	existing map[string]int // normalised URL -> the User's Bookmark ID
}

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// planImport works out what importing the items would do, from the User's
// Bookmarks and Tags as they are now.
func (m *BookmarkModel) planImport(ctx context.Context, q querier, userID int, items []ImportItem) (*importPlan, error) {
	p := &importPlan{existing: map[string]int{}}
	idTags := map[int]map[string]bool{} // a Bookmark ID -> its Tags
	userTags := map[string]bool{}

	rows, err := q.QueryContext(ctx, `SELECT id, normalised_url FROM bookmarks WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int
		var normalised string
		if err = rows.Scan(&id, &normalised); err != nil {
			rows.Close()
			return nil, err
		}
		p.existing[normalised] = id
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}

	rows, err = q.QueryContext(ctx,
		`SELECT t.name, bt.bookmark_id FROM tags t LEFT JOIN bookmark_tags bt ON bt.tag_id = t.id WHERE t.user_id = ?`,
		userID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var name string
		var id sql.NullInt64
		if err = rows.Scan(&name, &id); err != nil {
			rows.Close()
			return nil, err
		}
		userTags[name] = true
		if id.Valid {
			if idTags[int(id.Int64)] == nil {
				idTags[int(id.Int64)] = map[string]bool{}
			}
			idTags[int(id.Int64)][name] = true
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}

	s := &p.summary
	used := map[string]bool{} // Tags the import gives a Bookmark
	importTags := map[string]bool{}
	for _, item := range items {
		normalised, err := NormaliseURL(item.URL)
		if err != nil {
			return nil, err
		}
		for _, name := range item.Tags {
			importTags[name] = true
		}
		if id, ok := p.existing[normalised]; ok {
			s.Duplicates++
			gains := false
			for _, name := range item.Tags {
				used[name] = true
				gains = gains || !idTags[id][name]
			}
			if gains {
				s.Updated++
			}
			continue
		}
		s.New++
		for _, name := range item.Tags {
			used[name] = true
		}
		if item.Unfiled {
			s.Unfiled++
			used[UnfiledTag] = true
		}
	}
	for name := range used {
		if !userTags[name] {
			s.NewTags++
		}
	}
	s.Tags = len(importTags)
	s.BookmarksAfter = len(p.existing) + s.New
	s.TagsAfter = len(userTags) + s.NewTags
	s.MaxBookmarks, s.MaxTags = m.MaxBookmarks, m.MaxTags
	return p, nil
}

// PreviewImport says what importing the items would do, without saving anything.
func (m *BookmarkModel) PreviewImport(userID int, items []ImportItem) (*ImportSummary, error) {
	p, err := m.planImport(context.Background(), m.DB, userID, items)
	if err != nil {
		return nil, err
	}
	return &p.summary, nil
}

// Import saves the items, all or nothing. New Bookmarks are added with their
// dates and Tags; Duplicates only gain any new Tags. Returns ErrBookmarkLimit
// or ErrTagLimit, saving nothing, if the import would pass a limit.
func (m *BookmarkModel) Import(userID int, items []ImportItem) (*ImportSummary, error) {
	ctx := context.Background()
	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() // no-op after Commit

	// Planned inside the transaction, which holds the write lock, so the
	// counts can't change before the import is saved.
	p, err := m.planImport(ctx, tx, userID, items)
	if err != nil {
		return nil, err
	}
	switch {
	case p.summary.OverBookmarkLimit():
		return nil, ErrBookmarkLimit
	case p.summary.OverTagLimit():
		return nil, ErrTagLimit
	}

	insert, err := tx.PrepareContext(ctx, `INSERT INTO bookmarks
	         (user_id, url, normalised_url, domain, title, description, notes, favourite, read_later, created_at)
	         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return nil, err
	}
	defer insert.Close()
	tagger, err := newTagger(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	defer tagger.close()

	for _, item := range items {
		normalised, err := NormaliseURL(item.URL)
		if err != nil {
			return nil, err
		}
		tags := item.Tags
		id, ok := p.existing[normalised]
		if !ok {
			result, err := insert.ExecContext(ctx, userID, item.URL, normalised, urls.Domain(item.URL),
				item.Title, item.Description, item.Notes, item.Favourite, item.ReadLater, item.Created.UTC())
			if err != nil {
				return nil, err
			}
			id64, err := result.LastInsertId()
			if err != nil {
				return nil, err
			}
			id = int(id64)
			p.existing[normalised] = id
			if item.Unfiled {
				tags = append(slices.Clip(tags), UnfiledTag)
			}
		}
		for _, name := range tags {
			if err = tagger.tag(ctx, id, name); err != nil {
				return nil, err
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &p.summary, nil
}

// tagger links Bookmarks to Tags during an import, creating Tags as needed
// and remembering their IDs, with its statements prepared once.
type tagger struct {
	userID     int
	ids        map[string]int
	selectTag  *sql.Stmt
	insertTag  *sql.Stmt
	insertLink *sql.Stmt
}

func newTagger(ctx context.Context, tx *sql.Tx, userID int) (*tagger, error) {
	t := &tagger{userID: userID, ids: map[string]int{}}
	statements := []struct {
		stmt  **sql.Stmt
		query string
	}{
		{&t.selectTag, `SELECT id FROM tags WHERE user_id = ? AND name = ?`},
		{&t.insertTag, `INSERT INTO tags (user_id, name) VALUES (?, ?) RETURNING id`},
		{&t.insertLink, `INSERT OR IGNORE INTO bookmark_tags (bookmark_id, tag_id) VALUES (?, ?)`},
	}
	for _, s := range statements {
		stmt, err := tx.PrepareContext(ctx, s.query)
		if err != nil {
			t.close()
			return nil, err
		}
		*s.stmt = stmt
	}
	return t, nil
}

// tag links a Bookmark to the named Tag, creating the Tag if the User doesn't
// have it yet.
func (t *tagger) tag(ctx context.Context, bookmarkID int, name string) error {
	id, ok := t.ids[name]
	if !ok {
		err := t.selectTag.QueryRowContext(ctx, t.userID, name).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			err = t.insertTag.QueryRowContext(ctx, t.userID, name).Scan(&id)
		}
		if err != nil {
			return err
		}
		t.ids[name] = id
	}
	_, err := t.insertLink.ExecContext(ctx, bookmarkID, id)
	return err
}

func (t *tagger) close() {
	for _, stmt := range []*sql.Stmt{t.selectTag, t.insertTag, t.insertLink} {
		if stmt != nil {
			stmt.Close()
		}
	}
}

// PendingImport is a file a User has uploaded, waiting for them to confirm
// the import.
type PendingImport struct {
	Filename string
	Data     []byte
	Created  time.Time
}

// ImportModelInterface - the pending import methods used by the handlers.
type ImportModelInterface interface {
	Save(userID int, filename string, data []byte) error
	Get(userID int) (*PendingImport, error)
	Delete(userID int) error
}

// ImportModel keeps each User's pending import.
type ImportModel struct {
	DB *sql.DB
}

// Save keeps a User's uploaded file, replacing any earlier one. It also
// clears every pending import that has expired.
func (m *ImportModel) Save(userID int, filename string, data []byte) error {
	now := time.Now()
	if _, err := m.DB.Exec(`DELETE FROM imports WHERE created_at <= ?`, now.Add(-ImportExpiry).Unix()); err != nil {
		return err
	}
	_, err := m.DB.Exec(`INSERT INTO imports (user_id, filename, data, created_at) VALUES (?, ?, ?, ?)
	                     ON CONFLICT (user_id) DO UPDATE
	                     SET filename = excluded.filename, data = excluded.data, created_at = excluded.created_at`,
		userID, filename, data, now.Unix())
	return err
}

// Get returns the User's pending import, or ErrNoRecord if they have none or
// it has expired.
func (m *ImportModel) Get(userID int) (*PendingImport, error) {
	p := &PendingImport{}
	var created int64
	err := m.DB.QueryRow(`SELECT filename, data, created_at FROM imports WHERE user_id = ? AND created_at > ?`,
		userID, time.Now().Add(-ImportExpiry).Unix()).Scan(&p.Filename, &p.Data, &created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}
	p.Created = time.Unix(created, 0)
	return p, nil
}

// Delete removes the User's pending import, if they have one.
func (m *ImportModel) Delete(userID int) error {
	_, err := m.DB.Exec(`DELETE FROM imports WHERE user_id = ?`, userID)
	return err
}
