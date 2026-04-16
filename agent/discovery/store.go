package discovery

import (
	"context"
	"fmt"
	"sync"
)

// RegistryStore defines the persistence interface for agent registry data.
// Implementations can back the registry with different storage backends
// (in-memory, database, etc.).
type RegistryStore interface {
	Save(ctx context.Context, agent *AgentInfo) error
	Load(ctx context.Context, id string) (*AgentInfo, error)
	LoadAll(ctx context.Context) ([]*AgentInfo, error)
	Delete(ctx context.Context, id string) error
}

// InMemoryRegistryStore is a RegistryStore backed by an in-memory map.
// It preserves the existing default behavior of CapabilityRegistry.
type InMemoryRegistryStore struct {
	mu     sync.RWMutex
	agents map[string]*AgentInfo
}

// NewInMemoryRegistryStore creates a new InMemoryRegistryStore.
func NewInMemoryRegistryStore() *InMemoryRegistryStore {
	return &InMemoryRegistryStore{
		agents: make(map[string]*AgentInfo),
	}
}

func (s *InMemoryRegistryStore) Save(_ context.Context, agent *AgentInfo) error {
	if agent == nil || agent.Card == nil {
		return fmt.Errorf("invalid agent info")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agents[agent.Card.Name] = agent
	return nil
}

func (s *InMemoryRegistryStore) Load(_ context.Context, id string) (*AgentInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	info, ok := s.agents[id]
	if !ok {
		return nil, fmt.Errorf("agent %s not found", id)
	}
	return info, nil
}

func (s *InMemoryRegistryStore) LoadAll(_ context.Context) ([]*AgentInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*AgentInfo, 0, len(s.agents))
	for _, info := range s.agents {
		result = append(result, info)
	}
	return result, nil
}

func (s *InMemoryRegistryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.agents[id]; !ok {
		return fmt.Errorf("agent %s not found", id)
	}
	delete(s.agents, id)
	return nil
}

// Ensure InMemoryRegistryStore implements RegistryStore.
var _ RegistryStore = (*InMemoryRegistryStore)(nil)
