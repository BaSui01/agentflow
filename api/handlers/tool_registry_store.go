package handlers

import (
	"github.com/BaSui01/agentflow/agent/hosted"
	"gorm.io/gorm"
)

// ToolRegistryStore defines DB access for tool registration management.
type ToolRegistryStore interface {
	List() ([]hosted.ToolRegistration, error)
	Create(row *hosted.ToolRegistration) error
	GetByID(id uint) (hosted.ToolRegistration, error)
	GetByName(name string) (hosted.ToolRegistration, error)
	Update(row *hosted.ToolRegistration, updates map[string]any) error
	Reload(row *hosted.ToolRegistration) error
	Delete(id uint) (int64, error)
}

// GormToolRegistryStore implements ToolRegistryStore on top of gorm.
type GormToolRegistryStore struct {
	db *gorm.DB
}

func NewGormToolRegistryStore(db *gorm.DB) *GormToolRegistryStore {
	return &GormToolRegistryStore{db: db}
}

func (s *GormToolRegistryStore) List() ([]hosted.ToolRegistration, error) {
	var rows []hosted.ToolRegistration
	err := s.db.Order("id ASC").Find(&rows).Error
	return rows, err
}

func (s *GormToolRegistryStore) Create(row *hosted.ToolRegistration) error {
	return s.db.Create(row).Error
}

func (s *GormToolRegistryStore) GetByID(id uint) (hosted.ToolRegistration, error) {
	var row hosted.ToolRegistration
	err := s.db.Where("id = ?", id).First(&row).Error
	return row, err
}

func (s *GormToolRegistryStore) GetByName(name string) (hosted.ToolRegistration, error) {
	var row hosted.ToolRegistration
	err := s.db.Where("name = ?", name).First(&row).Error
	return row, err
}

func (s *GormToolRegistryStore) Update(row *hosted.ToolRegistration, updates map[string]any) error {
	return s.db.Model(row).Updates(updates).Error
}

func (s *GormToolRegistryStore) Reload(row *hosted.ToolRegistration) error {
	return s.db.First(row, row.ID).Error
}

func (s *GormToolRegistryStore) Delete(id uint) (int64, error) {
	result := s.db.Where("id = ?", id).Delete(&hosted.ToolRegistration{})
	return result.RowsAffected, result.Error
}
