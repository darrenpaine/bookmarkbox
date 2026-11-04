package models

import (
	"errors"
	"strings"
)

var (
	ErrNoRecord = errors.New("models: no matching record found")
	// Log in credentials failure
	ErrInvalidCredentials = errors.New("models: invalid credentials")
	// Email already in use when signing up
	ErrDuplicateEmail = errors.New("models: duplicate email")
	// The user has already saved this URL
	ErrDuplicateURL = errors.New("models: duplicate url")
)

// isUniqueViolation reports whether err is an SQLite UNIQUE constraint failure
// on the given "table.column". It matches on the message rather than a driver
// error type, so it works with both modernc.org/sqlite and libSQL (Turso).
func isUniqueViolation(err error, column string) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed: "+column)
}
