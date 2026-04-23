package hosted

import (
	"errors"

	"gorm.io/gorm"
)

// ToolProviderStore defines DB access for hosted tool provider configs.
type ToolProviderStore interface {
	List() ([]ToolProviderConfig, error)
	GetByProvider(provider string) (ToolProviderConfig, error)
	Create(row *ToolProviderConfig) error
	Update(row *ToolProviderConfig, updates map[string]any) error
	Reload(row *ToolProviderConfig) error
	DeleteByProvider(provider string) (int64, error)
}

// GormToolProviderStore implements ToolProviderStore on top of gorm.
type GormToolProviderStore struct {
	db *gorm.DB
}

// NewGormToolProviderStore creates a GORM-backed tool provider store.
func NewGormToolProviderStore(db *gorm.DB) *GormToolProviderStore {
	return &GormToolProviderStore{db: db}
}

func (s *GormToolProviderStore) List() ([]ToolProviderConfig, error) {
	var rows []ToolProviderConfig
	err := s.db.Order("priority ASC, id ASC").Limit(500).Find(&rows).Error
	return rows, err
}

func (s *GormToolProviderStore) GetByProvider(provider string) (ToolProviderConfig, error) {
	var row ToolProviderConfig
	err := s.db.Where("provider = ?", provider).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, ErrNotFound
	}
	return row, err
}

func (s *GormToolProviderStore) Create(row *ToolProviderConfig) error {
	return s.db.Create(row).Error
}

func (s *GormToolProviderStore) Update(row *ToolProviderConfig, updates map[string]any) error {
	return s.db.Model(row).Updates(updates).Error
}

func (s *GormToolProviderStore) Reload(row *ToolProviderConfig) error {
	return s.db.First(row, row.ID).Error
}

func (s *GormToolProviderStore) DeleteByProvider(provider string) (int64, error) {
	result := s.db.Where("provider = ?", provider).Delete(&ToolProviderConfig{})
	return result.RowsAffected, result.Error
}
