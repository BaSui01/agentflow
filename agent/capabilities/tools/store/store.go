package store

import (
	"context"
	"fmt"
	"sync"
)

// KeyFunc returns the stable persistence key for an item.
type KeyFunc[T any] func(T) (string, error)

// ValidateFunc validates an item before it is persisted.
type ValidateFunc[T any] func(T) error

// InMemoryRegistryStore is a concurrency-safe in-memory registry store.
//
// It is intentionally generic so the store subpackage stays below the tools
// facade and does not depend on root package types such as AgentInfo.
type InMemoryRegistryStore[T any] struct {
	mu       sync.RWMutex
	items    map[string]T
	keyFunc  KeyFunc[T]
	validate ValidateFunc[T]
}

// NewInMemoryRegistryStore creates an in-memory registry store.
func NewInMemoryRegistryStore[T any](keyFunc KeyFunc[T], validate ValidateFunc[T]) (*InMemoryRegistryStore[T], error) {
	if keyFunc == nil {
		return nil, fmt.Errorf("key func is required")
	}
	return &InMemoryRegistryStore[T]{
		items:    make(map[string]T),
		keyFunc:  keyFunc,
		validate: validate,
	}, nil
}

// Save stores or replaces an item.
func (s *InMemoryRegistryStore[T]) Save(ctx context.Context, item T) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil {
		return fmt.Errorf("store is nil")
	}
	if s.validate != nil {
		if err := s.validate(item); err != nil {
			return err
		}
	}
	key, err := s.keyFunc(item)
	if err != nil {
		return err
	}
	if key == "" {
		return fmt.Errorf("key is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[key] = item
	return nil
}

// Load retrieves an item by key.
func (s *InMemoryRegistryStore[T]) Load(ctx context.Context, id string) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if s == nil {
		return zero, fmt.Errorf("store is nil")
	}
	if id == "" {
		return zero, fmt.Errorf("id is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[id]
	if !ok {
		return zero, fmt.Errorf("item %s not found", id)
	}
	return item, nil
}

// LoadAll returns all stored items.
func (s *InMemoryRegistryStore[T]) LoadAll(ctx context.Context) ([]T, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]T, 0, len(s.items))
	for _, item := range s.items {
		result = append(result, item)
	}
	return result, nil
}

// Delete removes an item by key.
func (s *InMemoryRegistryStore[T]) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil {
		return fmt.Errorf("store is nil")
	}
	if id == "" {
		return fmt.Errorf("id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok {
		return fmt.Errorf("item %s not found", id)
	}
	delete(s.items, id)
	return nil
}
