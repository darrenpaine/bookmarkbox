package mocks

import (
	"slices"
	"strings"
	"time"

	"github.com/darrenpaine/bookmarkbox/internal/models"
)

// Bookmark 1 belongs to user 1 (alice@example.com in the user mock).
var mockBookmark = &models.Bookmark{
	ID:          1,
	UserID:      1,
	URL:         "https://go.dev/doc/effective_go",
	Domain:      "go.dev",
	Title:       "Effective Go",
	Description: "Tips for writing clear, idiomatic Go code.",
	Created:     time.Now(),
	Tags:        []string{"docs", "go"},
}

// DuplicateURL is rejected by Update, as if another bookmark already had it.
const DuplicateURL = "https://duplicate.example.com"

// Saving with this title or Tag behaves as if a limit has been reached.
const (
	OverBookmarkLimitTitle = "Over bookmark limit"
	OverTagLimitTag        = "over-tag-limit"
)

type BookmarkModel struct{}

func (m *BookmarkModel) Insert(userID int, b *models.Bookmark) (int, error) {
	if b.Title == OverBookmarkLimitTitle {
		return 0, models.ErrBookmarkLimit
	}
	if slices.Contains(models.NormaliseTags(b.Tags), OverTagLimitTag) {
		return 0, models.ErrTagLimit
	}
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

// BadMarker is refused by List, as a marker that can't be read.
const BadMarker = "bad-marker"

// List gives user 1 the mock Bookmark, unless a filter excludes it or the
// list starts after MockMarker, the mock Bookmark's own marker.
func (m *BookmarkModel) List(userID int, q models.ListQuery) (*models.ListPage, error) {
	if q.After == BadMarker || q.UpTo == BadMarker {
		return nil, models.ErrBadMarker
	}
	page := &models.ListPage{Bookmarks: []*models.Bookmark{}}
	if userID != mockBookmark.UserID || q.After == MockMarker || !matches(q.Filter) {
		return page, nil
	}
	page.Bookmarks = append(page.Bookmarks, mockBookmark)
	page.Total = 1
	page.Last = MockMarker
	return page, nil
}

// MockMarker is the marker List gives for the mock Bookmark.
const MockMarker = "mock-marker"

func matches(f models.Filter) bool {
	if f.Favourite && !mockBookmark.Favourite || f.ReadLater && !mockBookmark.ReadLater {
		return false
	}
	if f.Domain != "" && f.Domain != mockBookmark.Domain {
		return false
	}
	// Searches only the title: every word must be in it.
	for _, word := range strings.Fields(f.Search) {
		if !strings.Contains(strings.ToLower(mockBookmark.Title), strings.ToLower(word)) {
			return false
		}
	}
	for _, tag := range f.Tags {
		if !slices.Contains(mockBookmark.Tags, tag) {
			return false
		}
	}
	return true
}

func (m *BookmarkModel) TagCounts(userID int, f models.Filter) ([]*models.Tag, error) {
	tags := []*models.Tag{}
	if userID == mockBookmark.UserID && matches(f) {
		for i, name := range mockBookmark.Tags {
			tags = append(tags, &models.Tag{ID: i + 1, Name: name, BookmarkCount: 1})
		}
	}
	return tags, nil
}

func (m *BookmarkModel) Update(userID int, b *models.Bookmark) error {
	if _, err := m.Get(userID, b.ID); err != nil {
		return err
	}
	if b.URL == DuplicateURL {
		return models.ErrDuplicateURL
	}
	if slices.Contains(models.NormaliseTags(b.Tags), OverTagLimitTag) {
		return models.ErrTagLimit
	}
	return nil
}

func (m *BookmarkModel) ToggleFavourite(userID, id int) error {
	_, err := m.Get(userID, id)
	return err
}

func (m *BookmarkModel) ToggleReadLater(userID, id int) error {
	_, err := m.Get(userID, id)
	return err
}

func (m *BookmarkModel) AddTag(userID, id int, name string) error {
	if _, err := m.Get(userID, id); err != nil {
		return err
	}
	if slices.Contains(models.NormaliseTags([]string{name}), OverTagLimitTag) {
		return models.ErrTagLimit
	}
	return nil
}

func (m *BookmarkModel) RemoveTag(userID, id int, name string) error {
	_, err := m.Get(userID, id)
	return err
}

func (m *BookmarkModel) Delete(userID, id int) error {
	_, err := m.Get(userID, id)
	return err
}

func (m *BookmarkModel) All(userID int) ([]*models.Bookmark, error) {
	if userID == mockBookmark.UserID {
		return []*models.Bookmark{mockBookmark}, nil
	}
	return []*models.Bookmark{}, nil
}

func (m *BookmarkModel) Count(userID int, f models.Filter) (int, error) {
	if userID == mockBookmark.UserID && matches(f) {
		return 1, nil
	}
	return 0, nil
}

// PreviewImport counts the mock Bookmark's URL as a Duplicate for user 1. An
// item titled OverBookmarkLimitTitle passes a Bookmark limit of 1.
func (m *BookmarkModel) PreviewImport(userID int, items []models.ImportItem) (*models.ImportSummary, error) {
	s := &models.ImportSummary{}
	for _, item := range items {
		if item.URL == mockBookmark.URL && userID == mockBookmark.UserID {
			s.Duplicates++
			continue
		}
		s.New++
		if item.Unfiled {
			s.Unfiled++
		}
		if item.Title == OverBookmarkLimitTitle {
			s.MaxBookmarks = 1
		}
	}
	existing, _ := m.Count(userID, models.Filter{})
	s.BookmarksAfter = existing + s.New
	return s, nil
}

func (m *BookmarkModel) Import(userID int, items []models.ImportItem) (*models.ImportSummary, error) {
	s, _ := m.PreviewImport(userID, items)
	if s.OverBookmarkLimit() {
		return nil, models.ErrBookmarkLimit
	}
	return s, nil
}
