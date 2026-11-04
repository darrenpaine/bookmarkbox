package models

import (
	"errors"
	"slices"
	"testing"

	"github.com/darrenpaine/bookmarkbox/internal/assert"
)

// Adds bookmarks so that Alice has go (2), golang (2) and sql (1). The seeded
// bookmark has go, the second has go and golang, and the third has golang.
func seedTags(t *testing.T, m BookmarkModel) (second, third int) {
	second, err := m.Insert(1, &Bookmark{URL: "https://a.example.com", Title: "A", Tags: []string{"go", "golang"}})
	assert.NilError(t, err)
	third, err = m.Insert(1, &Bookmark{URL: "https://b.example.com", Title: "B", Tags: []string{"golang", "sql"}})
	assert.NilError(t, err)
	return second, third
}

func tagID(t *testing.T, m TagModel, userID int, name string) int {
	tags, err := m.List(userID)
	assert.NilError(t, err)
	for _, tag := range tags {
		if tag.Name == name {
			return tag.ID
		}
	}
	t.Fatalf("no tag %q", name)
	return 0
}

func TestTagModelList(t *testing.T) {
	db := newTestDB(t)
	seedTags(t, BookmarkModel{DB: db})
	m := TagModel{db}

	tags, err := m.List(1)
	assert.NilError(t, err)
	assert.Equal(t, len(tags), 3)
	assert.Equal(t, tags[0].Name, "go")
	assert.Equal(t, tags[0].BookmarkCount, 2)
	assert.Equal(t, tags[1].Name, "golang")
	assert.Equal(t, tags[1].BookmarkCount, 2)

	other, err := m.List(2)
	assert.NilError(t, err)
	assert.Equal(t, len(other), 0)
}

func TestTagModelRename(t *testing.T) {
	db := newTestDB(t)
	bookmarks := BookmarkModel{DB: db}
	second, _ := seedTags(t, bookmarks)
	m := TagModel{db}
	id := tagID(t, m, 1, "sql")

	assert.NilError(t, m.Rename(1, id, "databases"))
	tag, err := m.Get(1, id)
	assert.NilError(t, err)
	assert.Equal(t, tag.Name, "databases")

	t.Run("Existing name", func(t *testing.T) {
		err := m.Rename(1, id, "go")
		assert.Equal(t, errors.Is(err, ErrDuplicateTag), true)
	})

	t.Run("Other user", func(t *testing.T) {
		err := m.Rename(2, id, "hacked")
		assert.Equal(t, errors.Is(err, ErrNoRecord), true)
		_, err = m.Get(2, id)
		assert.Equal(t, errors.Is(err, ErrNoRecord), true)
	})

	t.Run("Same name as another user's tag", func(t *testing.T) {
		_, err := bookmarks.Insert(2, &Bookmark{URL: "https://c.example.com", Title: "C", Tags: []string{"bob"}})
		assert.NilError(t, err)
		assert.NilError(t, m.Rename(2, tagID(t, m, 2, "bob"), "go"))
	})

	b, err := bookmarks.Get(1, second)
	assert.NilError(t, err)
	if !slices.Equal(b.Tags, []string{"go", "golang"}) {
		t.Errorf("tags: got %q", b.Tags)
	}
}

func TestTagModelMerge(t *testing.T) {
	db := newTestDB(t)
	bookmarks := BookmarkModel{DB: db}
	second, third := seedTags(t, bookmarks)
	m := TagModel{db}
	golang := tagID(t, m, 1, "golang")

	t.Run("Target missing changes nothing", func(t *testing.T) {
		err := m.Merge(1, golang, "missing")
		assert.Equal(t, errors.Is(err, ErrNoRecord), true)
	})

	t.Run("Other user changes nothing", func(t *testing.T) {
		err := m.Merge(2, golang, "go")
		assert.Equal(t, errors.Is(err, ErrNoRecord), true)
	})

	if got := tagNames(t, db, 1); !slices.Equal(got, []string{"go", "golang", "sql"}) {
		t.Fatalf("tags before merge: got %q", got)
	}

	assert.NilError(t, m.Merge(1, golang, "go"))

	if got := tagNames(t, db, 1); !slices.Equal(got, []string{"go", "sql"}) {
		t.Errorf("tags after merge: got %q", got)
	}
	// The second bookmark had both tags, so it keeps one.
	b, err := bookmarks.Get(1, second)
	assert.NilError(t, err)
	if !slices.Equal(b.Tags, []string{"go"}) {
		t.Errorf("second bookmark: got %q", b.Tags)
	}
	b, err = bookmarks.Get(1, third)
	assert.NilError(t, err)
	if !slices.Equal(b.Tags, []string{"go", "sql"}) {
		t.Errorf("third bookmark: got %q", b.Tags)
	}
	tag, err := m.Get(1, tagID(t, m, 1, "go"))
	assert.NilError(t, err)
	assert.Equal(t, tag.BookmarkCount, 3)
}
