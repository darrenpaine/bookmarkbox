package models

import (
	"bytes"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/darrenpaine/bookmarkbox/internal/assert"
	"github.com/darrenpaine/bookmarkbox/internal/bookmarkfile"
)

var importNow = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

func TestPrepareImport(t *testing.T) {
	jan := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	feb := time.Date(2020, 2, 1, 0, 0, 0, 0, time.UTC)
	long := strings.Repeat("é", 300)
	links := []bookmarkfile.Link{
		{URL: "chrome://settings", Title: "Settings"},
		{URL: "https://example.com/" + strings.Repeat("a", MaxURLChars), Title: "Too long"},
		{URL: "https://a.example", Title: "A", Folders: []string{"Work, Projects", "  Go  "}, Added: feb},
		{URL: "https://b.example", Title: "  ", Folders: []string{strings.Repeat("x", 60)}},
		{URL: "https://c.example", Title: long, HasTags: true},
		{URL: "https://d.example", Title: "D", Tags: []string{"Web", "go"}, HasTags: true},
		{URL: "https://e.example", Title: "E"},
		{URL: "https://f.example", Title: strings.Repeat("x", MaxTitleChars-1) + " and more"},
		// Repeats: the same once normalised, so merged into the first
		{URL: "https://A.example/", Title: "A again", Folders: []string{"Reading"}, Added: jan, Favourite: true},
		{URL: "https://e.example/?utm_source=x", Title: "E in a Folder", Folders: []string{"Later"}, Description: "Second"},
	}
	f := PrepareImport(links, importNow)
	assert.Equal(t, f.Skipped, 2)
	assert.Equal(t, f.Merged, 2)
	assert.Equal(t, len(f.Items), 6)

	tests := []struct {
		name    string
		item    ImportItem
		title   string
		tags    []string
		created time.Time
		unfiled bool
	}{
		{"Folders become Tags, merged with a repeat", f.Items[0], "A", []string{"go", "reading", "work projects"}, jan, false},
		{"Empty title and long Folder name", f.Items[1], "https://b.example", []string{strings.Repeat("x", 50)}, importNow, false},
		{"Long title, empty TAGS", f.Items[2], strings.Repeat("é", MaxTitleChars), nil, importNow, false},
		{"TAGS attribute", f.Items[3], "D", []string{"go", "web"}, importNow, false},
		{"Unfiled, until a repeat is in a Folder", f.Items[4], "E", []string{"later"}, importNow, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.item.Title, tt.title)
			assert.Equal(t, slices.Equal(tt.item.Tags, tt.tags), true)
			assert.Equal(t, tt.item.Created.Equal(tt.created), true)
			assert.Equal(t, tt.item.Unfiled, tt.unfiled)
		})
	}
	assert.Equal(t, f.Items[0].Favourite, true)
	assert.Equal(t, f.Items[4].Description, "Second")
	// Cut to the limit, without the space left at the end
	assert.Equal(t, f.Items[5].Title, strings.Repeat("x", MaxTitleChars-1))

	// A link in no Folder and with no TAGS is unfiled
	f = PrepareImport([]bookmarkfile.Link{{URL: "https://e.example", Title: "E"}}, importNow)
	assert.Equal(t, f.Items[0].Unfiled, true)
}

