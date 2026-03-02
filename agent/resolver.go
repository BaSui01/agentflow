package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

// CachingResolver resolves agent IDs to live Agent instances, creating them
// on demand via AgentRegistry and caching them for reuse. It uses singleflight
// to ensure concurrent requests for the same agentID only trigger one
// Create+Init cycle.
type CachingResolver struct {
	registry *AgentRegistry
	provider llm.Provider
	memory   MemoryManager // optional; nil means stateless agents
	logger   *zap.Logger
	agents   sync.Map
	group    singleflight.Group

	// MongoDB persistence stores (required)
	promptStore       PromptStoreProvider
	conversationStore ConversationStoreProvider
	runStore          RunStoreProvider
}

// NewCachingResolver creates a CachingResolver backed by the given registry
// and LLM provider.
func NewCachingResolver(registry *AgentRegistry, provider llm.Provider, logger *zap.Logger) *CachingResolver {
	return &CachingResolver{
		registry: registry,
		provider: provider,
		logger:   logger,
	}
}

// WithMemory sets the MemoryManager used when creating new agent instances.
// When non-nil, agents created by this resolver will have memory capabilities.
func (r *CachingResolver) WithMemory(m MemoryManager) *CachingResolver {
	r.memory = m
	return r
}

// WithPromptStore sets the PromptStoreProvider for resolved agents.
func (r *CachingResolver) WithPromptStore(s PromptStoreProvider) *CachingResolver {
	r.promptStore = s
	return r
}

// WithConversationStore sets the ConversationStoreProvider for resolved agents.
func (r *CachingResolver) WithConversationStore(s ConversationStoreProvider) *CachingResolver {
	r.conversationStore = s
	return r
}

// WithRunStore sets the RunStoreProvider for resolved agents.
func (r *CachingResolver) WithRunStore(s RunStoreProvider) *CachingResolver {
	r.runStore = s
	return r
}

// Resolve returns a cached Agent for agentID, or creates and initialises one.
func (r *CachingResolver) Resolve(ctx context.Context, agentID string) (Agent, error) {
	// Fast path: already cached.
	if cached, ok := r.agents.Load(agentID); ok {
		return cached.(Agent), nil
	}

	// Deduplicate concurrent creation for the same ID.
	result, err, _ := r.group.Do(agentID, func() (any, error) {
		// Double-check after acquiring the flight.
		if cached, ok := r.agents.Load(agentID); ok {
			return cached, nil
		}

		cfg := types.AgentConfig{
			Core: types.CoreConfig{
				ID:   agentID,
				Name: agentID,
				Type: string(TypeGeneric),
			},
		}
		ag, err := r.registry.Create(cfg, r.provider, r.memory, nil, nil, r.logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create agent %q: %w", agentID, err)
		}

		// Inject MongoDB persistence stores.
		if ba, ok := ag.(*BaseAgent); ok {
			ba.SetPromptStore(r.promptStore)
			ba.SetConversationStore(r.conversationStore)
			ba.SetRunStore(r.runStore)
		}

		if err := ag.Init(ctx); err != nil {
			return nil, fmt.Errorf("failed to init agent %q: %w", agentID, err)
		}

		r.agents.Store(agentID, ag)
		return ag, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(Agent), nil
}

// TeardownAll tears down all cached agent instances. Intended to be called
// during graceful shutdown.
func (r *CachingResolver) TeardownAll(ctx context.Context) {
	r.agents.Range(func(key, value any) bool {
		if ag, ok := value.(Agent); ok {
			if err := ag.Teardown(ctx); err != nil {
				r.logger.Warn("Failed to teardown cached agent",
					zap.String("agent_id", key.(string)),
					zap.Error(err))
			}
		}
		return true
	})
}
