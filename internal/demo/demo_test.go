package demo

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/darrenpaine/bookmarkbox/internal/assert"
	"github.com/darrenpaine/bookmarkbox/internal/models"
	"github.com/darrenpaine/bookmarkbox/migrations"
	_ "modernc.org/sqlite"
)

// The demo's limits, as render.yaml sets them.
const (
	demoMaxBookmarks = 300
	demoMaxTags      = 300
)

var now = time.Date(2026, 11, 12, 9, 30, 0, 0, time.UTC)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "demo.db") + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = migrations.Up(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return db
}

func count(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The seed parses with nothing skipped or repeated, fits the demo's limits,
// and ends at the moment of the reset.
func TestSeed(t *testing.T) {
	items, err := Seed(now)
	assert.NilError(t, err)
	if len(items) < 150 || len(items) > demoMaxBookmarks {
		t.Fatalf("got %d seed Bookmarks", len(items))
	}

	tags := map[string]bool{}
	var newest, oldest time.Time
	var favourites, readLater, unfiled int
	for i, item := range items {
		for _, name := range item.Tags {
			tags[name] = true
		}
		if item.Unfiled {
			unfiled++
			tags[models.UnfiledTag] = true
		}
		if item.Favourite {
			favourites++
		}
		if item.ReadLater {
			readLater++
		}
		if i == 0 || item.Created.After(newest) {
			newest = item.Created
		}
		if i == 0 || item.Created.Before(oldest) {
			oldest = item.Created
		}
	}
	if len(tags) < 20 || len(tags) > demoMaxTags {
		t.Errorf("got %d Tags", len(tags))
	}
	assert.Equal(t, newest, now)
	if span := newest.Sub(oldest); span < 300*24*time.Hour || span > 366*24*time.Hour {
		t.Errorf("the dates span %s, not about a year", span)
	}
	if favourites == 0 || readLater == 0 || unfiled == 0 {
		t.Errorf("got %d favourites, %d Read later and %d unfiled", favourites, readLater, unfiled)
	}
}

func TestReset(t *testing.T) {
	db := newTestDB(t)
	users := &models.UserModel{DB: db}
	bookmarks := &models.BookmarkModel{DB: db, MaxBookmarks: demoMaxBookmarks, MaxTags: demoMaxTags}
	imports := &models.ImportModel{DB: db}

	// A visitor signs up, saves a Bookmark, starts an import and has a session.
	assert.NilError(t, users.Insert("Visitor", "visitor@example.com", "visitor-password"))
	visitor, err := users.Authenticate("visitor@example.com", "visitor-password")
	assert.NilError(t, err)
	_, err = bookmarks.Insert(visitor, &models.Bookmark{URL: "https://example.org/", Title: "Example", Tags: []string{"mine"}, Created: now})
	assert.NilError(t, err)
	assert.NilError(t, imports.Save(visitor, "bookmarks.html", []byte("<a href='https://example.net/'>x</a>")))
	_, err = db.Exec("INSERT INTO sessions (token, data, expiry) VALUES ('abc', x'00', 1e12)")
	assert.NilError(t, err)

	items, err := Seed(now)
	assert.NilError(t, err)

	// A reset at start-up, then a day later.
	for _, at := range []time.Time{now, now.Add(Every)} {
		id, err := Reset(context.Background(), db, bookmarks, at)
		assert.NilError(t, err)

		assert.Equal(t, count(t, db, "SELECT count(*) FROM users"), 1)
		assert.Equal(t, count(t, db, "SELECT count(*) FROM sessions"), 0)
		assert.Equal(t, count(t, db, "SELECT count(*) FROM imports"), 0)
		assert.Equal(t, count(t, db, "SELECT count(*) FROM tags WHERE name = 'mine'"), 0)

		loggedIn, err := users.Authenticate(Email, Password)
		assert.NilError(t, err)
		assert.Equal(t, loggedIn, id)
		n, err := bookmarks.Count(id, models.Filter{})
		assert.NilError(t, err)
		assert.Equal(t, n, len(items))

		var newest time.Time
		assert.NilError(t, db.QueryRow("SELECT created_at FROM bookmarks ORDER BY created_at DESC LIMIT 1").Scan(&newest))
		assert.Equal(t, newest.UTC(), at)
	}
}
