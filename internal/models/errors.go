package models

import (
	"errors"
)

var (
	ErrNoRecord = errors.New("models: no matching record found")
	// Log in credentials failure
	ErrInvalidCredentials = errors.New("models: invalid credentials")
	// Email already in use when signing up
	ErrDuplicateEmail = errors.New("models: duplicate email")
)
