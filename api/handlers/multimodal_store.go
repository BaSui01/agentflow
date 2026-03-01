package handlers

import (
	"sync"
	"time"
)

// ReferenceStore defines how multimodal reference images are persisted.
// Redis is the production backend; in-memory implementation is kept for tests.
type ReferenceStore interface {
	Save(asset *referenceAsset) error
	Get(id string) (*referenceAsset, bool)
	Delete(id string)
	Cleanup(expireBefore time.Time)
}

type MemoryReferenceStore struct {
	mu   sync.RWMutex
	data map[string]*referenceAsset
}

func NewMemoryReferenceStore() *MemoryReferenceStore {
	return &MemoryReferenceStore{data: make(map[string]*referenceAsset)}
}

func (s *MemoryReferenceStore) Save(asset *referenceAsset) error {
	if asset == nil {
		return nil
	}
	s.mu.Lock()
	s.data[asset.ID] = asset
	s.mu.Unlock()
	return nil
}

func (s *MemoryReferenceStore) Get(id string) (*referenceAsset, bool) {
	s.mu.RLock()
	asset, ok := s.data[id]
	s.mu.RUnlock()
	return asset, ok
}

func (s *MemoryReferenceStore) Delete(id string) {
	s.mu.Lock()
	delete(s.data, id)
	s.mu.Unlock()
}

func (s *MemoryReferenceStore) Cleanup(expireBefore time.Time) {
	s.mu.Lock()
	for id, asset := range s.data {
		if asset.CreatedAt.Before(expireBefore) {
			delete(s.data, id)
		}
	}
	s.mu.Unlock()
}

