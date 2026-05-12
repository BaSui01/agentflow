package tools

import (
	"context"
	"fmt"

	toolstore "github.com/BaSui01/agentflow/agent/capabilities/tools/store"
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
	inner *toolstore.InMemoryRegistryStore[*AgentInfo]
}

// NewInMemoryRegistryStore creates a new InMemoryRegistryStore.
func NewInMemoryRegistryStore() *InMemoryRegistryStore {
	inner, err := toolstore.NewInMemoryRegistryStore(agentInfoStoreKey, validateAgentInfoForStore)
	if err != nil {
		panic(err)
	}
	return &InMemoryRegistryStore{
		inner: inner,
	}
}

func agentInfoStoreKey(agent *AgentInfo) (string, error) {
	if agent == nil || agent.Card == nil {
		return "", fmt.Errorf("invalid agent info")
	}
	return agent.Card.Name, nil
}

func validateAgentInfoForStore(agent *AgentInfo) error {
	if agent == nil || agent.Card == nil {
		return fmt.Errorf("invalid agent info")
	}
	return nil
}

func (s *InMemoryRegistryStore) Save(ctx context.Context, agent *AgentInfo) error {
	return s.inner.Save(ctx, agent)
}

func (s *InMemoryRegistryStore) Load(ctx context.Context, id string) (*AgentInfo, error) {
	info, err := s.inner.Load(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("agent %s not found", id)
	}
	return info, nil
}

func (s *InMemoryRegistryStore) LoadAll(ctx context.Context) ([]*AgentInfo, error) {
	return s.inner.LoadAll(ctx)
}

func (s *InMemoryRegistryStore) Delete(ctx context.Context, id string) error {
	if err := s.inner.Delete(ctx, id); err != nil {
		return fmt.Errorf("agent %s not found", id)
	}
	return nil
}

// Ensure InMemoryRegistryStore implements RegistryStore.
var _ RegistryStore = (*InMemoryRegistryStore)(nil)
