package models

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/darrenpaine/bookmarkbox/migrations"
	_ "modernc.org/sqlite"
)

// newTestDB creates a fresh SQLite database file in a temporary directory,
// applies the migrations and loads testdata/setup.sql. The directory (and so
// the database) is removed automatically when the test ends.
func newTestDB(t *testing.T) *sql.DB {
	dsn := "file:" + filepath.Join(t.TempDir(), "test.db") + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if err = migrations.Up(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile("./testdata/setup.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(script)); err != nil {
		t.Fatal(err)
	}
	return db
}
