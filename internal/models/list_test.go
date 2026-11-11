package models

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/darrenpaine/bookmarkbox/internal/assert"
)

// listTitles returns the titles in a page, in order.
func listTitles(page *ListPage) []string {
	titles := []string{}
	for _, b := range page.Bookmarks {
		titles = append(titles, b.Title)
	}
	return titles
}

// newListModel gives Bob (user 2) four Bookmarks, so Alice's seeded one stays
// out of the way, and returns their IDs.
func newListModel(t *testing.T) (*BookmarkModel, map[string]int) {
	m := &BookmarkModel{DB: newTestDB(t)}
	day := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	ids := map[string]int{}
	for i, b := range []*Bookmark{
		{URL: "https://www.zeta.example/a", Title: "beta", Tags: []string{"go", "web"}, Favourite: true},
		{URL: "https://alpha.example/a", Title: "Delta", Tags: []string{"go"}, ReadLater: true},
		{URL: "https://alpha.example/b", Title: "alpha", Tags: []string{"web"}, Favourite: true, ReadLater: true},
		{URL: "https://mid.example/", Title: "Charlie"},
	} {
		b.Created = day.AddDate(0, 0, i)
		id, err := m.Insert(2, b)
		assert.NilError(t, err)
		ids[b.Title] = id
	}
	return m, ids
}

func TestBookmarkModelListSorts(t *testing.T) {
	m, _ := newListModel(t)
	tests := []struct {
		sort string
		want []string
	}{
		{"", []string{"Charlie", "alpha", "Delta", "beta"}},
		{SortNewest, []string{"Charlie", "alpha", "Delta", "beta"}},
		{SortOldest, []string{"beta", "Delta", "alpha", "Charlie"}},
		{SortTitle, []string{"alpha", "beta", "Charlie", "Delta"}},
		{SortDomain, []string{"alpha", "Delta", "Charlie", "beta"}}, // alpha.example, mid.example, zeta.example
		{"nonsense", []string{"Charlie", "alpha", "Delta", "beta"}},
	}
	for _, tt := range tests {
		t.Run(tt.sort, func(t *testing.T) {
			page, err := m.List(2, ListQuery{Sort: tt.sort})
			assert.NilError(t, err)
			if got := listTitles(page); !slices.Equal(got, tt.want) {
				t.Errorf("got %q; want %q", got, tt.want)
			}
			assert.Equal(t, page.Total, 4)
			assert.Equal(t, page.More, false)
		})
	}
}

func TestBookmarkModelListFilters(t *testing.T) {
	m, _ := newListModel(t)
	tests := []struct {
		name   string
		filter Filter
		want   []string
	}{
		{"Favourite", Filter{Favourite: true}, []string{"alpha", "beta"}},
		{"Read later", Filter{ReadLater: true}, []string{"alpha", "Delta"}},
		{"Both", Filter{Favourite: true, ReadLater: true}, []string{"alpha"}},
		{"Tag", Filter{Tags: []string{"go"}}, []string{"beta", "Delta"}},
		{"Every Tag must match", Filter{Tags: []string{"go", "web"}}, []string{"beta"}},
		{"Domain", Filter{Domain: "alpha.example"}, []string{"alpha", "Delta"}},
		{"Domain without www.", Filter{Domain: "zeta.example"}, []string{"beta"}},
		{"Domain and Tag", Filter{Domain: "alpha.example", Tags: []string{"web"}}, []string{"alpha"}},
		{"Unknown Tag", Filter{Tags: []string{"nope"}}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := m.List(2, ListQuery{Filter: tt.filter, Sort: SortTitle})
			assert.NilError(t, err)
			if got := listTitles(page); !slices.Equal(got, tt.want) {
				t.Errorf("got %q; want %q", got, tt.want)
			}
			assert.Equal(t, page.Total, len(tt.want))

			n, err := m.Count(2, tt.filter)
			assert.NilError(t, err)
			assert.Equal(t, n, len(tt.want))
		})
	}

	t.Run("Other users' Bookmarks are never listed", func(t *testing.T) {
		page, err := m.List(1, ListQuery{Filter: Filter{Tags: []string{"web"}}})
		assert.NilError(t, err)
		assert.Equal(t, page.Total, 0)
	})
}

func TestBookmarkModelListDomainFollowsURL(t *testing.T) {
	m, ids := newListModel(t)
	assert.NilError(t, m.Update(2, &Bookmark{ID: ids["Charlie"], URL: "https://www.alpha.example/c", Title: "Charlie"}))
	page, err := m.List(2, ListQuery{Filter: Filter{Domain: "alpha.example"}})
	assert.NilError(t, err)
	assert.Equal(t, page.Total, 3)
}

