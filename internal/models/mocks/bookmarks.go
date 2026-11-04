package mocks

import (
	"time"

	"github.com/darrenpaine/bookmarkbox/internal/models"
)

// Bookmark 1 belongs to user 1 (alice@example.com in the user mock).
var mockBookmark = &models.Bookmark{
	ID:          1,
	UserID:      1,
	URL:         "https://go.dev/doc/effective_go",
	Title:       "Effective Go",
	Description: "Tips for writing clear, idiomatic Go code.",
	Created:     time.Now(),
	Tags:        []string{"docs", "go"},
}

// DuplicateURL is rejected by Update, as if another bookmark already had it.
const DuplicateURL = "https://duplicate.example.com"

type BookmarkModel struct{}

func (m *BookmarkModel) Insert(userID int, b *models.Bookmark) (int, error) {
	if b.URL == mockBookmark.URL && userID == mockBookmark.UserID {
		return 0, models.ErrDuplicateURL
	}
	return 2, nil
}

func (m *BookmarkModel) Get(userID, id int) (*models.Bookmark, error) {
	if id == mockBookmark.ID && userID == mockBookmark.UserID {
		return mockBookmark, nil
	}
	return nil, models.ErrNoRecord
}

func (m *BookmarkModel) Latest(userID, limit int) ([]*models.Bookmark, error) {
	if userID == mockBookmark.UserID {
		return []*models.Bookmark{mockBookmark}, nil
	}
	return []*models.Bookmark{}, nil
}

func (m *BookmarkModel) Update(userID int, b *models.Bookmark) error {
	if _, err := m.Get(userID, b.ID); err != nil {
		return err
	}
	if b.URL == DuplicateURL {
		return models.ErrDuplicateURL
	}
	return nil
}

func (m *BookmarkModel) ToggleFavourite(userID, id int) error {
	_, err := m.Get(userID, id)
	return err
}

func (m *BookmarkModel) Delete(userID, id int) error {
	_, err := m.Get(userID, id)
	return err
}