func TestImport(t *testing.T) {
	db := newTestDB(t)
	m := &BookmarkModel{DB: db}
	items := PrepareImport([]bookmarkfile.Link{
		{URL: "https://new.example", Title: "New", Folders: []string{"Reading"}, Added: importNow},
		// Alice has this already, with the Tag go
		{URL: "https://go.dev/doc/effective_go/", Title: "Another title", Folders: []string{"Go"}},
		{URL: "https://loose.example", Title: "Loose"},
	}, importNow).Items

	preview, err := m.PreviewImport(1, items)
	assert.NilError(t, err)
	want := ImportSummary{New: 2, Duplicates: 1, Updated: 0, Unfiled: 1, Tags: 2, NewTags: 2,
		BookmarksAfter: 3, TagsAfter: 3}
	assert.Equal(t, *preview, want)

	// Previewing saves nothing
	n, err := m.Count(1, Filter{})
	assert.NilError(t, err)
	assert.Equal(t, n, 1)

	summary, err := m.Import(1, items)
	assert.NilError(t, err)
	assert.Equal(t, *summary, want)

	all, err := m.All(1)
	assert.NilError(t, err)
	assert.Equal(t, len(all), 3)
	// Oldest first; the Duplicate keeps its own title
	assert.Equal(t, all[0].Title, "Effective Go")
	assert.Equal(t, strings.Join(all[0].Tags, ","), "go")
	assert.Equal(t, all[1].Title, "New")
	assert.Equal(t, all[1].Created.Equal(importNow), true)
	assert.Equal(t, strings.Join(all[1].Tags, ","), "reading")
	assert.Equal(t, all[2].Title, "Loose")
	assert.Equal(t, strings.Join(all[2].Tags, ","), UnfiledTag)

	// Bob's Bookmarks are untouched
	n, err = m.Count(2, Filter{})
	assert.NilError(t, err)
	assert.Equal(t, n, 0)

	// Again: everything is a Duplicate. An unfiled Duplicate isn't given
	// unfiled, but a Duplicate does gain any new Tag.
	again := PrepareImport([]bookmarkfile.Link{
		{URL: "https://new.example", Title: "New", Folders: []string{"Reading"}},
		{URL: "https://go.dev/doc/effective_go", Title: "Effective Go"},
		{URL: "https://loose.example", Title: "Loose", Folders: []string{"Extra"}},
	}, importNow).Items
	summary, err = m.Import(1, again)
	assert.NilError(t, err)
	assert.Equal(t, summary.New, 0)
	assert.Equal(t, summary.Duplicates, 3)
	assert.Equal(t, summary.Updated, 1)
	assert.Equal(t, summary.ChangesNothing(), false)
	assert.Equal(t, strings.Join(mustGet(t, m, 1, 1).Tags, ","), "go")
	assert.Equal(t, strings.Join(mustGet(t, m, 1, all[2].ID).Tags, ","), "extra,unfiled")

	preview, err = m.PreviewImport(1, again)
	assert.NilError(t, err)
	assert.Equal(t, preview.ChangesNothing(), true)
}

func TestImportLimits(t *testing.T) {
	items := PrepareImport([]bookmarkfile.Link{
		{URL: "https://a.example", Title: "A", Folders: []string{"one"}},
		{URL: "https://b.example", Title: "B", Folders: []string{"two"}},
	}, importNow).Items

	tests := []struct {
		name         string
		maxBookmarks int
		maxTags      int
		wantErr      error
	}{
		{"Within the limits", 3, 3, nil},
		{"Over the Bookmark limit", 2, 0, ErrBookmarkLimit},
		{"Over the Tag limit", 0, 2, ErrTagLimit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &BookmarkModel{DB: newTestDB(t), MaxBookmarks: tt.maxBookmarks, MaxTags: tt.maxTags}
			preview, err := m.PreviewImport(1, items)
			assert.NilError(t, err)
			assert.Equal(t, preview.OverBookmarkLimit(), errors.Is(tt.wantErr, ErrBookmarkLimit))
			assert.Equal(t, preview.OverTagLimit(), errors.Is(tt.wantErr, ErrTagLimit))

			_, err = m.Import(1, items)
			assert.Equal(t, errors.Is(err, tt.wantErr), true)
			n, err := m.Count(1, Filter{})
			assert.NilError(t, err)
			if tt.wantErr != nil {
				assert.Equal(t, n, 1) // all or nothing
			} else {
				assert.Equal(t, n, 3)
			}
		})
	}
}

// Importing a browser's export: Root Folders don't become Tags, and links
// loose in them are unfiled.
func TestImportBraveExport(t *testing.T) {
	m := &BookmarkModel{DB: newTestDB(t)}
	file, err := os.Open("../bookmarkfile/testdata/brave.html")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	links, err := bookmarkfile.Parse(file, MaxImportLinks)
	assert.NilError(t, err)
	f := PrepareImport(links, importNow)
	assert.Equal(t, f.Skipped, 9) // chrome:, edge:, file: and ftp: links
	assert.Equal(t, f.Merged, 3)

	summary, err := m.Import(2, f.Items)
	assert.NilError(t, err)
	assert.Equal(t, summary.New, 23)
	assert.Equal(t, summary.Unfiled, 3)

	tags := map[string][]string{}
	all, err := m.All(2)
	assert.NilError(t, err)
	for _, b := range all {
		tags[b.URL] = b.Tags
	}
	assert.Equal(t, strings.Join(tags["https://www.example.org/page/1"], ","), "unfiled")
	assert.Equal(t, strings.Join(tags["https://docs.example.com/page/8"], ","), "astronomy,new folder,recipes")
	assert.Equal(t, strings.Join(tags["http://example.com/page/15"], ","), "knitting,photography,sailing")
	assert.Equal(t, strings.Join(tags["https://www.example.org/page/26"], ","), "poetry")
	assert.Equal(t, strings.Join(tags["https://example.com/page/30"], ","), "unfiled")
}

