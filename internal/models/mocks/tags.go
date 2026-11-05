package mocks

import "github.com/darrenpaine/bookmarkbox/internal/models"

// Tags 1 and 2 belong to user 1. Rename treats "docs" and "go" as already taken.
var mockTags = []*models.Tag{
	{ID: 1, Name: "docs", BookmarkCount: 1},
	{ID: 2, Name: "go", BookmarkCount: 3},
}

type TagModel struct{}

func (m *TagModel) Get(userID, id int) (*models.Tag, error) {
	for _, tag := range mockTags {
		if userID == 1 && tag.ID == id {
			return tag, nil
		}
	}
	return nil, models.ErrNoRecord
}

func (m *TagModel) List(userID int) ([]*models.Tag, error) {
	if userID == 1 {
		return mockTags, nil
	}
	return []*models.Tag{}, nil
}

func (m *TagModel) Rename(userID, id int, name string) error {
	if _, err := m.Get(userID, id); err != nil {
		return err
	}
	for _, tag := range mockTags {
		if tag.Name == name {
			return models.ErrDuplicateTag
		}
	}
	return nil
}

func (m *TagModel) Merge(userID, id int, name string) error {
	_, err := m.Get(userID, id)
	return err
}
