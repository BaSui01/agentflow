package router

import "gorm.io/gorm"

// GormAPIKeyStore implements the API key admin store on top of gorm.
type GormAPIKeyStore struct {
	db *gorm.DB
}

// NewGormAPIKeyStore creates a GORM-backed API key admin store.
func NewGormAPIKeyStore(db *gorm.DB) *GormAPIKeyStore {
	return &GormAPIKeyStore{db: db}
}

func (s *GormAPIKeyStore) ListProviders() ([]LLMProvider, error) {
	var providers []LLMProvider
	err := s.db.Order("id ASC").Limit(500).Find(&providers).Error
	return providers, err
}

func (s *GormAPIKeyStore) ListAPIKeys(providerID uint) ([]LLMProviderAPIKey, error) {
	var keys []LLMProviderAPIKey
	err := s.db.Where("provider_id = ?", providerID).Order("priority ASC, id ASC").Limit(500).Find(&keys).Error
	return keys, err
}

func (s *GormAPIKeyStore) CreateAPIKey(key *LLMProviderAPIKey) error {
	return s.db.Create(key).Error
}

func (s *GormAPIKeyStore) GetAPIKey(keyID, providerID uint) (LLMProviderAPIKey, error) {
	var key LLMProviderAPIKey
	err := s.db.Where("id = ? AND provider_id = ?", keyID, providerID).First(&key).Error
	return key, err
}

func (s *GormAPIKeyStore) UpdateAPIKey(key *LLMProviderAPIKey, updates map[string]any) error {
	return s.db.Model(key).Updates(updates).Error
}

func (s *GormAPIKeyStore) ReloadAPIKey(key *LLMProviderAPIKey) error {
	return s.db.First(key, key.ID).Error
}

func (s *GormAPIKeyStore) DeleteAPIKey(keyID, providerID uint) (int64, error) {
	result := s.db.Where("id = ? AND provider_id = ?", keyID, providerID).Delete(&LLMProviderAPIKey{})
	return result.RowsAffected, result.Error
}
