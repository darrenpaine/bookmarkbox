package models

import (
	"database/sql"
	"errors"
	"slices"
	"testing"

	"github.com/darrenpaine/bookmarkbox/internal/assert"
)

func TestNormaliseTags(t *testing.T) {
	got := NormaliseTags([]string{" Go ", "web", "", "go", "Databases"})
	want := []string{"databases", "go", "web"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q; want %q", got, want)
	}
}

func TestBookmarkModelGet(t *testing.T) {
	db := newTestDB(t)
	m := BookmarkModel{DB: db}

	t.Run("Owner", func(t *testing.T) {
		b, err := m.Get(1, 1)
		assert.NilError(t, err)
		assert.Equal(t, b.Title, "Effective Go")
		assert.Equal(t, len(b.Tags), 1)
		assert.Equal(t, b.Tags[0], "go")
		assert.Equal(t, b.Created.Year(), 2026)
	})

	t.Run("Other user", func(t *testing.T) {
		_, err := m.Get(2, 1)
		assert.Equal(t, errors.Is(err, ErrNoRecord), true)
	})

	t.Run("Missing", func(t *testing.T) {
		_, err := m.Get(1, 99)
		assert.Equal(t, errors.Is(err, ErrNoRecord), true)
	})
}

func TestBookmarkModelInsert(t *testing.T) {
	db := newTestDB(t)
	m := BookmarkModel{DB: db}

	id, err := m.Insert(1, &Bookmark{
		URL:       "https://pkg.go.dev/database/sql",
		Title:     "database/sql",
		Favourite: true,
		Tags:      []string{"Go", "databases", "go"},
	})
	assert.NilError(t, err)

	b, err := m.Get(1, id)
	assert.NilError(t, err)
	assert.Equal(t, b.Favourite, true)
	if !slices.Equal(b.Tags, []string{"databases", "go"}) {
		t.Errorf("tags: got %q", b.Tags)
	}

	// "go" already existed for Alice, so only "databases" is new.
	var tagCount int
	assert.NilError(t, db.QueryRow("SELECT count(*) FROM tags WHERE user_id = 1").Scan(&tagCount))
	assert.Equal(t, tagCount, 2)

	t.Run("Duplicate URL", func(t *testing.T) {
		_, err := m.Insert(1, &Bookmark{URL: "https://go.dev/doc/effective_go", Title: "Again"})
		assert.Equal(t, errors.Is(err, ErrDuplicateURL), true)
	})

	t.Run("Duplicate after normalising", func(t *testing.T) {
		_, err := m.Insert(1, &Bookmark{URL: "https://GO.dev/doc/effective_go/?utm_source=x", Title: "Again"})
		assert.Equal(t, errors.Is(err, ErrDuplicateURL), true)
	})

	t.Run("Original URL kept for display", func(t *testing.T) {
		id, err := m.Insert(1, &Bookmark{URL: "https://Example.com/?utm_source=x", Title: "Example"})
		assert.NilError(t, err)
		b, err := m.Get(1, id)
		assert.NilError(t, err)
		assert.Equal(t, b.URL, "https://Example.com/?utm_source=x")
		assert.NilError(t, m.Delete(1, id))
	})

	t.Run("Same URL, other user", func(t *testing.T) {
		_, err := m.Insert(2, &Bookmark{URL: "https://go.dev/doc/effective_go", Title: "Bob's copy"})
		assert.NilError(t, err)
	})

	t.Run("Latest is newest first", func(t *testing.T) {
		list, err := m.Latest(1, 10)
		assert.NilError(t, err)
		assert.Equal(t, len(list), 2)
		assert.Equal(t, list[0].ID, id)
	})
}

func TestBookmarkModelToggleAndDelete(t *testing.T) {
	db := newTestDB(t)
	m := BookmarkModel{DB: db}

	assert.NilError(t, m.ToggleFavourite(1, 1))
	b, _ := m.Get(1, 1)
	assert.Equal(t, b.Favourite, true)

	// Another user can't change or delete it.
	assert.Equal(t, errors.Is(m.ToggleFavourite(2, 1), ErrNoRecord), true)
	assert.Equal(t, errors.Is(m.Delete(2, 1), ErrNoRecord), true)

	assert.NilError(t, m.Delete(1, 1))
	var links int
	assert.NilError(t, db.QueryRow("SELECT count(*) FROM bookmark_tags").Scan(&links))
	assert.Equal(t, links, 0)
}

