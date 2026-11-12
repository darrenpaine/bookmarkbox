package models

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// bcryptCost is the work factor for password hashes.
const bcryptCost = 12

// MaxPasswordBytes is the longest password bcrypt takes. Longer ones are
// refused rather than cut short.
const MaxPasswordBytes = 72

// dummyHash is compared against when an email has no account. It is a hash
// of a password no account has, at bcryptCost.
var dummyHash = []byte("$2a$12$KXmL2L8Ixzv0fDWHW5rVfO7n1d1c7KMYDBPX3dRO1WkT1IROHXAKi")

// Base interface for UserModel
type UserModelInterface interface {
	Insert(name, email, password string) error
	Authenticate(email, password string) (int, error)
	Viewer(id int) (*Viewer, error)
	SetTheme(id int, theme string) error
	Get(id int) (*User, error)
	PasswordUpdate(id int, currentPassword, newPassword string) error
	SignupOpen() (bool, error)
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

// The themes a User can choose. ThemeSystem follows the device's setting.
const (
	ThemeSystem = "system"
	ThemeLight  = "light"
	ThemeDark   = "dark"
)

// ValidTheme reports whether theme is one a User can choose.
func ValidTheme(theme string) bool {
	return theme == ThemeSystem || theme == ThemeLight || theme == ThemeDark
}

// Viewer is what every page needs about the logged-in User.
type Viewer struct {
	ID    int
	Name  string
	Theme string
}

// UserModel type wraps a database connection pool. Signup closes at MaxUsers
// users; 0 means unlimited.
type UserModel struct {
	DB       *sql.DB
	MaxUsers int
}

// SignupOpen reports whether another user can sign up.
func (m *UserModel) SignupOpen() (bool, error) {
	if m.MaxUsers <= 0 {
		return true, nil
	}
	var count int
	if err := m.DB.QueryRow("SELECT count(*) FROM users").Scan(&count); err != nil {
		return false, err
	}
	return count < m.MaxUsers, nil
}

// Add a new record to the "users" table. Returns ErrDuplicateEmail, or
// ErrUserLimit if signup is full.
func (m *UserModel) Insert(name, email, password string) error {
	ctx := context.Background()
	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op after Commit

	if _, err = InsertUserTx(ctx, tx, name, email, password); err != nil {
		return err
	}
	over, err := overLimit(ctx, tx, m.MaxUsers, "SELECT count(*) FROM users")
	if err != nil {
		return err
	}
	if over {
		return ErrUserLimit
	}
	return tx.Commit()
}

// InsertUserTx adds a User inside a transaction the caller commits, and
// returns its ID. It doesn't check the User limit. Returns ErrDuplicateEmail
// if the email is taken.
func InsertUserTx(ctx context.Context, tx *sql.Tx, name, email, password string) (int, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return 0, err
	}
	var id int
	err = tx.QueryRowContext(ctx,
		`INSERT INTO users (name, email, hashed_password, created) VALUES (?, ?, ?, ?) RETURNING id`,
		name, email, string(hashedPassword), time.Now().UTC()).Scan(&id)
	if err != nil {
		// The email column is UNIQUE (and case-insensitive).
		if isUniqueViolation(err, "users.email") {
			return 0, ErrDuplicateEmail
		}
		return 0, err
	}
	return id, nil
}

// DeleteEveryoneTx removes every User and session, inside a transaction the
// caller commits. Each User's Bookmarks, Tags and pending import go with them.
func DeleteEveryoneTx(ctx context.Context, tx *sql.Tx) error {
	for _, stmt := range []string{"DELETE FROM sessions", "DELETE FROM users"} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
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
			// Take as long as a wrong password would, so the time taken
			// doesn't show which emails have an account.
			bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
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

// Viewer returns the details every page needs for a logged-in User, or
// ErrNoRecord if the User no longer exists.
func (m *UserModel) Viewer(id int) (*Viewer, error) {
	v := &Viewer{ID: id}
	err := m.DB.QueryRow("SELECT name, theme FROM users WHERE id = ?", id).Scan(&v.Name, &v.Theme)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoRecord
	}
	if err != nil {
		return nil, err
	}
	return v, nil
}

// SetTheme saves a User's theme. It returns ErrInvalidTheme for a theme
// they can't choose, and ErrNoRecord if the User doesn't exist.
func (m *UserModel) SetTheme(id int, theme string) error {
	if !ValidTheme(theme) {
		return ErrInvalidTheme
	}
	res, err := m.DB.Exec("UPDATE users SET theme = ? WHERE id = ?", theme, id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNoRecord
	}
	return nil
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

	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcryptCost)
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
