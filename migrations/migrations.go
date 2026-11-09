// Package migrations holds the database schema as numbered goose migrations,
// embedded in the binary so the app can bring a database up to date on start.
// Most are SQL files; a migration that needs Go code is registered below.
package migrations

import (
	"context"
	"database/sql"
	"embed"

	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var files embed.FS

// Up applies any migrations that have not yet been run.
func Up(ctx context.Context, db *sql.DB) error {
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, files,
		goose.WithGoMigrations(addDomain))
	if err != nil {
		return err
	}
	_, err = provider.Up(ctx)
	return err
}
