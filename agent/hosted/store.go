package hosted

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// ErrNotFound is returned when a record lookup yields no result.
var ErrNotFound = errors.New("record not found")

// ToolRegistryStore defines DB access for tool registration management.
type ToolRegistryStore interface {
	List() ([]ToolRegistration, error)
	Create(row *ToolRegistration) error
	GetByID(id uint) (ToolRegistration, error)
	GetByName(name string) (ToolRegistration, error)
	Update(row *ToolRegistration, updates map[string]any) error
	Reload(row *ToolRegistration) error
	Delete(id uint) (int64, error)
	WithTransaction(ctx context.Context, fn func(ToolRegistryStore) error) error
}

// GormToolRegistryStore implements ToolRegistryStore on top of gorm.
type GormToolRegistryStore struct {
	db *gorm.DB
}

// NewGormToolRegistryStore creates a GORM-backed tool registry store.
func NewGormToolRegistryStore(db *gorm.DB) *GormToolRegistryStore {
	return &GormToolRegistryStore{db: db}
}

func (s *GormToolRegistryStore) List() ([]ToolRegistration, error) {
	var rows []ToolRegistration
	err := s.db.Order("id ASC").Limit(500).Find(&rows).Error
	return rows, err
}

func (s *GormToolRegistryStore) Create(row *ToolRegistration) error {
	return s.db.Create(row).Error
}

func (s *GormToolRegistryStore) GetByID(id uint) (ToolRegistration, error) {
	var row ToolRegistration
	err := s.db.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, ErrNotFound
	}
	return row, err
}

func (s *GormToolRegistryStore) GetByName(name string) (ToolRegistration, error) {
	var row ToolRegistration
	err := s.db.Where("name = ?", name).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, ErrNotFound
	}
	return row, err
}

func (s *GormToolRegistryStore) Update(row *ToolRegistration, updates map[string]any) error {
	return s.db.Model(row).Updates(updates).Error
}

func (s *GormToolRegistryStore) Reload(row *ToolRegistration) error {
	return s.db.First(row, row.ID).Error
}

func (s *GormToolRegistryStore) Delete(id uint) (int64, error) {
	result := s.db.Where("id = ?", id).Delete(&ToolRegistration{})
	return result.RowsAffected, result.Error
}

func (s *GormToolRegistryStore) WithTransaction(ctx context.Context, fn func(ToolRegistryStore) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(NewGormToolRegistryStore(tx))
	})
}