// A BookmarkBox export imported into BookmarkBox loses nothing: every field
// comes back exactly, and the date to the second.
func TestExportImportRoundTrip(t *testing.T) {
	m := &BookmarkModel{DB: newTestDB(t)}
	saved := []*Bookmark{
		{URL: "https://example.com/?a=1&b=\"2\"", Title: `Fish & chips <b>"quoted"</b>`,
			Description: "Line one\r\nline two ", Notes: "Notes\r\n\twith a tab\n",
			Tags: []string{"café", "go", "work things"}, Favourite: true, ReadLater: true,
			Created: time.Date(2024, 5, 6, 7, 8, 9, 123456789, time.UTC)},
		{URL: "https://example.org", Title: "Untagged ☕", Created: time.Date(1999, 1, 2, 3, 4, 5, 0, time.UTC)},
		{URL: "http://example.net/page#part", Title: "Spaces in the description", Description: " ", ReadLater: true,
			Tags: []string{UnfiledTag}},
	}
	for _, b := range saved {
		_, err := m.Insert(2, b)
		assert.NilError(t, err)
	}
	before, err := m.All(2)
	assert.NilError(t, err)
	before = append(before, mustGet(t, m, 1, 1)) // Alice's seeded Bookmark too

	var file bytes.Buffer
	assert.NilError(t, bookmarkfile.Write(&file, ExportLinks(before)))

	// Into a new database, for Alice
	fresh := &BookmarkModel{DB: newTestDB(t)}
	assert.NilError(t, fresh.Delete(1, 1))
	links, err := bookmarkfile.Parse(&file, MaxImportLinks)
	assert.NilError(t, err)
	f := PrepareImport(links, importNow)
	assert.Equal(t, f.Skipped+f.Merged, 0)
	summary, err := fresh.Import(1, f.Items)
	assert.NilError(t, err)
	assert.Equal(t, summary.Unfiled, 0)

	after, err := fresh.All(1)
	assert.NilError(t, err)
	assert.Equal(t, len(after), len(before))
	byURL := map[string]*Bookmark{}
	for _, b := range after {
		byURL[b.URL] = b
	}
	for _, want := range before {
		t.Run(want.URL, func(t *testing.T) {
			got := byURL[want.URL]
			if got == nil {
				t.Fatal("missing")
			}
			assert.Equal(t, got.Title, want.Title)
			assert.Equal(t, got.Description, want.Description)
			assert.Equal(t, got.Notes, want.Notes)
			assert.Equal(t, got.Domain, want.Domain)
			assert.Equal(t, got.Favourite, want.Favourite)
			assert.Equal(t, got.ReadLater, want.ReadLater)
			assert.Equal(t, strings.Join(got.Tags, ","), strings.Join(want.Tags, ","))
			assert.Equal(t, got.Created.Unix(), want.Created.Unix())
		})
	}
}

func mustGet(t *testing.T, m *BookmarkModel, userID, id int) *Bookmark {
	t.Helper()
	b, err := m.Get(userID, id)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPendingImport(t *testing.T) {
	db := newTestDB(t)
	m := &ImportModel{DB: db}

	_, err := m.Get(1)
	assert.Equal(t, errors.Is(err, ErrNoRecord), true)

	assert.NilError(t, m.Save(1, "first.html", []byte("one")))
	assert.NilError(t, m.Save(1, "second.html", []byte("two"))) // replaces the first
	assert.NilError(t, m.Save(2, "bob.html", []byte("bob")))
	p, err := m.Get(1)
	assert.NilError(t, err)
	assert.Equal(t, p.Filename, "second.html")
	assert.Equal(t, string(p.Data), "two")

	// An expired import is gone, and the next save clears it away
	_, err = db.Exec(`UPDATE imports SET created_at = ? WHERE user_id = 2`,
		time.Now().Add(-ImportExpiry-time.Second).Unix())
	assert.NilError(t, err)
	_, err = m.Get(2)
	assert.Equal(t, errors.Is(err, ErrNoRecord), true)
	assert.NilError(t, m.Save(1, "third.html", []byte("three")))
	var rows int
	assert.NilError(t, db.QueryRow(`SELECT count(*) FROM imports`).Scan(&rows))
	assert.Equal(t, rows, 1)

	assert.NilError(t, m.Delete(1))
	_, err = m.Get(1)
	assert.Equal(t, errors.Is(err, ErrNoRecord), true)
}
