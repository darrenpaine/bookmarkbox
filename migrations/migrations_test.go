package migrations

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/darrenpaine/bookmarkbox/internal/assert"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

// Bookmarks saved before the domain column existed get their domain filled in.
func TestAddDomainFillsExistingBookmarks(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "test.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	provider, err := goose.NewProvider(goose.DialectSQLite3, db, files, goose.WithGoMigrations(addDomain))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.UpTo(ctx, 1); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO users (name, email, hashed_password, created) VALUES ('A', 'a@example.com', 'x', '2026-01-01');
		INSERT INTO bookmarks (user_id, url, normalised_url, title, created_at) VALUES
			(1, 'https://www.Example.com/a', 'https://www.example.com/a', 'A', '2026-01-02'),
			(1, 'https://go.dev/doc', 'https://go.dev/doc', 'B', '2026-01-03')`)
	if err != nil {
		t.Fatal(err)
	}

	if err = Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	got := map[int]string{}
	rows, err := db.Query(`SELECT id, domain FROM bookmarks`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var domain string
		if err := rows.Scan(&id, &domain); err != nil {
			t.Fatal(err)
		}
		got[id] = domain
	}
	assert.Equal(t, got[1], "example.com")
	assert.Equal(t, got[2], "go.dev")

	// And the migration can be undone.
	if _, err = provider.DownTo(ctx, 1); err != nil {
		t.Fatal(err)
	}
}
