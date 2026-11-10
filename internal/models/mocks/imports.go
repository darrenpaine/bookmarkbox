package mocks

import (
	"time"

	"github.com/darrenpaine/bookmarkbox/internal/models"
)

// ImportModel keeps pending imports in memory.
type ImportModel struct {
	pending map[int]*models.PendingImport
}

func (m *ImportModel) Save(userID int, filename string, data []byte) error {
	if m.pending == nil {
		m.pending = map[int]*models.PendingImport{}
	}
	m.pending[userID] = &models.PendingImport{Filename: filename, Data: data, Created: time.Now()}
	return nil
}

func (m *ImportModel) Get(userID int) (*models.PendingImport, error) {
	p, ok := m.pending[userID]
	if !ok {
		return nil, models.ErrNoRecord
	}
	return p, nil
}

func (m *ImportModel) Delete(userID int) error {
	delete(m.pending, userID)
	return nil
}
