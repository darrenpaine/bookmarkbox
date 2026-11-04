package models

import (
	"database/sql"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Base interface for UserModel
type UserModelInterface interface {
	Insert(name, email, password string) error
	Authenticate(email, password string) (int, error)
	Exists(id int) (bool, error)
	Get(id int) (*User, error)
	PasswordUpdate(id int, currentPassword, newPassword string) error
}

// User type aligned with the database "users" table.
type User struct {
	UserModelInterface
	ID             int
	Name           string
	Email          string
	HashedPassword []byte
	Created        time.Time
}

// UserModel type wraps a database connection pool.
type UserModel struct {
	DB *sql.DB
}

// Add a new record to the "users" table.
func (m *UserModel) Insert(name, email, password string) error {
	// Create a bcrypt hash of the plain-text password.
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return err
	}
	stmt := `INSERT INTO users (name, email, hashed_password, created)
             VALUES(?, ?, ?, ?)`
	_, err = m.DB.Exec(stmt, name, email, string(hashedPassword), time.Now().UTC())
	if err != nil {
		// The email column is UNIQUE (and case-insensitive).
		if isUniqueViolation(err, "users.email") {
			return ErrDuplicateEmail
		}
		return err
	}
	return nil
}

// Check if the credentials match a user.
// Return the relevant user ID if they do. Else ErrInvalidCredentials.
func (m *UserModel) Authenticate(email, password string) (int, error) {
	var id int
	var hashedPassword []byte
	stmt := "SELECT id, hashed_password FROM users WHERE email = ?"
	err := m.DB.QueryRow(stmt, email).Scan(&id, &hashedPassword)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrInvalidCredentials
		} else {
			return 0, err
		}
	}
	// Check whether the hashed password and plain-text password provided match. // If they don't, we return the ErrInvalidCredentials error.
	err = bcrypt.CompareHashAndPassword(hashedPassword, []byte(password))
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return 0, ErrInvalidCredentials
		} else {
			return 0, err
		}
	}
	// Otherwise, the password is correct. Return the user ID.
	return id, nil
}

// Check if a user exists with a specific ID.
func (m *UserModel) Exists(id int) (bool, error) {
	var exists bool
	stmt := "SELECT EXISTS(SELECT true FROM users WHERE id = ?)"
	err := m.DB.QueryRow(stmt, id).Scan(&exists)
	return exists, err
}

// Get user details for a given ID
func (m *UserModel) Get(id int) (*User, error) {
	var (
		name    string
		email   string
		created time.Time
	)
	stmt := "SELECT name, email, created FROM users WHERE id = ?"
	err := m.DB.QueryRow(stmt, id).Scan(&name, &email, &created)
	if err != nil {
		return &User{}, ErrNoRecord
	}
	return &User{
		ID:      id,
		Name:    name,
		Email:   email,
		Created: created,
	}, nil
}

// Check current and new and update the user password
func (m *UserModel) PasswordUpdate(id int, currentPassword, newPassword string) error {
	var (
		currentHash string
	)
	stmt := "SELECT hashed_password FROM users WHERE id = ?"
	err := m.DB.QueryRow(stmt, id).Scan(&currentHash)
	if err != nil {
		return ErrNoRecord
	}
	// Check whether the hashed password and plain-text password provided match. // If they don't, we return the ErrInvalidCredentials error.
	err = bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(currentPassword))
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return ErrInvalidCredentials
		} else {
			return err
		}
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), 12)
	if err != nil {
		return err
	}
	stmt = `UPDATE users SET hashed_password = ? WHERE id = ?`
	_, err = m.DB.Exec(stmt, string(newHash), id)
	if err != nil {
		return err
	}

	return nil
}
