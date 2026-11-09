package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/darrenpaine/bookmarkbox/internal/assert"
)

func env(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func TestParseConfigDefaults(t *testing.T) {
	cfg, err := parseConfig(nil, env(nil))
	assert.NilError(t, err)
	assert.Equal(t, cfg, config{addr: ":4000", dbPath: "./bookmarkbox.db", cookieSecure: true, maxUsers: 1})
}

func TestParseConfigEnvironment(t *testing.T) {
	cfg, err := parseConfig(nil, env(map[string]string{
		"PORT":                   "8080",
		"DATABASE_PATH":          "/data/box.db",
		"COOKIE_SECURE":          "false",
		"MAX_USERS":              "5",
		"MAX_BOOKMARKS_PER_USER": "300",
		"MAX_TAGS_PER_USER":      "200",
	}))
	assert.NilError(t, err)
	assert.Equal(t, cfg, config{
		addr: ":8080", dbPath: "/data/box.db", maxUsers: 5, maxBookmarks: 300, maxTags: 200,
	})
}

func TestParseConfigFlagsOverrideEnvironment(t *testing.T) {
	args := []string{"-addr", "127.0.0.1:5000", "-db", "flag.db", "-cookie-secure=false", "-max-users", "0", "-debug"}
	cfg, err := parseConfig(args, env(map[string]string{
		"PORT": "8080", "DATABASE_PATH": "env.db", "COOKIE_SECURE": "true", "MAX_USERS": "5",
	}))
	assert.NilError(t, err)
	assert.Equal(t, cfg, config{addr: "127.0.0.1:5000", dbPath: "flag.db", debug: true})
}

func TestParseConfigRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
	}{
		{name: "Port is text", env: map[string]string{"PORT": "http"}},
		{name: "Port out of range", env: map[string]string{"PORT": "70000"}},
		{name: "Cookie setting is text", env: map[string]string{"COOKIE_SECURE": "maybe"}},
		{name: "User limit is text", env: map[string]string{"MAX_USERS": "abc"}},
		{name: "Negative user limit", env: map[string]string{"MAX_USERS": "-1"}},
		{name: "Bookmark limit is text", env: map[string]string{"MAX_BOOKMARKS_PER_USER": "1k"}},
		{name: "Tag limit is text", env: map[string]string{"MAX_TAGS_PER_USER": "1.5"}},
		{name: "Negative flag", args: []string{"-max-tags=-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseConfig(tt.args, env(tt.env)); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

// Every path must keep foreign keys on, including one that looks like a URI.
func TestSQLiteDSN(t *testing.T) {
	for _, name := range []string{"plain.db", "with space.db", "what?#100%.db"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			db, err := openDB(sqliteDSN(path))
			assert.NilError(t, err)
			defer db.Close()

			var foreignKeys, busyTimeout int
			var journalMode, file string
			assert.NilError(t, db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys))
			assert.NilError(t, db.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout))
			assert.NilError(t, db.QueryRow("PRAGMA journal_mode").Scan(&journalMode))
			assert.NilError(t, db.QueryRow("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&file))
			assert.Equal(t, foreignKeys, 1)
			assert.Equal(t, busyTimeout, 5000)
			assert.Equal(t, journalMode, "wal")
			assert.Equal(t, file, path)

			// A transaction holds the write lock from the start, so another
			// connection can't take it, even before the first has written.
			tx, err := db.Begin()
			assert.NilError(t, err)
			defer tx.Rollback()
			other, err := db.Conn(context.Background())
			assert.NilError(t, err)
			defer other.Close()
			_, err = other.ExecContext(context.Background(), "PRAGMA busy_timeout = 0")
			assert.NilError(t, err)
			if _, err = other.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err == nil {
				t.Error("a second connection took the write lock")
			}
		})
	}
}
