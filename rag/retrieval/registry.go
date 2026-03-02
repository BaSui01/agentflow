package retrieval

import (
	"fmt"
	"sort"
	"sync"
)

// StrategyFactory builds a retriever for a concrete strategy kind.
type StrategyFactory func() (Retriever, error)

// StrategyRegistry centralizes retrieval strategy construction.
type StrategyRegistry struct {
	mu        sync.RWMutex
	factories map[StrategyKind]StrategyFactory
}

// NewStrategyRegistry creates an empty retrieval strategy registry.
func NewStrategyRegistry() *StrategyRegistry {
	return &StrategyRegistry{factories: map[StrategyKind]StrategyFactory{}}
}

// Register adds a strategy factory. Duplicate registration is rejected.
func (r *StrategyRegistry) Register(kind StrategyKind, factory StrategyFactory) error {
	if factory == nil {
		return fmt.Errorf("strategy factory is nil: %s", kind)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.factories[kind]; exists {
		return fmt.Errorf("strategy already registered: %s", kind)
	}
	r.factories[kind] = factory
	return nil
}

// Build constructs a retriever by strategy kind.
func (r *StrategyRegistry) Build(kind StrategyKind) (Retriever, error) {
	r.mu.RLock()
	factory, ok := r.factories[kind]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("strategy is not registered: %s", kind)
	}
	return factory()
}

// List returns all registered strategy kinds.
func (r *StrategyRegistry) List() []StrategyKind {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]StrategyKind, 0, len(r.factories))
	for k := range r.factories {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		return string(out[i]) < string(out[j])
	})
	return out
}

// RegisterDefaultStrategies mounts available strategy nodes into registry.
func RegisterDefaultStrategies(reg *StrategyRegistry, nodes StrategyNodes) error {
	if reg == nil {
		return fmt.Errorf("strategy registry is nil")
	}

	defaults := map[StrategyKind]struct {
		enabled bool
	}{
		StrategyHybrid:     {enabled: nodes.Hybrid != nil},
		StrategyBM25:       {enabled: nodes.BM25 != nil},
		StrategyVector:     {enabled: nodes.Vector != nil},
		StrategyGraph:      {enabled: nodes.Graph != nil},
		StrategyContextual: {enabled: nodes.Contextual != nil},
		StrategyMultiHop:   {enabled: nodes.MultiHop != nil},
	}

	for kind, v := range defaults {
		if !v.enabled {
			continue
		}
		k := kind
		if err := reg.Register(k, func() (Retriever, error) {
			return NewStrategyNode(k, nodes)
		}); err != nil {
			return err
		}
	}
	return nil
}

