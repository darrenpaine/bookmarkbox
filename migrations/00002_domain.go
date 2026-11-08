package migrations

import (
	"context"
	"database/sql"

	"github.com/darrenpaine/bookmarkbox/internal/urls"
	"github.com/pressly/goose/v3"
)

// addDomain stores each Bookmark's domain, so lists can filter and sort by it.
// It is a Go migration because existing Bookmarks get their domain from the
// same urls.Domain rule the app uses on every save, which SQL can't express.
var addDomain = goose.NewGoMigration(2,
	&goose.GoFunc{RunTx: func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			ALTER TABLE bookmarks ADD COLUMN domain TEXT NOT NULL DEFAULT '';
			CREATE INDEX bookmarks_user_domain_idx ON bookmarks (user_id, domain)`)
		if err != nil {
			return err
		}
		return fillDomains(ctx, tx)
	}},
	&goose.GoFunc{RunTx: func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			DROP INDEX bookmarks_user_domain_idx;
			ALTER TABLE bookmarks DROP COLUMN domain`)
		return err
	}},
)

func fillDomains(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, url FROM bookmarks`)
	if err != nil {
		return err
	}
	domains := map[int]string{}
	for rows.Next() {
		var id int
		var url string
		if err := rows.Scan(&id, &url); err != nil {
			rows.Close()
			return err
		}
		domains[id] = urls.Domain(url)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for id, domain := range domains {
		if _, err := tx.ExecContext(ctx, `UPDATE bookmarks SET domain = ? WHERE id = ?`, domain, id); err != nil {
			return err
		}
	}
	return nil
}
