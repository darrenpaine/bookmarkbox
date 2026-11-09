package models

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/darrenpaine/bookmarkbox/internal/assert"
)

// searchTitles returns the titles of Bob's Bookmarks matching a search, A-Z.
func searchTitles(t *testing.T, m *BookmarkModel, search string) []string {
	t.Helper()
	page, err := m.List(2, ListQuery{Filter: Filter{Search: search}, Sort: SortTitle})
	assert.NilError(t, err)
	assert.Equal(t, page.Total, len(page.Bookmarks))
	return listTitles(page)
}

func TestBookmarkModelSearch(t *testing.T) {
	m, _ := newListModel(t)
	_, err := m.Insert(2, &Bookmark{URL: "https://cafe.example/", Title: "Café notes",
		Description: "Espresso machines compared", Notes: "Ask about the grinder",
		Created: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)})
	assert.NilError(t, err)

	tests := []struct {
		name, search string
		want         []string
	}{
		{"Title", "charlie", []string{"Charlie"}},
		{"Start of a word", "del", []string{"Delta"}},
		{"Description", "espresso", []string{"Café notes"}},
		{"Notes", "grinder", []string{"Café notes"}},
		{"URL", "zeta", []string{"beta"}},
		{"Tag", "go", []string{"beta", "Delta"}},
		{"Every word must match", "alpha web", []string{"alpha"}},
		{"Accents don't matter", "cafe", []string{"Café notes"}},
		{"Case doesn't matter", "CHARLIE", []string{"Charlie"}},
		{"No match", "nothing", []string{}},
		{"Search syntax is just text", `go OR -web "NEAR(`, []string{}},
		{"A lone quote", `"delta`, []string{"Delta"}},
		{"No letters or digits", "-- ** ()", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := searchTitles(t, m, tt.search); !slices.Equal(got, tt.want) {
				t.Errorf("got %q; want %q", got, tt.want)
			}
		})
	}

	t.Run("With other Filters", func(t *testing.T) {
		page, err := m.List(2, ListQuery{Filter: Filter{Search: "alpha", Favourite: true}})
		assert.NilError(t, err)
		assert.Equal(t, listTitles(page)[0], "alpha")
		assert.Equal(t, page.Total, 1)
	})

	t.Run("Other users' Bookmarks are never found", func(t *testing.T) {
		page, err := m.List(1, ListQuery{Filter: Filter{Search: "charlie"}, Sort: SortMatch})
		assert.NilError(t, err)
		assert.Equal(t, page.Total, 0)
	})

	t.Run("Tag counts", func(t *testing.T) {
		tags, err := m.TagCounts(2, Filter{Search: "alpha"})
		assert.NilError(t, err)
		assert.Equal(t, len(tags), 2) // alpha has web, Delta (alpha.example) has go
	})
}

// The index follows every change to a Bookmark and its Tags.
func TestBookmarkModelSearchFollowsChanges(t *testing.T) {
	m, ids := newListModel(t)
	tags := &TagModel{DB: m.DB}
	find := func(search string) []string { return searchTitles(t, m, search) }

	assert.NilError(t, m.Update(2, &Bookmark{ID: ids["Charlie"], URL: "https://mid.example/", Title: "Charles"}))
	assert.Equal(t, len(find("charlie")), 0)
	assert.Equal(t, find("charles")[0], "Charles")

	assert.NilError(t, m.AddTag(2, ids["Charlie"], "kubernetes"))
	assert.Equal(t, find("kube")[0], "Charles")
	assert.NilError(t, m.RemoveTag(2, ids["Charlie"], "kubernetes"))
	assert.Equal(t, len(find("kube")), 0)

	// Renaming a Tag changes every Bookmark that has it.
	list, err := tags.List(2)
	assert.NilError(t, err)
	byName := map[string]int{}
	for _, tag := range list {
		byName[tag.Name] = tag.ID
	}
	assert.NilError(t, tags.Rename(2, byName["web"], "frontend"))
	if got := find("frontend"); !slices.Equal(got, []string{"alpha", "beta"}) {
		t.Errorf("after rename got %q", got)
	}
	assert.Equal(t, len(find("web")), 0)

	// Merging moves a Tag's Bookmarks to another.
	assert.NilError(t, tags.Merge(2, byName["web"], "go")) // the same Tag, now named frontend
	if got := find("go"); !slices.Equal(got, []string{"alpha", "beta", "Delta"}) {
		t.Errorf("after merge got %q", got)
	}
	assert.Equal(t, len(find("frontend")), 0)

	// Favourite and Read later changes leave the index alone.
	assert.NilError(t, m.ToggleFavourite(2, ids["Delta"]))
	assert.Equal(t, find("delta")[0], "Delta")

	assert.NilError(t, m.Delete(2, ids["Delta"]))
	assert.Equal(t, len(find("delta")), 0)
	var rows int
	assert.NilError(t, m.DB.QueryRow(`SELECT count(*) FROM bookmarks_fts`).Scan(&rows))
	assert.Equal(t, rows, 4) // Alice's seeded Bookmark, and Bob's other three
}

