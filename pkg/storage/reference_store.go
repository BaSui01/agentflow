package storage

import (
	"sync"
	"time"
)

// ReferenceAsset represents a stored multimodal reference (e.g. uploaded image).
type ReferenceAsset struct {
	ID        string    `json:"id"`
	FileName  string    `json:"file_name"`
	MimeType  string    `json:"mime_type"`
	Size      int       `json:"size"`
	CreatedAt time.Time `json:"created_at"`
	Data      []byte    `json:"-"`
}

// ReferenceStore defines how multimodal reference images are persisted.
// Redis is the production backend; in-memory implementation is kept for tests.
type ReferenceStore interface {
	Save(asset *ReferenceAsset) error
	Get(id string) (*ReferenceAsset, bool)
	Delete(id string)
	Cleanup(expireBefore time.Time)
}

// MemoryReferenceStore is an in-memory implementation of ReferenceStore.
type MemoryReferenceStore struct {
	mu   sync.RWMutex
	data map[string]*ReferenceAsset
}

// NewMemoryReferenceStore creates an in-memory reference store.
func NewMemoryReferenceStore() *MemoryReferenceStore {
	return &MemoryReferenceStore{data: make(map[string]*ReferenceAsset)}
}

func (s *MemoryReferenceStore) Save(asset *ReferenceAsset) error {
	if asset == nil {
		return nil
	}
	s.mu.Lock()
	s.data[asset.ID] = asset
	s.mu.Unlock()
	return nil
}

func (s *MemoryReferenceStore) Get(id string) (*ReferenceAsset, bool) {
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