// insertMany gives Bob n Bookmarks titled "Bookmark 000" upwards, a minute apart.
func insertMany(t *testing.T, m *BookmarkModel, n int) {
	start := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	for i := range n {
		_, err := m.Insert(2, &Bookmark{
			URL:     fmt.Sprintf("https://example.com/%03d", i),
			Title:   fmt.Sprintf("Bookmark %03d", i),
			Created: start.Add(time.Duration(i) * time.Minute),
		})
		assert.NilError(t, err)
	}
}

func TestBookmarkModelListBatches(t *testing.T) {
	m := &BookmarkModel{DB: newTestDB(t)}
	insertMany(t, m, 120)

	for _, sort := range Sorts {
		t.Run(sort, func(t *testing.T) {
			seen := []string{}
			q := ListQuery{Sort: sort}
			for {
				page, err := m.List(2, q)
				assert.NilError(t, err)
				assert.Equal(t, page.Total, 120)
				seen = append(seen, listTitles(page)...)
				if !page.More {
					break
				}
				assert.Equal(t, len(page.Bookmarks), ListBatch)
				q.After = page.Last
			}
			assert.Equal(t, len(seen), 120)
			slices.Sort(seen)
			assert.Equal(t, len(slices.Compact(seen)), 120)
		})
	}
}

func TestBookmarkModelListShowMoreAfterChanges(t *testing.T) {
	m := &BookmarkModel{DB: newTestDB(t)}
	insertMany(t, m, 60)

	first, err := m.List(2, ListQuery{Sort: SortOldest})
	assert.NilError(t, err)
	assert.Equal(t, first.More, true)

	// Deleting a shown row, and the last shown row, mustn't skip anything.
	assert.NilError(t, m.Delete(2, first.Bookmarks[10].ID))
	assert.NilError(t, m.Delete(2, first.Bookmarks[49].ID))

	next, err := m.List(2, ListQuery{Sort: SortOldest, After: first.Last})
	assert.NilError(t, err)
	assert.Equal(t, len(next.Bookmarks), 10)
	assert.Equal(t, next.Bookmarks[0].Title, "Bookmark 050")
	assert.Equal(t, next.More, false)
}

func TestBookmarkModelListUpTo(t *testing.T) {
	m := &BookmarkModel{DB: newTestDB(t)}
	insertMany(t, m, 120)

	first, err := m.List(2, ListQuery{})
	assert.NilError(t, err)
	second, err := m.List(2, ListQuery{After: first.Last})
	assert.NilError(t, err)

	// Reloading down to the second batch's last row shows both batches.
	page, err := m.List(2, ListQuery{UpTo: second.Last})
	assert.NilError(t, err)
	assert.Equal(t, len(page.Bookmarks), 100)
	assert.Equal(t, page.Bookmarks[0].Title, "Bookmark 119")
	assert.Equal(t, page.Bookmarks[99].Title, "Bookmark 020")
	assert.Equal(t, page.Last, second.Last)
	assert.Equal(t, page.More, true)

	// A Bookmark added since appears in its place, at the top.
	_, err = m.Insert(2, &Bookmark{URL: "https://example.com/new", Title: "New",
		Created: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)})
	assert.NilError(t, err)
	page, err = m.List(2, ListQuery{UpTo: second.Last})
	assert.NilError(t, err)
	assert.Equal(t, len(page.Bookmarks), 101)
	assert.Equal(t, page.Bookmarks[0].Title, "New")
}

func TestBookmarkModelListBadMarker(t *testing.T) {
	m, _ := newListModel(t)
	page, err := m.List(2, ListQuery{Sort: SortTitle})
	assert.NilError(t, err)

	for name, q := range map[string]ListQuery{
		"Not base64":    {After: "!!!"},
		"Not JSON":      {After: "bm90IGpzb24"},
		"Another sort":  {Sort: SortNewest, After: page.Last},
		"Wrong keys":    {Sort: SortDomain, UpTo: marker{Sort: SortDomain, Keys: []string{"a"}, ID: 1}.encode()},
		"UpTo is wrong": {UpTo: "x"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := m.List(2, q)
			assert.Equal(t, errors.Is(err, ErrBadMarker), true)
		})
	}
}

func TestBookmarkModelTagCounts(t *testing.T) {
	m, _ := newListModel(t)
	counts := func(f Filter) string {
		tags, err := m.TagCounts(2, f)
		assert.NilError(t, err)
		s := ""
		for _, tag := range tags {
			s += fmt.Sprintf("%s:%d ", tag.Name, tag.BookmarkCount)
		}
		return s
	}
	assert.Equal(t, counts(Filter{}), "go:2 web:2 ")
	assert.Equal(t, counts(Filter{Favourite: true}), "go:1 web:2 ")
	assert.Equal(t, counts(Filter{Tags: []string{"go"}}), "go:2 web:1 ")
	assert.Equal(t, counts(Filter{Domain: "mid.example"}), "")
}