func TestBookmarkModelSearchBestMatch(t *testing.T) {
	m := &BookmarkModel{DB: newTestDB(t)}
	day := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	for i, b := range []*Bookmark{
		{URL: "https://a.example/", Title: "Kitchen", Notes: "A note on sourdough"},
		{URL: "https://b.example/", Title: "Sourdough starter"},
		{URL: "https://c.example/", Title: "Bread", Tags: []string{"sourdough"}},
	} {
		b.Created = day.AddDate(0, 0, i)
		_, err := m.Insert(2, b)
		assert.NilError(t, err)
	}
	page, err := m.List(2, ListQuery{Filter: Filter{Search: "sourdough"}, Sort: SortMatch})
	assert.NilError(t, err)
	// The title counts for most, then the Tags, and the notes least.
	if got := listTitles(page); !slices.Equal(got, []string{"Sourdough starter", "Bread", "Kitchen"}) {
		t.Errorf("got %q", got)
	}

	t.Run("Without a search, the newest come first", func(t *testing.T) {
		page, err := m.List(2, ListQuery{Sort: SortMatch})
		assert.NilError(t, err)
		assert.Equal(t, listTitles(page)[0], "Bread")
	})

	t.Run("Markers don't apply", func(t *testing.T) {
		_, err := m.List(2, ListQuery{Filter: Filter{Search: "sourdough"}, Sort: SortMatch, After: "x"})
		assert.Equal(t, errors.Is(err, ErrBadMarker), true)
	})
}

func TestBookmarkModelSearchCap(t *testing.T) {
	m := &BookmarkModel{DB: newTestDB(t)}
	insertMany(t, m, 120)

	page, err := m.List(2, ListQuery{Filter: Filter{Search: "bookmark"}, Sort: SortMatch})
	assert.NilError(t, err)
	assert.Equal(t, len(page.Bookmarks), SearchMax)
	assert.Equal(t, page.Total, 120)
	assert.Equal(t, page.More, false)
	assert.Equal(t, page.Bookmarks[0].Title, "Bookmark 119") // equal matches: newest first

	// Another sort shows every match, in batches.
	page, err = m.List(2, ListQuery{Filter: Filter{Search: "bookmark"}, Sort: SortOldest})
	assert.NilError(t, err)
	assert.Equal(t, len(page.Bookmarks), ListBatch)
	assert.Equal(t, page.More, true)
}

// bm25 takes its weights by position, so they must name every column of the
// search index, in order.
func TestMatchWeightsFollowTheIndex(t *testing.T) {
	db := newTestDB(t)
	rows, err := db.Query(`SELECT name FROM pragma_table_info('bookmarks_fts') ORDER BY cid`)
	assert.NilError(t, err)
	defer rows.Close()
	columns := []string{}
	for rows.Next() {
		var name string
		assert.NilError(t, rows.Scan(&name))
		columns = append(columns, name)
	}
	assert.NilError(t, rows.Err())

	weighted := []string{}
	for _, w := range matchWeights {
		weighted = append(weighted, w.column)
	}
	if !slices.Equal(weighted, columns) {
		t.Errorf("matchWeights has columns %q; bookmarks_fts has %q", weighted, columns)
	}
	assert.Equal(t, bm25Weights(), "10, 4, 3, 1, 6")
}
