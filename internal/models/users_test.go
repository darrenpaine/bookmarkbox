package models

import (
	"errors"
	"testing"
	"time"

	"github.com/darrenpaine/bookmarkbox/internal/assert"
	"golang.org/x/crypto/bcrypt"
)

func TestUserModelViewer(t *testing.T) {
	if testing.Short() {
		t.Skip("models: skipping integration DB tests")
	}
	db := newTestDB(t)
	m := UserModel{DB: db}

	v, err := m.Viewer(1)
	assert.NilError(t, err)
	assert.Equal(t, *v, Viewer{ID: 1, Name: "Alice Jones", Theme: ThemeSystem})

	for _, id := range []int{0, 3} {
		_, err = m.Viewer(id)
		assert.Equal(t, errors.Is(err, ErrNoRecord), true)
	}
}

func TestUserModelSetTheme(t *testing.T) {
	if testing.Short() {
		t.Skip("models: skipping integration DB tests")
	}
	db := newTestDB(t)
	m := UserModel{DB: db}

	for _, theme := range []string{ThemeDark, ThemeLight, ThemeSystem} {
		assert.NilError(t, m.SetTheme(1, theme))
		v, err := m.Viewer(1)
		assert.NilError(t, err)
		assert.Equal(t, v.Theme, theme)
	}
	// Only Alice changed.
	v, err := m.Viewer(2)
	assert.NilError(t, err)
	assert.Equal(t, v.Theme, ThemeSystem)

	assert.Equal(t, errors.Is(m.SetTheme(1, "sepia"), ErrInvalidTheme), true)
	assert.Equal(t, errors.Is(m.SetTheme(1, ""), ErrInvalidTheme), true)
	assert.Equal(t, errors.Is(m.SetTheme(3, ThemeDark), ErrNoRecord), true)

	// The database refuses other values too, whatever the code does.
	_, err = db.Exec("UPDATE users SET theme = 'sepia' WHERE id = 1")
	assert.Equal(t, err != nil, true)
}

func TestUserModelInsertDuplicateEmail(t *testing.T) {
	if testing.Short() {
		t.Skip("models: skipping integration DB tests")
	}
	db := newTestDB(t)
	m := UserModel{DB: db}

	assert.NilError(t, m.Insert("Carol", "carol@example.com", "pa$$word"))
	// Email is unique regardless of case.
	err := m.Insert("Alice again", "ALICE@example.com", "pa$$word")
	assert.Equal(t, errors.Is(err, ErrDuplicateEmail), true)
}

func TestUserLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("models: skipping integration DB tests")
	}
	db := newTestDB(t) // has two users
	m := UserModel{DB: db, MaxUsers: 3}

	open, err := m.SignupOpen()
	assert.NilError(t, err)
	assert.Equal(t, open, true)

	assert.NilError(t, m.Insert("Carol", "carol@example.com", "pa$$word"))

	open, err = m.SignupOpen()
	assert.NilError(t, err)
	assert.Equal(t, open, false)

	err = m.Insert("Dan", "dan@example.com", "pa$$word")
	assert.Equal(t, errors.Is(err, ErrUserLimit), true)

	var count int
	assert.NilError(t, db.QueryRow("SELECT count(*) FROM users").Scan(&count))
	assert.Equal(t, count, 3)

	t.Run("Unlimited", func(t *testing.T) {
		m := UserModel{DB: db}
		open, err := m.SignupOpen()
		assert.NilError(t, err)
		assert.Equal(t, open, true)
		assert.NilError(t, m.Insert("Dan", "dan@example.com", "pa$$word"))
	})
}

// An email with no account takes as long to refuse as a wrong password, so
// timing the login form doesn't show who has an account.
func TestUserModelAuthenticateTiming(t *testing.T) {
	if testing.Short() {
		t.Skip("models: skipping integration DB tests")
	}
	cost, err := bcrypt.Cost(dummyHash)
	assert.NilError(t, err)
	assert.Equal(t, cost, bcryptCost)

	m := UserModel{DB: newTestDB(t)}
	timed := func(email string) time.Duration {
		start := time.Now()
		_, err := m.Authenticate(email, "wrong-password")
		assert.Equal(t, errors.Is(err, ErrInvalidCredentials), true)
		return time.Since(start)
	}
	known := timed("alice@example.com")
	unknown := timed("nobody@example.com")
	// bcrypt at cost 12 takes hundreds of milliseconds; skipping it, under a
	// millisecond. Half is a wide margin for a busy machine.
	if unknown < known/2 {
		t.Errorf("unknown email took %v, a wrong password %v", unknown, known)
	}
}
