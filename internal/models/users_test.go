package models

import (
	"errors"
	"testing"

	"github.com/darrenpaine/bookmarkbox/internal/assert"
)

func TestUserModelExists(t *testing.T) {
	// Long tests - skip if -short is selected
	if testing.Short() {
		t.Skip("models: skipping integration DB tests")
	}
	// Set up a suite of table-driven tests and expected results.
	tests := []struct {
		name   string
		userID int
		want   bool
	}{
		{
			name:   "Valid ID",
			userID: 1,
			want:   true,
		},
		{
			name:   "Zero ID",
			userID: 0,
			want:   false,
		},
		{
			name:   "Non-existent ID",
			userID: 3,
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// New DB for each test
			db := newTestDB(t)
			m := UserModel{DB: db}
			// Call the UserModel.Exists() method and check that the return
			// value and error match the expected values for the sub-test.
			exists, err := m.Exists(tt.userID)
			assert.Equal(t, exists, tt.want)
			assert.NilError(t, err)
		})
	}
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