func TestBookmarkModelUpdate(t *testing.T) {
	db := newTestDB(t)
	m := BookmarkModel{DB: db}

	other, err := m.Insert(1, &Bookmark{URL: "https://sqlite.org", Title: "SQLite", Tags: []string{"go", "databases"}})
	assert.NilError(t, err)
	before, err := m.Get(1, 1)
	assert.NilError(t, err)
	_, err = db.Exec("UPDATE bookmarks SET favicon = 'icon.png' WHERE id = 1")
	assert.NilError(t, err)
	var goTagID int
	assert.NilError(t, db.QueryRow("SELECT id FROM tags WHERE name = 'go'").Scan(&goTagID))

	t.Run("Changes fields and tags", func(t *testing.T) {
		err := m.Update(1, &Bookmark{
			ID: 1, URL: "https://go.dev/doc/effective_go/", Title: "New title", Description: "New description",
			Notes: "Notes", Favourite: true, ReadLater: true, Tags: []string{"GO", "style"},
		})
		assert.NilError(t, err)
		b, err := m.Get(1, 1)
		assert.NilError(t, err)
		assert.Equal(t, b.Title, "New title")
		assert.Equal(t, b.Description, "New description")
		assert.Equal(t, b.Notes, "Notes")
		assert.Equal(t, b.Favourite, true)
		assert.Equal(t, b.ReadLater, true)
		assert.Equal(t, b.Created.Equal(before.Created), true)
		// The same site after normalising, so the favicon stays.
		assert.Equal(t, b.Favicon, "icon.png")
		if !slices.Equal(b.Tags, []string{"go", "style"}) {
			t.Errorf("tags: got %q", b.Tags)
		}
	})

	t.Run("Kept tag keeps its ID", func(t *testing.T) {
		var id int
		assert.NilError(t, db.QueryRow("SELECT id FROM tags WHERE name = 'go'").Scan(&id))
		assert.Equal(t, id, goTagID)
	})

	t.Run("New URL clears favicon", func(t *testing.T) {
		assert.NilError(t, m.Update(1, &Bookmark{ID: 1, URL: "https://go.dev/blog", Title: "Blog"}))
		b, err := m.Get(1, 1)
		assert.NilError(t, err)
		assert.Equal(t, b.Favicon, "")
		assert.Equal(t, len(b.Tags), 0)
	})

	t.Run("Tag only used here is removed", func(t *testing.T) {
		if got := tagNames(t, db, 1); !slices.Equal(got, []string{"databases", "go"}) {
			t.Errorf("tags: got %q", got)
		}
	})

	t.Run("Saving without changes", func(t *testing.T) {
		assert.NilError(t, m.Update(1, &Bookmark{ID: 1, URL: "https://go.dev/blog", Title: "Blog"}))
	})

	t.Run("Duplicate URL changes nothing", func(t *testing.T) {
		err := m.Update(1, &Bookmark{ID: 1, URL: "https://SQLite.org/", Title: "Changed", Tags: []string{"new"}})
		assert.Equal(t, errors.Is(err, ErrDuplicateURL), true)
		b, err := m.Get(1, 1)
		assert.NilError(t, err)
		assert.Equal(t, b.Title, "Blog")
		if got := tagNames(t, db, 1); slices.Contains(got, "new") {
			t.Errorf("tag was saved: %q", got)
		}
	})

	t.Run("Other user", func(t *testing.T) {
		err := m.Update(2, &Bookmark{ID: other, URL: "https://hacked.example.com", Title: "Hacked"})
		assert.Equal(t, errors.Is(err, ErrNoRecord), true)
		b, err := m.Get(1, other)
		assert.NilError(t, err)
		assert.Equal(t, b.Title, "SQLite")
	})
}

func tagNames(t *testing.T, db *sql.DB, userID int) []string {
	rows, err := db.Query("SELECT name FROM tags WHERE user_id = ? ORDER BY name", userID)
	assert.NilError(t, err)
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var name string
		assert.NilError(t, rows.Scan(&name))
		names = append(names, name)
	}
	return names
}

func TestUnusedTagsAreRemoved(t *testing.T) {
	db := newTestDB(t)
	m := BookmarkModel{DB: db}

	id, err := m.Insert(1, &Bookmark{URL: "https://sqlite.org", Title: "SQLite", Tags: []string{"go", "databases"}})
	assert.NilError(t, err)

	// "go" is still used by the seeded bookmark. Deleting a bookmark reaches
	// bookmark_tags through a foreign key cascade, so this proves the trigger
	// fires for cascades.
	assert.NilError(t, m.Delete(1, id))
	if got := tagNames(t, db, 1); !slices.Equal(got, []string{"go"}) {
		t.Errorf("tags after delete: got %q", got)
	}

	assert.NilError(t, m.Delete(1, 1))
	if got := tagNames(t, db, 1); len(got) != 0 {
		t.Errorf("tags after last delete: got %q", got)
	}
}
