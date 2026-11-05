package models

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

var (
	ErrNoRecord = errors.New("models: no matching record found")
	// Log in credentials failure
	ErrInvalidCredentials = errors.New("models: invalid credentials")
	// Email already in use when signing up
	ErrDuplicateEmail = errors.New("models: duplicate email")
	// The user already has a bookmark with this normalised URL
	ErrDuplicateURL = errors.New("models: duplicate url")
	// The user already has a tag with this name
	ErrDuplicateTag = errors.New("models: duplicate tag")
	// A save would take the site or user past a configured limit
	ErrUserLimit     = errors.New("models: user limit reached")
	ErrBookmarkLimit = errors.New("models: bookmark limit reached")
	ErrTagLimit      = errors.New("models: tag limit reached")
)

// overLimit reports whether a count query inside a transaction exceeds limit;
// a limit of 0 means unlimited. Callers make their changes first, count, and
// roll back if over. SQLite allows one writer at a time and the transaction has
// already written, so no concurrent request can change the count in between.
func overLimit(ctx context.Context, tx *sql.Tx, limit int, query string, args ...any) (bool, error) {
	if limit <= 0 {
		return false, nil
	}
	var count int
	if err := tx.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return false, err
	}
	return count > limit, nil
}

// isUniqueViolation reports whether err is an SQLite UNIQUE constraint failure
// on the given columns, written as SQLite reports them ("table.column", or
// "table.a, table.b" for a composite constraint).
func isUniqueViolation(err error, column string) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed: "+column)
}
