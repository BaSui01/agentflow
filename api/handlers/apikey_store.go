package handlers

import (
	"github.com/BaSui01/agentflow/llm"
	"gorm.io/gorm"
)

// APIKeyStore defines the data access operations required by APIKeyHandler.
type APIKeyStore interface {
	// ListProviders returns all LLM providers ordered by ID.
	ListProviders() ([]llm.LLMProvider, error)

	// ListAPIKeys returns all API keys for a given provider, ordered by priority then ID.
	ListAPIKeys(providerID uint) ([]llm.LLMProviderAPIKey, error)

	// CreateAPIKey persists a new API key record.
	CreateAPIKey(key *llm.LLMProviderAPIKey) error

	// GetAPIKey retrieves a single API key by key ID and provider ID.
	GetAPIKey(keyID, providerID uint) (llm.LLMProviderAPIKey, error)

	// UpdateAPIKey applies partial updates to an existing API key.
	UpdateAPIKey(key *llm.LLMProviderAPIKey, updates map[string]any) error

	// ReloadAPIKey refreshes the API key record from the database.
	ReloadAPIKey(key *llm.LLMProviderAPIKey) error

	// DeleteAPIKey removes an API key by key ID and provider ID.
	// Returns the number of rows affected.
	DeleteAPIKey(keyID, providerID uint) (int64, error)
}

// GormAPIKeyStore implements APIKeyStore using gorm.DB.
type GormAPIKeyStore struct {
	db *gorm.DB
}

// NewGormAPIKeyStore creates a new GormAPIKeyStore.
func NewGormAPIKeyStore(db *gorm.DB) *GormAPIKeyStore {
	return &GormAPIKeyStore{db: db}
}

func (s *GormAPIKeyStore) ListProviders() ([]llm.LLMProvider, error) {
	var providers []llm.LLMProvider
	err := s.db.Order("id ASC").Find(&providers).Error
	return providers, err
}

func (s *GormAPIKeyStore) ListAPIKeys(providerID uint) ([]llm.LLMProviderAPIKey, error) {
	var keys []llm.LLMProviderAPIKey
	err := s.db.Where("provider_id = ?", providerID).Order("priority ASC, id ASC").Find(&keys).Error
	return keys, err
}

func (s *GormAPIKeyStore) CreateAPIKey(key *llm.LLMProviderAPIKey) error {
	return s.db.Create(key).Error
}

func (s *GormAPIKeyStore) GetAPIKey(keyID, providerID uint) (llm.LLMProviderAPIKey, error) {
	var key llm.LLMProviderAPIKey
	err := s.db.Where("id = ? AND provider_id = ?", keyID, providerID).First(&key).Error
	return key, err
}

func (s *GormAPIKeyStore) UpdateAPIKey(key *llm.LLMProviderAPIKey, updates map[string]any) error {
	return s.db.Model(key).Updates(updates).Error
}

func (s *GormAPIKeyStore) ReloadAPIKey(key *llm.LLMProviderAPIKey) error {
	return s.db.First(key, key.ID).Error
}

func (s *GormAPIKeyStore) DeleteAPIKey(keyID, providerID uint) (int64, error) {
	result := s.db.Where("id = ? AND provider_id = ?", keyID, providerID).Delete(&llm.LLMProviderAPIKey{})
	return result.RowsAffected, result.Error
}

