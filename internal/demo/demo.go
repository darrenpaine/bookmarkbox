// Package demo runs the public demo's data: the shared Demo User and the
// Bookmarks seeded into its account, recreated from scratch at every reset.
package demo

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"time"

	"github.com/darrenpaine/bookmarkbox/internal/bookmarkfile"
	"github.com/darrenpaine/bookmarkbox/internal/models"
)

// The Demo User's login. It is shown on the home page, so it isn't a secret.
// example.com is reserved, so the email can never reach anyone.
const (
	Name     = "Demo"
	Email    = "demo@example.com"
	Password = "demo-password-1"
)

// Every is how often the data is reset, counted from start-up.
const Every = 24 * time.Hour

// seedFile is a browser bookmark file of public links. Its Folders become the
// Demo User's Tags.
//
//go:embed seed.html
var seedFile []byte

// Seed reads the seed file into the Bookmarks it becomes, with their dates
// moved forward so the newest is now and the rest keep their spacing.
func Seed(now time.Time) ([]models.ImportItem, error) {
	links, err := bookmarkfile.Parse(bytes.NewReader(seedFile), models.MaxImportLinks)
	if err != nil {
		return nil, fmt.Errorf("demo seed: %w", err)
	}
	f := models.PrepareImport(links, now)
	if f.Skipped > 0 || f.Merged > 0 {
		return nil, fmt.Errorf("demo seed: %d links skipped and %d merged", f.Skipped, f.Merged)
	}
	var newest time.Time
	for _, item := range f.Items {
		if item.Created.After(newest) {
			newest = item.Created
		}
	}
	shift := now.Sub(newest)
	for i := range f.Items {
		f.Items[i].Created = f.Items[i].Created.Add(shift)
	}
	return f.Items, nil
}

// Reset deletes every User, with their Bookmarks, Tags and sessions, then
// recreates the Demo User and its seed Bookmarks, all in one transaction. It
// returns the Demo User's ID.
func Reset(ctx context.Context, db *sql.DB, bookmarks *models.BookmarkModel, now time.Time) (int, error) {
	items, err := Seed(now)
	if err != nil {
		return 0, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() // no-op after Commit

	if err = models.DeleteEveryoneTx(ctx, tx); err != nil {
		return 0, err
	}
	id, err := models.InsertUserTx(ctx, tx, Name, Email, Password)
	if err != nil {
		return 0, err
	}
	if _, err = bookmarks.ImportTx(ctx, tx, id, items); err != nil {
		return 0, fmt.Errorf("demo seed: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}
