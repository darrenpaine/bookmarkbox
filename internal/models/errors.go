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
	// The user already has a bookmark with this normalised URL
	ErrDuplicateURL = errors.New("models: duplicate url")
)

// isUniqueViolation reports whether err is an SQLite UNIQUE constraint failure
// on the given columns, written as SQLite reports them ("table.column", or
// "table.a, table.b" for a composite constraint).
func isUniqueViolation(err error, column string) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed: "+column)
}
