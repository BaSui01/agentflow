package handlers

import (
	"github.com/BaSui01/agentflow/agent/hosted"
	"gorm.io/gorm"
)

// ToolProviderStore defines DB access for hosted tool provider configs.
type ToolProviderStore interface {
	List() ([]hosted.ToolProviderConfig, error)
	GetByProvider(provider string) (hosted.ToolProviderConfig, error)
	Create(row *hosted.ToolProviderConfig) error
	Update(row *hosted.ToolProviderConfig, updates map[string]any) error
	Reload(row *hosted.ToolProviderConfig) error
	DeleteByProvider(provider string) (int64, error)
}

type GormToolProviderStore struct {
	db *gorm.DB
}

func NewGormToolProviderStore(db *gorm.DB) *GormToolProviderStore {
	return &GormToolProviderStore{db: db}
}

func (s *GormToolProviderStore) List() ([]hosted.ToolProviderConfig, error) {
	var rows []hosted.ToolProviderConfig
	err := s.db.Order("priority ASC, id ASC").Limit(500).Find(&rows).Error
	return rows, err
}

func (s *GormToolProviderStore) GetByProvider(provider string) (hosted.ToolProviderConfig, error) {
	var row hosted.ToolProviderConfig
	err := s.db.Where("provider = ?", provider).First(&row).Error
	return row, err
}

func (s *GormToolProviderStore) Create(row *hosted.ToolProviderConfig) error {
	return s.db.Create(row).Error
}

func (s *GormToolProviderStore) Update(row *hosted.ToolProviderConfig, updates map[string]any) error {
	return s.db.Model(row).Updates(updates).Error
}

func (s *GormToolProviderStore) Reload(row *hosted.ToolProviderConfig) error {
	return s.db.First(row, row.ID).Error
}

func (s *GormToolProviderStore) DeleteByProvider(provider string) (int64, error) {
	result := s.db.Where("provider = ?", provider).Delete(&hosted.ToolProviderConfig{})
	return result.RowsAffected, result.Error
}
