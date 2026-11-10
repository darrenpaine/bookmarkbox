package mocks

import (
	"github.com/darrenpaine/bookmarkbox/internal/models"
)

// FullEmail is rejected by Insert as if another signup took the last place.
const FullEmail = "full@example.com"

// SignupClosed makes SignupOpen report that signup is full. Theme is the
// theme of User 1, Alice, and SetTheme changes it.
type UserModel struct {
	SignupClosed bool
	Theme        string
}

func (m *UserModel) SignupOpen() (bool, error) {
	return !m.SignupClosed, nil
}

func (m *UserModel) Insert(name, email, password string) error {
	switch email {
	case "dupe@example.com":
		return models.ErrDuplicateEmail
	case FullEmail:
		return models.ErrUserLimit
	default:
		return nil
	}
}
func (m *UserModel) Authenticate(email, password string) (int, error) {
	if email == "alice@example.com" && password == "pa$$word" {
		return 1, nil
	}
	return 0, models.ErrInvalidCredentials
}
func (m *UserModel) Viewer(id int) (*models.Viewer, error) {
	if id != 1 {
		return nil, models.ErrNoRecord
	}
	theme := m.Theme
	if theme == "" {
		theme = models.ThemeSystem
	}
	return &models.Viewer{ID: 1, Name: "Alice Jones", Theme: theme}, nil
}

func (m *UserModel) SetTheme(id int, theme string) error {
	if !models.ValidTheme(theme) {
		return models.ErrInvalidTheme
	}
	if id != 1 {
		return models.ErrNoRecord
	}
	m.Theme = theme
	return nil
}

func (m *UserModel) Get(id int) (*models.User, error) {
	return &models.User{}, nil
}

func (m *UserModel) PasswordUpdate(id int, currentPassword, newPassword string) error {
	switch id {
	case 1:
		if currentPassword != "pa$$word" {
			return models.ErrInvalidCredentials
		}
		return nil
	default:
		return models.ErrNoRecord
	}
}
